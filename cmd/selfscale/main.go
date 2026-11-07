// Comando selfscale: gerenciador MAPE-K que ajusta as réplicas do frontend.
// Este arquivo é o composition root: só monta e liga os componentes.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"

	effectors "github.com/shna/selfscale-rl/manager/effectors/kube"
	"github.com/shna/selfscale-rl/manager/learning/qlearning"
	"github.com/shna/selfscale-rl/manager/mapek"
	"github.com/shna/selfscale-rl/manager/mapek/analyzer"
	"github.com/shna/selfscale-rl/manager/mapek/executor"
	"github.com/shna/selfscale-rl/manager/mapek/knowledge"
	"github.com/shna/selfscale-rl/manager/mapek/monitor"
	"github.com/shna/selfscale-rl/manager/mapek/planner"
	sensors "github.com/shna/selfscale-rl/manager/sensors/kube"
	"github.com/shna/selfscale-rl/manager/sensors/prometheus"
	"github.com/shna/selfscale-rl/manager/storage"
)

// Intervalo de consulta da prontidão das réplicas pelo Executor.
const readinessPoll = 2 * time.Second

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "arquivo de configuração")
	mode := flag.String("mode", "observe", "observe | finetune | eval | online")
	scenario := flag.String("scenario", "none", "rótulo do cenário de carga (C0..C5)")
	history := flag.String("history", "", "arquivo do histórico (padrão: <history_dir>/history_<mode>_<scenario>_<hora>.csv)")
	flag.Parse()

	if err := run(*cfgPath, *mode, *scenario, *history); err != nil {
		slog.Error("selfscale encerrado com erro", "err", err)
		os.Exit(1)
	}
}

func run(cfgPath, modeName, scenario, historyPath string) error {
	cfg, err := storage.LoadConfig(cfgPath)
	if err != nil {
		return err
	}
	mode, err := mapek.ParseMode(modeName)
	if err != nil {
		return err
	}
	kube, err := newKubeTouchpoints(cfg)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := ensureNoHPA(ctx, kube.effector, mode); err != nil {
		return err
	}
	agent, store, err := newAgent(cfg, mode)
	if err != nil {
		return err
	}
	hist, err := storage.NewCSVHistory(historyFile(cfg, historyPath, modeName, scenario), modeName, scenario)
	if err != nil {
		return err
	}
	defer hist.Close()

	loop := buildLoop(cfg, mode, kube, agent, store, hist)
	slog.Info("iniciando laço MAPE-K", "mode", modeName, "scenario", scenario, "period", cfg.Sensors.Period)
	loop.Run(ctx, cfg.Sensors.Period)

	if mode.Learn {
		return knowledge.Checkpointer{Agent: agent, Store: store}.Save()
	}
	return nil
}

// kubeTouchpoints são os pontos de contato com o Kubernetes: o sensor lê o
// Deployment gerenciado e o efetuador o escala.
type kubeTouchpoints struct {
	sensor   *sensors.Sensor
	effector *effectors.Effector
}

func newKubeTouchpoints(cfg storage.Config) (kubeTouchpoints, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{CurrentContext: cfg.Target.KubeconfigContext}
	restCfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	if err != nil {
		return kubeTouchpoints{}, fmt.Errorf("carregando kubeconfig: %w", err)
	}
	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return kubeTouchpoints{}, fmt.Errorf("criando cliente kubernetes: %w", err)
	}
	metrics, err := metricsclient.NewForConfig(restCfg)
	if err != nil {
		return kubeTouchpoints{}, fmt.Errorf("criando cliente de métricas: %w", err)
	}
	t := cfg.Target
	return kubeTouchpoints{
		sensor:   sensors.NewSensor(client, metrics, t.Namespace, t.Deployment, t.PodSelector),
		effector: effectors.NewEffector(client, t.Namespace, t.Deployment),
	}, nil
}

// ensureNoHPA impede que HPA e SelfScale-RL atuem no mesmo Deployment.
func ensureNoHPA(ctx context.Context, effector *effectors.Effector, mode mapek.Mode) error {
	if !mode.Act {
		return nil
	}
	has, err := effector.HasHPA(ctx)
	if err != nil {
		return err
	}
	if has {
		return errors.New("há um HPA no frontend; rode `make hpa-off` antes de atuar")
	}
	return nil
}

// newAgent cria o agente com a estratégia de seleção do modo e carrega a Q-table salva.
func newAgent(cfg storage.Config, mode mapek.Mode) (*qlearning.Agent, storage.JSONQTable, error) {
	s, l := cfg.Settings(), cfg.Learning
	var sel qlearning.ActionSelector = qlearning.Greedy{}
	switch {
	case mode.Learn:
		sel = qlearning.NewEpsilonGreedy(l.EpsilonFinetune, l.EpsilonMin, l.EpsilonDecay, l.Seed)
	case mode.Act:
		sel = qlearning.NewEpsilonGreedy(l.EpsilonEval, l.EpsilonEval, 1, l.Seed)
	}
	agent := qlearning.NewAgent(s.Policy, s.Learning, sel)
	store := storage.JSONQTable{Path: l.QTablePath}

	table, err := store.Load()
	switch {
	case err == nil:
		slog.Info("q-table carregada", "path", l.QTablePath)
		return agent, store, agent.SetTable(table)
	case errors.Is(err, os.ErrNotExist) && mode.Name != "eval":
		slog.Warn("q-table não encontrada; começando do conhecimento prévio", "path", l.QTablePath)
		return agent, store, nil
	default:
		return nil, store, err
	}
}

func historyFile(cfg storage.Config, override, mode, scenario string) string {
	if override != "" {
		return override
	}
	name := fmt.Sprintf("history_%s_%s_%s.csv", mode, scenario, time.Now().Format("20060102-150405"))
	return filepath.Join(cfg.Run.HistoryDir, name)
}

func buildLoop(cfg storage.Config, mode mapek.Mode, kube kubeTouchpoints, agent *qlearning.Agent,
	store storage.JSONQTable, hist knowledge.HistoryWriter) *mapek.Loop {
	s := cfg.Settings()
	k := knowledge.New(hist)
	traffic := prometheus.NewSource(cfg.Sensors.PrometheusURL, cfg.Sensors.Period, &http.Client{Timeout: 5 * time.Second})
	analyzerOpts := analyzer.Options{Learn: mode.Learn, Act: mode.Act, EpisodeCycles: cfg.Run.EpisodeCycles}
	execOpts := executor.Options{
		Target: cfg.TargetName(), Allowlist: cfg.Target.Allowlist, PollInterval: readinessPoll, Async: true,
	}
	return &mapek.Loop{
		Monitor:  monitor.New(traffic, kube.sensor, kube.sensor, monitor.DefaultProcessors()),
		Analyzer: analyzer.New(agent, agent, knowledge.Checkpointer{Agent: agent, Store: store}, k, s, analyzerOpts),
		Planner:  planner.New(k, s.Policy),
		Executor: executor.New(kube.effector, kube.sensor, k, execOpts),
	}
}
