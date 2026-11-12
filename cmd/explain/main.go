// explain roda o laco MAPE-K real (as mesmas etapas do cmd/selfscale) e
// explica em portugues o que cada etapa recebeu, decidiu e por que.
//
// Um unico laco roda; cada etapa escreve no seu proprio log (monitor.log,
// analisador.log, ...) e todas juntas em tudo.log. Assim cada terminal pode
// acompanhar uma etapa do MESMO laco (os numeros de ciclo batem).
//
// Modelos (--modelo):
//
//	pretreinado  Q-table salva (learning.qtable_path), politica gulosa, sem aprender
//	zero         Q-table zerada (sem conhecimento previo), aprende durante o laco
//	             com epsilon-greedy e salva em --salvar (nunca na Q-table pre-treinada)
//
// Normalmente e usado por scripts/selfscale.sh. Uso direto:
//
//	explain --etapa laco [--modelo zero] [--tela]   laco completo (atua no cluster)
//	explain --etapa executor --replicas N            executa um plano montado a mao
//	explain --etapa conhecimento [--modelo zero]     metas, politicas e Q-table
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"

	effectors "github.com/shna/selfscale-rl/manager/effectors/kube"
	"github.com/shna/selfscale-rl/manager/learning/qlearning"
	"github.com/shna/selfscale-rl/manager/mapek/analyzer"
	"github.com/shna/selfscale-rl/manager/mapek/executor"
	"github.com/shna/selfscale-rl/manager/mapek/knowledge"
	"github.com/shna/selfscale-rl/manager/mapek/monitor"
	"github.com/shna/selfscale-rl/manager/mapek/planner"
	sensors "github.com/shna/selfscale-rl/manager/sensors/kube"
	"github.com/shna/selfscale-rl/manager/sensors/prometheus"
	"github.com/shna/selfscale-rl/manager/storage"
)

// Cores ANSI para separar as etapas na tela.
const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	cyan   = "\033[36m"
	yellow = "\033[33m"
	green  = "\033[32m"
	red    = "\033[31m"
	blue   = "\033[34m"
	purple = "\033[35m"
)

// Modelos de decisao do laco.
const (
	modelPretrained = "pretreinado"
	modelZero       = "zero"
)

// Etapas e o nome do log de cada uma.
var stages = []string{"monitor", "analisador", "planejador", "executor", "conhecimento"}

func main() {
	etapa := flag.String("etapa", "laco", "laco | executor | conhecimento")
	cfgPath := flag.String("config", "configs/config.yaml", "arquivo de configuracao")
	logs := flag.String("logs", "data/laco", "pasta dos logs de cada etapa")
	tela := flag.Bool("tela", false, "laco: tambem mostra tudo nesta tela")
	ciclos := flag.Int("ciclos", 0, "numero de ciclos (0 = ate Ctrl+C)")
	replicas := flag.Int("replicas", 2, "etapa executor: numero de replicas desejado")
	modelo := flag.String("modelo", modelPretrained, "pretreinado | zero")
	salvar := flag.String("salvar", "data/qtable_zero.json", "modelo zero: onde salvar a Q-table aprendida")
	flag.Parse()

	m := model{name: *modelo, savePath: *salvar}
	if err := run(*etapa, *cfgPath, *logs, *tela, *ciclos, *replicas, m); err != nil {
		fmt.Fprintf(os.Stderr, "%serro:%s %v\n", red, reset, err)
		os.Exit(1)
	}
}

func run(etapa, cfgPath, logs string, tela bool, ciclos, replicas int, m model) error {
	if m.name != modelPretrained && m.name != modelZero {
		return fmt.Errorf("modelo desconhecido %q (use %s ou %s)", m.name, modelPretrained, modelZero)
	}
	cfg, err := storage.LoadConfig(cfgPath)
	if err != nil {
		return err
	}
	if etapa == "conhecimento" {
		return showKnowledge(os.Stdout, cfg, m)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch etapa {
	case "executor":
		d, err := newSession(cfg, consoleOutputs(), model{name: modelPretrained})
		if err != nil {
			return err
		}
		return d.executorOnly(ctx, replicas)
	case "laco":
		o, closeAll, err := fileOutputs(logs, tela)
		if err != nil {
			return err
		}
		defer closeAll()
		d, err := newSession(cfg, o, m)
		if err != nil {
			return err
		}
		return d.loop(ctx, ciclos)
	default:
		return fmt.Errorf("etapa desconhecida %q", etapa)
	}
}

// ------------------------------------------------------------------ saidas

// outputs leva o texto de cada etapa ao seu destino: o log da etapa e o
// log geral (tudo.log), mais a tela quando pedida.
type outputs struct {
	stage map[string]io.Writer // so o log da etapa
	all   io.Writer            // tudo.log (+ tela)
}

// w devolve o destino de uma etapa: o log dela e o geral.
func (o outputs) w(stage string) io.Writer {
	return io.MultiWriter(o.stage[stage], o.all)
}

// header escreve o cabecalho do ciclo uma vez em cada log.
func (o outputs) header(text string) {
	for _, s := range stages {
		fmt.Fprint(o.stage[s], text)
	}
	fmt.Fprint(o.all, text)
}

func consoleOutputs() outputs {
	o := outputs{stage: map[string]io.Writer{}, all: os.Stdout}
	for _, s := range stages {
		o.stage[s] = io.Discard
	}
	return o
}

func fileOutputs(dir string, tela bool) (outputs, func(), error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return outputs{}, nil, err
	}
	var files []*os.File
	closeAll := func() {
		for _, f := range files {
			_ = f.Close()
		}
	}
	open := func(name string) (*os.File, error) {
		f, err := os.Create(filepath.Join(dir, name+".log"))
		if err == nil {
			files = append(files, f)
		}
		return f, err
	}
	o := outputs{stage: map[string]io.Writer{}}
	for _, s := range stages {
		f, err := open(s)
		if err != nil {
			closeAll()
			return outputs{}, nil, err
		}
		o.stage[s] = f
	}
	all, err := open("tudo")
	if err != nil {
		closeAll()
		return outputs{}, nil, err
	}
	o.all = all
	if tela {
		o.all = io.MultiWriter(all, os.Stdout)
	}
	return o, closeAll, nil
}

// ------------------------------------------------------------------ montagem

// session monta os componentes reais, como o composition root cmd/selfscale.
type session struct {
	cfg      storage.Config
	s        knowledge.Settings
	out      outputs
	model    model
	agent    *qlearning.Agent
	store    storage.JSONQTable // onde o modelo zero salva o que aprendeu
	k        *knowledge.Base
	hist     *recorder
	effector *effectors.Effector
	mon      *monitor.Monitor
	ana      *analyzer.Analyzer
	pla      *planner.Planner
	exe      *executor.Executor
	sinceUp  int // ciclos desde o ultimo aumento (o que o cooldown olha)
}

// recorder e o historico visto pelo laco comentado: guarda o ultimo registro do ciclo
// (para mostrar na tela) e repassa ao CSV.
type recorder struct {
	csv  *storage.CSVHistory
	path string
	last *knowledge.CycleRecord
}

func (r *recorder) Write(c knowledge.CycleRecord) error {
	r.last = &c
	return r.csv.Write(c)
}

type noEpisodes struct{}

func (noEpisodes) EndEpisode() {}

// model e a escolha de Q-table do laco: pre-treinada (so usa) ou zerada (aprende).
type model struct {
	name     string
	savePath string
}

func (m model) learns() bool { return m.name == modelZero }

// newAgent monta o agente do modelo escolhido.
func newAgent(cfg storage.Config, m model) (*qlearning.Agent, error) {
	s, l := cfg.Settings(), cfg.Learning
	if m.learns() {
		lp := s.Learning
		lp.PriorInit = false // sem conhecimento previo: a tabela comeca toda em zero
		sel := qlearning.NewEpsilonGreedy(l.EpsilonFinetune, l.EpsilonMin, l.EpsilonDecay, l.Seed)
		return qlearning.NewAgent(s.Policy, lp, sel), nil
	}
	agent := qlearning.NewAgent(s.Policy, s.Learning, qlearning.Greedy{})
	table, err := (storage.JSONQTable{Path: l.QTablePath}).Load()
	if err != nil {
		return agent, nil // sem Q-table salva: usa o conhecimento previo
	}
	return agent, agent.SetTable(table)
}

func newSession(cfg storage.Config, out outputs, m model) (*session, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{CurrentContext: cfg.Target.KubeconfigContext}
	restCfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("carregando kubeconfig: %w", err)
	}
	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, err
	}
	metrics, err := metricsclient.NewForConfig(restCfg)
	if err != nil {
		return nil, err
	}
	t := cfg.Target
	sensor := sensors.NewSensor(client, metrics, t.Namespace, t.Deployment, t.PodSelector)
	eff := effectors.NewEffector(client, t.Namespace, t.Deployment)

	s := cfg.Settings()
	agent, err := newAgent(cfg, m)
	if err != nil {
		return nil, err
	}

	path := filepath.Join(cfg.Run.HistoryDir, "history_laco.csv")
	csv, err := storage.NewCSVHistory(path, "explain", m.name)
	if err != nil {
		return nil, err
	}
	hist := &recorder{csv: csv, path: path}
	k := knowledge.New(hist)
	traffic := prometheus.NewSource(cfg.Sensors.PrometheusURL, cfg.Sensors.Period, &http.Client{Timeout: 5 * time.Second})

	store := storage.JSONQTable{Path: m.savePath}
	return &session{
		cfg: cfg, s: s, out: out, model: m, agent: agent, store: store, k: k, hist: hist, effector: eff, sinceUp: math.MaxInt32,
		mon: monitor.New(traffic, sensor, sensor, monitor.DefaultProcessors()),
		ana: newAnalyzer(cfg, m, agent, store, k),
		pla: planner.New(k, s.Policy),
		exe: executor.New(eff, sensor, k, executor.Options{
			Target: cfg.TargetName(), Allowlist: cfg.Target.Allowlist, PollInterval: 2 * time.Second,
		}),
	}, nil
}

// newAnalyzer liga o aprendizado so no modelo zero. Ele salva a Q-table
// aprendida ao fim de cada episodio; o pre-treinado nunca altera a sua.
func newAnalyzer(cfg storage.Config, m model, agent *qlearning.Agent, store storage.JSONQTable, k *knowledge.Base) *analyzer.Analyzer {
	s := cfg.Settings()
	if !m.learns() {
		return analyzer.New(agent, agent, noEpisodes{}, k, s, analyzer.Options{Act: true})
	}
	ckpt := knowledge.Checkpointer{Agent: agent, Store: store}
	opts := analyzer.Options{Act: true, Learn: true, EpisodeCycles: cfg.Run.EpisodeCycles}
	return analyzer.New(agent, agent, ckpt, k, s, opts)
}

// ------------------------------------------------------------------ laco

func (d *session) loop(ctx context.Context, ciclos int) error {
	has, err := d.effector.HasHPA(ctx)
	if err != nil {
		return err
	}
	if has {
		return errors.New("ha um HPA no frontend; rode `make hpa-off` antes")
	}
	d.intro()
	defer d.saveLearned()
	ticker := time.NewTicker(d.cfg.Sensors.Period)
	defer ticker.Stop()
	for n := 1; ciclos == 0 || n <= ciclos; n++ {
		d.cycle(ctx, time.Now())
		if n == ciclos {
			break
		}
		// Uma execucao pode demorar mais que o periodo: reinicia a contagem para
		// o proximo ciclo medir de novo, sem repetir a mesma leitura.
		ticker.Reset(d.cfg.Sensors.Period)
		select {
		case <-ctx.Done():
			fmt.Fprintf(d.out.all, "\nLaco encerrado.\n")
			return nil
		case <-ticker.C:
		}
	}
	return nil
}

// saveLearned grava a Q-table aprendida pelo modelo zero ao encerrar o laco.
func (d *session) saveLearned() {
	if !d.model.learns() {
		return
	}
	if err := d.store.Save(d.agent.Table()); err != nil {
		fmt.Fprintf(d.out.all, "%sfalha ao salvar a Q-table aprendida:%s %v\n", red, reset, err)
		return
	}
	fmt.Fprintf(d.out.all, "Q-table aprendida salva em %s\n", d.store.Path)
}

func (d *session) intro() {
	text := fmt.Sprintf("%sSelfScale-RL — laco MAPE-K em execucao%s\n"+
		"%sUm ciclo a cada %s. SLA: p95 <= %.0f ms. Replicas entre %d e %d. ATUA no cluster.%s\n"+
		"%sModelo: %s%s\n"+
		"%sFluxo de cada ciclo: Monitor → Analisador → Planejador → Executor → Conhecimento%s\n",
		bold, reset, dim, d.cfg.Sensors.Period, d.s.Goals.SLAP95Ms, d.s.Policy.MinReplicas, d.s.Policy.MaxReplicas, reset,
		bold, d.modelDescription(), reset, dim, reset)
	d.out.header(text)
}

func (d *session) modelDescription() string {
	if d.model.learns() {
		return fmt.Sprintf("ZERO — Q-table zerada, aprende a cada ciclo (epsilon-greedy, epsilon=%.2f); salva em %s",
			d.agent.Epsilon(), d.store.Path)
	}
	return fmt.Sprintf("PRE-TREINADO — Q-table %s, politica gulosa, sem aprender", d.cfg.Learning.QTablePath)
}

// cycle executa um ciclo completo: M → A → P → E → K. Cada etapa recebe a
// mensagem produzida pela anterior (Sintoma → Pedido → Plano → Resultado).
func (d *session) cycle(ctx context.Context, now time.Time) {
	sym := d.mon.Handle(ctx, now)
	d.out.header(fmt.Sprintf("\n%s━━━━━━━━━━ ciclo %d · %s ━━━━━━━━━━%s\n", bold, sym.Cycle, now.Format("15:04:05"), reset))
	printMonitor(d.out.w("monitor"), sym)

	before := d.agent.Table()
	req := d.ana.Handle(sym)
	d.printAnalyzer(d.out.w("analisador"), req)
	d.printLearning(d.out.w("analisador"), before)

	plan := d.pla.Handle(req)
	printPlanner(d.out.w("planejador"), plan, d.s.Policy)

	executed := d.printExecutor(ctx, d.out.w("executor"), plan)
	d.printKnowledge(d.out.w("conhecimento"), sym, executed)
}

// ------------------------------------------------------------------ etapas

func printMonitor(w io.Writer, s knowledge.Symptom) {
	fmt.Fprintf(w, "%s[M] MONITOR%s %s— le os sensores (proxy/Prometheus + API Kubernetes) → normaliza → filtra%s\n", cyan+bold, reset, dim, reset)
	fmt.Fprintf(w, "    latencia p95 = %s   vazao = %.1f req/s   erros = %.1f %%\n", ms(s.P95Ms), s.RPS, s.ErrorPct)
	fmt.Fprintf(w, "    CPU frontend = %s   replicas = %d (prontas %d)\n", pct(s.CPUPct), s.Replicas, s.ReadyReplicas)
	switch {
	case !s.Valid:
		fmt.Fprintf(w, "    dado valido  = %sNAO%s → algum sensor falhou ou deu valor impossivel\n", red, reset)
	case s.RPS == 0:
		fmt.Fprintf(w, "    dado valido  = %ssim%s (sem trafego: p95 indefinido vira 0 pelo filtro FillIdleLatency)\n", green, reset)
	default:
		fmt.Fprintf(w, "    dado valido  = %ssim%s\n", green, reset)
	}
	fmt.Fprintf(w, "    %s→ envia o Sintoma ao Analisador%s\n", dim, reset)
}

func (d *session) printAnalyzer(w io.Writer, r knowledge.ChangeRequest) {
	g := d.s.Goals
	fmt.Fprintf(w, "%s[A] ANALISADOR%s %s— recebeu o Sintoma: classifica, monta o estado, avalia e decide (Q-Learning)%s\n", yellow+bold, reset, dim, reset)
	fmt.Fprintf(w, "    recebido      = p95 %s, %d replica(s), valido=%v\n", ms(r.Symptom.P95Ms), r.Symptom.Replicas, r.Symptom.Valid)
	fmt.Fprintf(w, "    classificacao = %s  %s\n", colorKind(r.Symptom.Kind), explainKind(r.Symptom.Kind, g))
	if r.Reason == knowledge.ReasonInvalidData {
		fmt.Fprintf(w, "    %sdado invalido: nao decide nem aprende neste ciclo%s\n", dim, reset)
		fmt.Fprintf(w, "    %s→ envia ao Planejador um pedido marcado como dado invalido%s\n", dim, reset)
		return
	}
	fmt.Fprintf(w, "    estado        = %s%s%s  (faixa do p95: LOW < %.0f ms <= OK <= %.0f ms < HIGH ; / replicas)\n",
		bold, r.State, reset, g.LowRatio*g.SLAP95Ms, g.SLAP95Ms)
	if !math.IsNaN(r.Reward) {
		fmt.Fprintf(w, "    recompensa    = %+.2f  (nota da acao do ciclo anterior: SLA ok %+.1f / violado %+.1f, −%.1f por replica, −%.1f se oscilar)\n",
			r.Reward, d.s.Reward.SLAOk, d.s.Reward.SLAViolation, d.s.Reward.CostPerReplica, d.s.Reward.OscillationPenalty)
	}
	fmt.Fprintf(w, "    Q(%s)       = %s\n", r.State, d.qRow(r.State))
	fmt.Fprintf(w, "    decisao       = %s%s%+d replica(s)%s  %s\n", bold, yellow, r.Delta, reset, d.decisionReason(r))
	fmt.Fprintf(w, "    %s→ envia o Pedido de mudanca (%+d) ao Planejador%s\n", dim, r.Delta, reset)
}

// decisionReason diz se a acao veio da tabela ou de uma exploracao aleatoria.
func (d *session) decisionReason(r knowledge.ChangeRequest) string {
	if r.Delta != d.agent.Best(r.State) {
		return fmt.Sprintf("(%sexploracao aleatoria%s, epsilon=%.2f: testa uma acao para aprender)", purple, reset, r.Epsilon)
	}
	if d.model.learns() {
		return fmt.Sprintf("(acao valida de maior Q; explora com chance epsilon=%.2f)", r.Epsilon)
	}
	return "(acao valida de maior Q)"
}

// printLearning mostra a celula da Q-table que o aprendizado mudou neste ciclo.
func (d *session) printLearning(w io.Writer, before knowledge.QTable) {
	if !d.model.learns() {
		return
	}
	after := d.agent.Table()
	for i := range after {
		for c := range after[i] {
			if after[i][c] != before[i][c] {
				st := knowledge.StateFromIndex(i, d.s.Policy)
				fmt.Fprintf(w, "    %saprendeu%s      = Q(%s, %+d): %.3f → %.3f\n",
					purple+bold, reset, st, knowledge.Actions()[c], before[i][c], after[i][c])
				return
			}
		}
	}
	fmt.Fprintf(w, "    %saprendeu%s      = nada neste ciclo (sem ciclo anterior valido para avaliar)\n", purple+bold, reset)
}

func printPlanner(w io.Writer, p knowledge.AdaptationPlan, pol knowledge.Policy) {
	fmt.Fprintf(w, "%s[P] PLANEJADOR%s %s— transforma a decisao em plano seguro (limites, reducao 1 a 1, cooldown, pendencia)%s\n", blue+bold, reset, dim, reset)
	fmt.Fprintf(w, "    recebido      = pedido %+d com %d replica(s) atuais\n", p.Request.Delta, p.Fallback)
	switch {
	case p.BlockedBy != "":
		fmt.Fprintf(w, "    resultado     = %sBLOQUEADO (%s)%s: %s\n", red, p.BlockedBy, reset, explainBlock(p.BlockedBy, pol))
	case p.Empty:
		fmt.Fprintf(w, "    resultado     = nada a fazer, mantem %d replica(s)\n", p.Fallback)
	default:
		steps := make([]string, len(p.Steps))
		for i, s := range p.Steps {
			steps[i] = fmt.Sprintf("%s(%d)", s.Kind, s.Replicas)
		}
		fmt.Fprintf(w, "    resultado     = %splano: %d → %d replica(s)%s\n", green, p.Fallback, p.Target, reset)
		fmt.Fprintf(w, "    passos        = %s; prazo %s; plano B = voltar para %d\n", strings.Join(steps, " → "), p.Timeout, p.Fallback)
	}
	fmt.Fprintf(w, "    %s→ envia o Plano ao Executor%s\n", dim, reset)
}

func (d *session) printExecutor(ctx context.Context, w io.Writer, p knowledge.AdaptationPlan) knowledge.Delta {
	fmt.Fprintf(w, "%s[E] EXECUTOR%s %s— recebeu o Plano: allowlist → scale → aguarda prontas → confirma (API Kubernetes)%s\n", green+bold, reset, dim, reset)
	if p.Empty {
		fmt.Fprintf(w, "    plano vazio: nada a executar\n")
	} else {
		fmt.Fprintf(w, "    escalando %s: %d → %d replica(s)... aguardando os pods ficarem prontos\n", d.cfg.TargetName(), p.Fallback, p.Target)
	}
	executed := d.exe.Dispatch(ctx, p)
	if executed != 0 {
		d.printResult(w)
	}
	fmt.Fprintf(w, "    %s→ registra o resultado na Base de Conhecimento%s\n", dim, reset)
	return executed
}

func (d *session) printResult(w io.Writer) {
	if d.hist.last == nil || d.hist.last.Finished == nil {
		return
	}
	f := d.hist.last.Finished
	switch {
	case f.Success:
		fmt.Fprintf(w, "    %sconcluido%s: %+d replica(s), prontas em %.1f s\n", green, reset, f.ExecutedDelta, f.TimeToReady.Seconds())
	case f.RolledBack:
		fmt.Fprintf(w, "    %sfalhou%s: plano B aplicado (voltou ao numero anterior)\n", red, reset)
	default:
		fmt.Fprintf(w, "    %sinterrompido%s (laco encerrado durante a execucao)\n", yellow, reset)
	}
}

func (d *session) printKnowledge(w io.Writer, sym knowledge.Symptom, executed knowledge.Delta) {
	if executed > 0 {
		d.sinceUp = 0
	} else if d.sinceUp < math.MaxInt32 {
		d.sinceUp++
	}
	last, prev := d.k.LastExecuted()
	fmt.Fprintf(w, "%s[K] CONHECIMENTO%s %s— memoria compartilhada pelas etapas (metas, Q-table, historico)%s\n", purple+bold, reset, dim, reset)
	fmt.Fprintf(w, "    ultima acao executada = %+d (anterior %+d)  %s← o Analisador usa para calcular a recompensa%s\n", last, prev, dim, reset)
	if d.sinceUp < math.MaxInt32 {
		fmt.Fprintf(w, "    ciclos desde o ultimo aumento = %d  %s← o Planejador so reduz a partir de %d (cooldown)%s\n",
			d.sinceUp, dim, d.s.Policy.ScaleDownCooldownCycles, reset)
	}
	fmt.Fprintf(w, "    historico: ciclo %d gravado em %s\n", sym.Cycle, d.hist.path)
	if d.model.learns() {
		fmt.Fprintf(w, "    Q-table aprendida: salva em %s a cada %d ciclos e ao parar o laco\n", d.store.Path, d.cfg.Run.EpisodeCycles)
	}
}

// ------------------------------------------------------------------ executor isolado

func (d *session) executorOnly(ctx context.Context, target int) error {
	pol := d.s.Policy
	if target < pol.MinReplicas || target > pol.MaxReplicas {
		return fmt.Errorf("replicas deve estar entre %d e %d", pol.MinReplicas, pol.MaxReplicas)
	}
	has, err := d.effector.HasHPA(ctx)
	if err != nil {
		return err
	}
	if has {
		return errors.New("ha um HPA no frontend; rode `make hpa-off` antes")
	}
	w := os.Stdout
	sym := d.mon.Handle(ctx, time.Now())
	fmt.Fprintf(w, "%sSelfScale-RL — Executor isolado%s\n", bold, reset)
	fmt.Fprintf(w, "%sPlano montado a mao (sem Analisador/Planejador) para ver so a execucao.%s\n", dim, reset)
	fmt.Fprintf(w, "    replicas atuais = %d (prontas %d); alvo = %d; allowlist = %v\n",
		sym.Replicas, sym.ReadyReplicas, target, d.cfg.Target.Allowlist)
	if target == sym.Replicas {
		fmt.Fprintln(w, "    ja esta no alvo; nada a fazer. Use outro numero de replicas.")
		return nil
	}
	plan := knowledge.AdaptationPlan{
		T: time.Now(), Target: target, Fallback: sym.Replicas, Timeout: pol.ReadinessTimeout,
		Steps: []knowledge.Step{
			{Kind: knowledge.StepScale, Replicas: target},
			{Kind: knowledge.StepAwaitReady, Replicas: target},
			{Kind: knowledge.StepConfirm, Replicas: target},
		},
		Request: knowledge.ChangeRequest{Symptom: sym, Delta: knowledge.Delta(target - sym.Replicas)},
	}
	d.printExecutor(ctx, w, plan)
	return nil
}

// ------------------------------------------------------------------ conhecimento

func showKnowledge(w io.Writer, cfg storage.Config, m model) error {
	s := cfg.Settings()
	g, p, r, l := s.Goals, s.Policy, s.Reward, s.Learning
	fmt.Fprintf(w, "%s[K] BASE DE CONHECIMENTO%s %s— o que todas as etapas compartilham%s\n\n", purple+bold, reset, dim, reset)

	fmt.Fprintf(w, "%sMetas e politicas (configs/config.yaml, definidas pelo operador)%s\n", bold, reset)
	fmt.Fprintf(w, "  alvo gerenciado : %s   periodo do ciclo: %s\n", cfg.TargetName(), cfg.Sensors.Period)
	fmt.Fprintf(w, "  SLA             : p95 <= %.0f ms  (LOW < %.0f ms; risk a partir de %.0f ms)\n", g.SLAP95Ms, g.LowRatio*g.SLAP95Ms, g.RiskRatio*g.SLAP95Ms)
	fmt.Fprintf(w, "  replicas        : %d a %d; reduz no maximo %d por vez; cooldown de %d ciclos apos aumentar\n",
		p.MinReplicas, p.MaxReplicas, p.ScaleDownMaxStep, p.ScaleDownCooldownCycles)
	fmt.Fprintf(w, "  recompensa      : SLA ok %+.1f | violado %+.1f | −%.1f por replica | −%.1f se oscilar\n",
		r.SLAOk, r.SLAViolation, r.CostPerReplica, r.OscillationPenalty)
	fmt.Fprintf(w, "  Q-Learning      : alfa=%.2f gama=%.2f\n\n", l.Alpha, l.Gamma)

	agent, err := newAgent(cfg, m)
	if err != nil {
		return err
	}
	if m.learns() {
		fmt.Fprintf(w, "%sModelo ZERO: Q-table zerada (15 estados × 4 acoes)%s\n", bold, reset)
		fmt.Fprintf(w, "  %sNo empate o agente mantem as replicas (acao 0); aprende com a recompensa a cada ciclo%s\n", dim, reset)
		fmt.Fprintf(w, "  %se salva o que aprendeu em %s.%s\n", dim, m.savePath, reset)
	} else {
		fmt.Fprintf(w, "%sModelo PRE-TREINADO: Q-table %s (15 estados × 4 acoes)%s\n", bold, cfg.Learning.QTablePath, reset)
	}
	fmt.Fprintf(w, "  %-8s %8s %8s %8s %8s   melhor\n", "estado", "−1", "0", "+1", "+2")
	for i := 0; i < knowledge.NumStates(p); i++ {
		st := knowledge.StateFromIndex(i, p)
		fmt.Fprintf(w, "  %-8s %s   %s%+d%s\n", st, qCells(agent, st, p), bold, agent.Best(st), reset)
	}
	fmt.Fprintf(w, "  %s(— = acao invalida naquele estado, ex.: reduzir com 1 replica)%s\n", dim, reset)
	return nil
}

func qCells(a *qlearning.Agent, st knowledge.State, p knowledge.Policy) string {
	row := a.Table()[st.Index(p)]
	valid := validSet(a, st)
	cells := make([]string, len(row))
	for c, v := range row {
		cells[c] = fmt.Sprintf("%8.2f", v)
		if !valid[c] {
			cells[c] = fmt.Sprintf("%8s", "—")
		}
	}
	return strings.Join(cells, " ")
}

// ------------------------------------------------------------------ auxiliares

func validSet(a *qlearning.Agent, st knowledge.State) map[int]bool {
	valid := map[int]bool{}
	for _, c := range a.ValidActions(st.Replicas) {
		valid[c] = true
	}
	return valid
}

func (d *session) qRow(st knowledge.State) string {
	row := d.agent.Table()[st.Index(d.s.Policy)]
	valid := validSet(d.agent, st)
	parts := make([]string, len(row))
	for c, a := range knowledge.Actions() {
		v := fmt.Sprintf("%.2f", row[c])
		if !valid[c] {
			v = "—"
		}
		parts[c] = fmt.Sprintf("%+d:%s", a, v)
	}
	return strings.Join(parts, "  ")
}

func colorKind(k knowledge.SymptomKind) string {
	switch k {
	case knowledge.KindViolation, knowledge.KindInvalid:
		return red + bold + string(k) + reset
	case knowledge.KindRisk, knowledge.KindIdle:
		return yellow + bold + string(k) + reset
	default:
		return green + bold + string(k) + reset
	}
}

func explainKind(k knowledge.SymptomKind, g knowledge.Goals) string {
	switch k {
	case knowledge.KindViolation:
		return fmt.Sprintf("(p95 acima do SLA de %.0f ms: cliente mal atendido)", g.SLAP95Ms)
	case knowledge.KindRisk:
		return fmt.Sprintf("(p95 perto do SLA: >= %.0f ms)", g.RiskRatio*g.SLAP95Ms)
	case knowledge.KindIdle:
		return "(latencia baixa com mais de 1 replica: recurso sobrando)"
	case knowledge.KindInvalid:
		return "(dado nao confiavel)"
	default:
		return "(dentro da meta)"
	}
}

func explainBlock(b string, pol knowledge.Policy) string {
	switch b {
	case knowledge.BlockedCooldown:
		return fmt.Sprintf("so reduz %d ciclos depois do ultimo aumento (evita oscilar)", pol.ScaleDownCooldownCycles)
	case knowledge.BlockedPending:
		return "ainda ha replicas subindo ou uma execucao em andamento"
	case knowledge.BlockedBounds:
		return fmt.Sprintf("ja esta no limite (%d a %d replicas)", pol.MinReplicas, pol.MaxReplicas)
	case knowledge.BlockedInvalidData:
		return "dado invalido: nao se atua sem medir direito"
	default:
		return ""
	}
}

func ms(v float64) string {
	if math.IsNaN(v) {
		return "—"
	}
	return fmt.Sprintf("%.1f ms", v)
}

func pct(v float64) string {
	if math.IsNaN(v) {
		return "— (metrics-server indisponivel; nao afeta a decisao)"
	}
	return fmt.Sprintf("%.0f %%", v)
}
