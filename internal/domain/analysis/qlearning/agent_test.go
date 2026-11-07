package qlearning

import (
	"math"
	"testing"

	"github.com/shna/selfscale-rl/internal/domain"
)

var (
	testPolicy = domain.Policy{MinReplicas: 1, MaxReplicas: 5}
	testParams = domain.LearningParams{Alpha: 0.1, Gamma: 0.9}
)

func state(b domain.LatencyBand, r int) domain.State { return domain.State{Band: b, Replicas: r} }

func deltas(a *Agent, cols []int) []domain.Delta {
	out := make([]domain.Delta, len(cols))
	for i, c := range cols {
		out[i] = a.actions[c]
	}
	return out
}

func TestValidActions_Mask(t *testing.T) {
	a := NewAgent(testPolicy, testParams, Greedy{})
	cases := []struct {
		replicas int
		want     []domain.Delta
	}{
		{1, []domain.Delta{0, 1, 2}},
		{3, []domain.Delta{-1, 0, 1, 2}},
		{4, []domain.Delta{-1, 0, 1}},
		{5, []domain.Delta{-1, 0}},
	}
	for _, c := range cases {
		got := deltas(a, a.ValidActions(c.replicas))
		if len(got) != len(c.want) {
			t.Fatalf("réplicas=%d: %v, quer %v", c.replicas, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("réplicas=%d: %v, quer %v", c.replicas, got, c.want)
			}
		}
	}
}

func TestLearn_MatchesFormula(t *testing.T) {
	a := NewAgent(testPolicy, testParams, Greedy{})
	s, next := state(domain.BandHigh, 2), state(domain.BandOK, 3)
	a.q[next.Index(testPolicy)][a.column(0)] = 1.0 // max Q(s′) = 1

	a.Learn(s, +1, 0.7, next)

	want := 0.1 * (0.7 + 0.9*1.0) // Q inicial = 0
	if got := a.q[s.Index(testPolicy)][a.column(+1)]; math.Abs(got-want) > 1e-9 {
		t.Errorf("Q = %v, quer %v", got, want)
	}
}

func TestLearn_MaxOnlyOverValidActions(t *testing.T) {
	a := NewAgent(testPolicy, testParams, Greedy{})
	next := state(domain.BandOK, 5)
	a.q[next.Index(testPolicy)][a.column(+2)] = 100 // inválida com 5 réplicas
	a.Learn(state(domain.BandOK, 4), +1, 0, next)
	if got := a.q[state(domain.BandOK, 4).Index(testPolicy)][a.column(+1)]; got != 0 {
		t.Errorf("Q = %v, quer 0 (ação inválida não entra no max)", got)
	}
}

func TestLearn_IgnoresUnknownDelta(t *testing.T) {
	a := NewAgent(testPolicy, testParams, Greedy{})
	a.Learn(state(domain.BandOK, 1), +3, 10, state(domain.BandOK, 4))
	for _, row := range a.q {
		for _, v := range row {
			if v != 0 {
				t.Fatal("Q-table não deveria mudar")
			}
		}
	}
}

func TestDecide_GreedyPicksBestAndTieBreaksBySmallestChange(t *testing.T) {
	a := NewAgent(testPolicy, testParams, Greedy{})
	s := state(domain.BandOK, 3)
	if d := a.Decide(s); d != 0 {
		t.Errorf("empate deve manter (Δ=0), veio %d", d)
	}
	a.q[s.Index(testPolicy)][a.column(+2)] = 1
	if d := a.Decide(s); d != +2 {
		t.Errorf("Decide = %d, quer +2", d)
	}
}

func TestDecide_EpsilonZeroIsGreedy(t *testing.T) {
	a := NewAgent(testPolicy, testParams, NewEpsilonGreedy(0, 0, 1, 42))
	s := state(domain.BandLow, 4)
	a.q[s.Index(testPolicy)][a.column(-1)] = 0.9
	for range 50 {
		if d := a.Decide(s); d != -1 {
			t.Fatalf("Decide = %d, quer -1", d)
		}
	}
}

func TestDecide_ExplorationOnlyPicksValidActions(t *testing.T) {
	a := NewAgent(testPolicy, testParams, NewEpsilonGreedy(1, 1, 1, 42))
	for range 200 {
		if d := a.Decide(state(domain.BandOK, 5)); d > 0 {
			t.Fatalf("ação inválida %d com 5 réplicas", d)
		}
	}
}

func TestEpsilonGreedy_Decay(t *testing.T) {
	e := NewEpsilonGreedy(1, 0.05, 0.5, 1)
	e.EndEpisode()
	if e.Epsilon() != 0.5 {
		t.Errorf("ε = %v, quer 0.5", e.Epsilon())
	}
	for range 10 {
		e.EndEpisode()
	}
	if e.Epsilon() != 0.05 {
		t.Errorf("ε = %v, quer o mínimo 0.05", e.Epsilon())
	}
}

func TestNewAgent_PriorKnowledge(t *testing.T) {
	a := NewAgent(testPolicy, domain.LearningParams{PriorInit: true}, Greedy{})
	if d := a.Best(state(domain.BandHigh, 1)); d != +1 {
		t.Errorf("HIGH/1 → %d, quer +1 (empate +1/+2 resolve pelo menor)", d)
	}
	if d := a.Best(state(domain.BandLow, 3)); d != -1 {
		t.Errorf("LOW/3 → %d, quer -1", d)
	}
	if d := a.Best(state(domain.BandLow, 1)); d != 0 {
		t.Errorf("LOW/1 → %d, quer 0", d)
	}
	if a.Epsilon() != 0 {
		t.Error("Greedy deve ter ε = 0")
	}
	a.EndEpisode() // sem efeito no Greedy
}

func TestTable_CopyAndSet(t *testing.T) {
	a := NewAgent(testPolicy, testParams, Greedy{})
	tbl := a.Table()
	tbl[0][0] = 5
	if a.q[0][0] != 0 {
		t.Error("Table deve devolver cópia")
	}
	if err := a.SetTable(tbl); err != nil || a.q[0][0] != 5 {
		t.Errorf("SetTable falhou: %v", err)
	}
	if err := a.SetTable(Table{{1}}); err == nil {
		t.Error("esperava erro de dimensão (estados)")
	}
	bad := a.Table()
	bad[3] = []float64{1}
	if err := a.SetTable(bad); err == nil {
		t.Error("esperava erro de dimensão (ações)")
	}
}
