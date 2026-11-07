// Package executor é a etapa E do MAPE-K ("fazer"): autoriza o plano, executa
// os passos (Scale → AwaitReady → Confirm), aplica o plano B em caso de falha
// e registra o resultado na Base de Conhecimento.
//
// Arquivos: ports.go (efetuadores), steps.go (passos no padrão Command) e
// executor.go (Engine de Workflow).
package executor

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/shna/selfscale-rl/manager/mapek/knowledge"
)

// Options configura o Executor.
type Options struct {
	Target       string   // "namespace/deployment" gerenciado
	Allowlist    []string // alvos que o Executor tem autorização para escalar
	PollInterval time.Duration
	// Async executa o workflow em segundo plano para não bloquear o laço.
	// O simulador usa false: lá o tempo só avança entre ciclos.
	Async bool
}

// Executor é o Executor.
type Executor struct {
	scaler Scaler
	reader ReplicaReader
	k      *knowledge.Base
	opts   Options
	now    func() time.Time
}

// New cria o Executor.
func New(s Scaler, r ReplicaReader, k *knowledge.Base, o Options) *Executor {
	return &Executor{scaler: s, reader: r, k: k, opts: o, now: time.Now}
}

// Dispatch inicia a execução do plano e grava o registro do ciclo.
// Devolve a variação de réplicas efetivamente disparada (0 se nada foi feito).
func (e *Executor) Dispatch(ctx context.Context, plan knowledge.AdaptationPlan) knowledge.Delta {
	executed := knowledge.Delta(0)
	switch {
	case plan.Empty:
	case !e.authorized():
		slog.Error("alvo fora da allowlist; plano recusado", "target", e.opts.Target)
	default:
		executed = knowledge.Delta(plan.Target - plan.Fallback)
	}

	e.k.RecordDispatch(executed)
	if executed != 0 {
		e.start(ctx, plan)
	}
	e.k.RecordCycle(plan, executed)
	return executed
}

// authorized é o Interpretador de Ações: só escala o que está na allowlist.
func (e *Executor) authorized() bool {
	return slices.Contains(e.opts.Allowlist, e.opts.Target)
}

func (e *Executor) start(ctx context.Context, plan knowledge.AdaptationPlan) {
	run := func() { e.k.FinishExecution(e.Execute(ctx, plan)) }
	if e.opts.Async {
		go run()
		return
	}
	run()
}

// Execute roda os passos do plano dentro do timeout; em falha, aplica o plano B
// (volta às réplicas anteriores, a última configuração válida).
func (e *Executor) Execute(ctx context.Context, plan knowledge.AdaptationPlan) knowledge.ExecutionResult {
	start := e.now()
	res := knowledge.ExecutionResult{T: start, ExecutedDelta: knowledge.Delta(plan.Target - plan.Fallback)}

	if err := e.runSteps(ctx, plan); err != nil {
		if ctx.Err() != nil { // encerramento do gerenciador: não é falha da adaptação
			return res
		}
		slog.Warn("execução falhou; aplicando plano B", "err", err, "fallback", plan.Fallback)
		res.RolledBack = true
		if err := e.scaler.Scale(ctx, plan.Fallback); err != nil {
			slog.Error("plano B falhou", "err", err)
		}
		return res
	}
	res.Success = true
	res.TimeToReady = e.now().Sub(start)
	slog.Info("adaptação concluída", "target", plan.Target, "time_to_ready", res.TimeToReady)
	return res
}

func (e *Executor) runSteps(ctx context.Context, plan knowledge.AdaptationPlan) error {
	cmds, err := e.buildCommands(plan.Steps)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, plan.Timeout)
	defer cancel()
	for _, c := range cmds {
		if err := c.Execute(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Run executa os planos recebidos em in (chPE) até ctx terminar.
func (e *Executor) Run(ctx context.Context, in <-chan knowledge.AdaptationPlan) {
	for {
		select {
		case <-ctx.Done():
			return
		case p := <-in:
			e.Dispatch(ctx, p)
		}
	}
}
