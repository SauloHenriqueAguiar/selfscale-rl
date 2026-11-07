// Package mapek monta o laço MAPE-K: liga as quatro etapas (cada uma em seu
// subpacote: monitor, analyzer, planner, executor) por goroutines e canais,
// todas compartilhando a Base de Conhecimento (knowledge).
package mapek

import (
	"context"
	"sync"
	"time"

	"github.com/shna/selfscale-rl/manager/mapek/analyzer"
	"github.com/shna/selfscale-rl/manager/mapek/executor"
	"github.com/shna/selfscale-rl/manager/mapek/knowledge"
	"github.com/shna/selfscale-rl/manager/mapek/monitor"
	"github.com/shna/selfscale-rl/manager/mapek/planner"
)

// Loop liga as etapas por canais:
//
//	Monitor --chMA--> Analyzer --chAP--> Planner --chPE--> Executor
type Loop struct {
	Monitor  *monitor.Monitor
	Analyzer *analyzer.Analyzer
	Planner  *planner.Planner
	Executor *executor.Executor
}

// Run inicia o laço com um ticker do período informado e bloqueia até ctx terminar.
func (l *Loop) Run(ctx context.Context, period time.Duration) {
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	l.RunWithTicks(ctx, ticker.C)
}

// RunWithTicks é o Run com a fonte de ticks injetada (testável sem relógio real).
func (l *Loop) RunWithTicks(ctx context.Context, ticks <-chan time.Time) {
	chMA := make(chan knowledge.Symptom)
	chAP := make(chan knowledge.ChangeRequest)
	chPE := make(chan knowledge.AdaptationPlan)

	var wg sync.WaitGroup
	wg.Add(4)
	go func() { defer wg.Done(); l.Monitor.Run(ctx, ticks, chMA) }()
	go func() { defer wg.Done(); l.Analyzer.Run(ctx, chMA, chAP) }()
	go func() { defer wg.Done(); l.Planner.Run(ctx, chAP, chPE) }()
	go func() { defer wg.Done(); l.Executor.Run(ctx, chPE) }()
	wg.Wait()
}

// Step executa um ciclo completo de forma síncrona (usado pelo simulador).
func (l *Loop) Step(ctx context.Context, now time.Time) knowledge.AdaptationPlan {
	plan := l.Planner.Handle(l.Analyzer.Handle(l.Monitor.Handle(ctx, now)))
	l.Executor.Dispatch(ctx, plan)
	return plan
}
