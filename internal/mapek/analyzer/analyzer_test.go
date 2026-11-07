package analyzer

import (
	"context"
	"math"
	"testing"

	"github.com/shna/selfscale-rl/internal/domain"
	"github.com/shna/selfscale-rl/internal/mapek/knowledge"
	"github.com/shna/selfscale-rl/internal/mapek/mapektest"
)

func newTestAnalyzer(d domain.Delta, opts Options) (*Analyzer, *mapektest.Learner, *mapektest.Episodes, *knowledge.Base) {
	l, e, k := &mapektest.Learner{}, &mapektest.Episodes{}, knowledge.New(&mapektest.History{})
	return New(mapektest.Decider{Delta: d}, l, e, k, mapektest.Settings(), opts), l, e, k
}

func symptom(p95 float64, replicas int) domain.Symptom {
	return domain.Symptom{Valid: true, P95Ms: p95, RPS: 10, Replicas: replicas, ReadyReplicas: replicas}
}

func TestAnalyzer_InvalidSymptomYieldsNoChangeAndNoLearning(t *testing.T) {
	a, l, _, _ := newTestAnalyzer(+1, Options{Learn: true, Act: true})
	a.Handle(symptom(300, 1))
	req := a.Handle(domain.Symptom{Valid: false})

	if req.Delta != 0 || req.Reason != domain.ReasonInvalidData || req.Symptom.Kind != domain.KindInvalid {
		t.Errorf("req = %+v", req)
	}
	a.Handle(symptom(300, 1)) // após inválido, a cadeia recomeça: ainda sem aprender
	if len(l.Calls) != 0 {
		t.Errorf("não deveria aprender: %v", l.Calls)
	}
}

func TestAnalyzer_RewardUsesExecutedDeltaFromKnowledge(t *testing.T) {
	a, l, _, k := newTestAnalyzer(+2, Options{Learn: true, Act: true})
	if first := a.Handle(symptom(300, 1)); !math.IsNaN(first.Reward) {
		t.Error("primeiro ciclo não tem recompensa")
	}
	k.RecordDispatch(+1) // o Planejador limitou/alterou a decisão de +2

	req := a.Handle(symptom(170, 2))

	if len(l.Calls) != 1 {
		t.Fatalf("esperava 1 atualização, veio %d", len(l.Calls))
	}
	c := l.Calls[0]
	wantS, wantNext := domain.State{Band: domain.BandHigh, Replicas: 1}, domain.State{Band: domain.BandOK, Replicas: 2}
	if c.Executed != +1 || c.S != wantS || c.Next != wantNext {
		t.Errorf("Learn = %+v", c)
	}
	if math.Abs(c.Reward-0.8) > 1e-9 || req.Reward != c.Reward {
		t.Errorf("recompensa = %v, quer 0.8", c.Reward)
	}
	if req.Delta != +2 || req.Reason != domain.ReasonPolicy || req.Symptom.Kind != domain.KindRisk {
		t.Errorf("req = %+v", req)
	}
}

func TestAnalyzer_ObserveModeOnlyClassifies(t *testing.T) {
	a, l, _, _ := newTestAnalyzer(+1, Options{})
	a.Handle(symptom(300, 1))
	req := a.Handle(symptom(300, 1))
	if req.Delta != 0 || req.Reason != domain.ReasonObserve || len(l.Calls) != 0 {
		t.Errorf("observe não deve decidir nem aprender: %+v", req)
	}
	if math.IsNaN(req.Reward) {
		t.Error("observe ainda calcula a recompensa (para comparar estratégias)")
	}
}

func TestAnalyzer_EndsEpisodeEveryNCycles(t *testing.T) {
	a, _, e, _ := newTestAnalyzer(0, Options{Learn: true, Act: true, EpisodeCycles: 3})
	for range 7 {
		a.Handle(symptom(120, 1))
	}
	if e.Ended != 2 {
		t.Errorf("episódios = %d, quer 2", e.Ended)
	}
}

func TestAnalyzer_RunForwardsRequestsUntilCancelled(t *testing.T) {
	a, _, _, _ := newTestAnalyzer(+1, Options{Act: true})
	in, out := make(chan domain.Symptom), make(chan domain.ChangeRequest)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { a.Run(ctx, in, out); close(done) }()

	in <- symptom(300, 1)
	if req := <-out; req.Delta != +1 {
		t.Errorf("Delta = %d", req.Delta)
	}
	in <- symptom(300, 1) // ninguém lê a saída: o cancelamento deve destravar
	cancel()
	<-done
}
