package mapek

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shna/selfscale-rl/internal/domain"
	"github.com/shna/selfscale-rl/internal/mapek/analyzer"
	"github.com/shna/selfscale-rl/internal/mapek/executor"
	"github.com/shna/selfscale-rl/internal/mapek/knowledge"
	"github.com/shna/selfscale-rl/internal/mapek/mapektest"
	"github.com/shna/selfscale-rl/internal/mapek/monitor"
	"github.com/shna/selfscale-rl/internal/mapek/planner"
)

const target = "boutique/frontend"

// newTestLoop monta o laço completo com fakes; decide devolve sempre o mesmo Δ.
func newTestLoop(c *mapektest.Cluster, traffic mapektest.Traffic, decider *mapektest.Decider, async bool) (*Loop, *mapektest.History) {
	h := &mapektest.History{}
	k := knowledge.New(h)
	s := mapektest.Settings()
	opts := executor.Options{Target: target, Allowlist: []string{target}, PollInterval: time.Millisecond, Async: async}
	return &Loop{
		Monitor:  monitor.New(traffic, mapektest.CPU{Pct: 50}, c, monitor.DefaultProcessors()),
		Analyzer: analyzer.New(decider, &mapektest.Learner{}, &mapektest.Episodes{}, k, s, analyzer.Options{Act: true}),
		Planner:  planner.New(k, s.Policy),
		Executor: executor.New(c, c, k, opts),
	}, h
}

func TestLoop_StepRunsOneCycleEndToEnd(t *testing.T) {
	c := &mapektest.Cluster{Desired: 1, Ready: 1, InstantReady: true}
	l, h := newTestLoop(c, mapektest.Traffic{T: domain.Traffic{P95Seconds: 0.4, RPS: 50}}, &mapektest.Decider{Delta: +1}, false)

	plan := l.Step(context.Background(), time.Now())

	if plan.Target != 2 || c.DesiredReplicas() != 2 || h.Count() != 1 {
		t.Errorf("plano=%+v réplicas=%d registros=%d", plan, c.DesiredReplicas(), h.Count())
	}
	if kind := h.Records()[0].Plan.Request.Symptom.Kind; kind != domain.KindViolation {
		t.Errorf("sintoma = %v", kind)
	}
}

func TestLoop_ScaleDownRespectsCooldown(t *testing.T) {
	c := &mapektest.Cluster{Desired: 1, Ready: 1, InstantReady: true}
	decider := &mapektest.Decider{Delta: +1}
	l, _ := newTestLoop(c, mapektest.Traffic{T: domain.Traffic{P95Seconds: 0.4, RPS: 50}}, decider, false)
	l.Step(context.Background(), time.Now()) // aumenta para 2

	decider.Delta = -1
	var blocked []string
	for range 3 {
		blocked = append(blocked, l.Step(context.Background(), time.Now()).BlockedBy)
	}
	want := []string{domain.BlockedCooldown, "", domain.BlockedBounds}
	for i := range want {
		if blocked[i] != want[i] {
			t.Fatalf("bloqueios = %q, quer %q", blocked, want)
		}
	}
	if c.DesiredReplicas() != 1 {
		t.Errorf("réplicas = %d, quer 1", c.DesiredReplicas())
	}
}

func TestLoop_RunWithTicksProcessesCyclesAndStops(t *testing.T) {
	c := &mapektest.Cluster{Desired: 1, Ready: 1, InstantReady: true}
	l, h := newTestLoop(c, mapektest.Traffic{T: domain.Traffic{P95Seconds: 0.05, RPS: 5}}, &mapektest.Decider{}, true)
	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); l.RunWithTicks(ctx, ticks) }()
	ticks <- time.Now()
	ticks <- time.Now()
	mapektest.WaitUntil(t, func() bool { return h.Count() >= 2 })
	cancel()
	wg.Wait()
}
