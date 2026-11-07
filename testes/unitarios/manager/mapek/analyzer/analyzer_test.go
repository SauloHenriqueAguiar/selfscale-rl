package analyzer_test

import (
	"context"
	"math"
	"testing"

	"github.com/shna/selfscale-rl/manager/mapek/analyzer"
	"github.com/shna/selfscale-rl/manager/mapek/knowledge"
	"github.com/shna/selfscale-rl/testes/fakes"
)

func newTestAnalyzer(d knowledge.Delta, opts analyzer.Options) (*analyzer.Analyzer, *fakes.Learner, *fakes.Episodes, *knowledge.Base) {
	l, e, k := &fakes.Learner{}, &fakes.Episodes{}, knowledge.New(&fakes.History{})
	return analyzer.New(fakes.Decider{Delta: d}, l, e, k, fakes.Settings(), opts), l, e, k
}

func symptom(p95 float64, replicas int) knowledge.Symptom {
	return knowledge.Symptom{Valid: true, P95Ms: p95, RPS: 10, Replicas: replicas, ReadyReplicas: replicas}
}

func TestAnalyzer_InvalidSymptomYieldsNoChangeAndNoLearning(t *testing.T) {
	a, l, _, _ := newTestAnalyzer(+1, analyzer.Options{Learn: true, Act: true})
	a.Handle(symptom(300, 1))
	req := a.Handle(knowledge.Symptom{Valid: false})

	if req.Delta != 0 || req.Reason != knowledge.ReasonInvalidData || req.Symptom.Kind != knowledge.KindInvalid {
		t.Errorf("req = %+v", req)
	}
	a.Handle(symptom(300, 1)) // após inválido, a cadeia recomeça: ainda sem aprender
	if len(l.Calls) != 0 {
		t.Errorf("não deveria aprender: %v", l.Calls)
	}
}

func TestAnalyzer_RewardUsesExecutedDeltaFromKnowledge(t *testing.T) {
	a, l, _, k := newTestAnalyzer(+2, analyzer.Options{Learn: true, Act: true})
	if first := a.Handle(symptom(300, 1)); !math.IsNaN(first.Reward) {
		t.Error("primeiro ciclo não tem recompensa")
	}
	k.RecordDispatch(+1) // o Planejador limitou/alterou a decisão de +2

	req := a.Handle(symptom(170, 2))

	if len(l.Calls) != 1 {
		t.Fatalf("esperava 1 atualização, veio %d", len(l.Calls))
	}
	c := l.Calls[0]
	wantS, wantNext := knowledge.State{Band: knowledge.BandHigh, Replicas: 1}, knowledge.State{Band: knowledge.BandOK, Replicas: 2}
	if c.Executed != +1 || c.S != wantS || c.Next != wantNext {
		t.Errorf("Learn = %+v", c)
	}
	if math.Abs(c.Reward-0.8) > 1e-9 || req.Reward != c.Reward {
		t.Errorf("recompensa = %v, quer 0.8", c.Reward)
	}
	if req.Delta != +2 || req.Reason != knowledge.ReasonPolicy || req.Symptom.Kind != knowledge.KindRisk {
		t.Errorf("req = %+v", req)
	}
}

func TestAnalyzer_ObserveModeOnlyClassifies(t *testing.T) {
	a, l, _, _ := newTestAnalyzer(+1, analyzer.Options{})
	a.Handle(symptom(300, 1))
	req := a.Handle(symptom(300, 1))
	if req.Delta != 0 || req.Reason != knowledge.ReasonObserve || len(l.Calls) != 0 {
		t.Errorf("observe não deve decidir nem aprender: %+v", req)
	}
	if math.IsNaN(req.Reward) {
		t.Error("observe ainda calcula a recompensa (para comparar estratégias)")
	}
}

func TestAnalyzer_EndsEpisodeEveryNCycles(t *testing.T) {
	a, _, e, _ := newTestAnalyzer(0, analyzer.Options{Learn: true, Act: true, EpisodeCycles: 3})
	for range 7 {
		a.Handle(symptom(120, 1))
	}
	if e.Ended != 2 {
		t.Errorf("episódios = %d, quer 2", e.Ended)
	}
}

func TestAnalyzer_RunForwardsRequestsUntilCancelled(t *testing.T) {
	a, _, _, _ := newTestAnalyzer(+1, analyzer.Options{Act: true})
	in, out := make(chan knowledge.Symptom), make(chan knowledge.ChangeRequest)
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
