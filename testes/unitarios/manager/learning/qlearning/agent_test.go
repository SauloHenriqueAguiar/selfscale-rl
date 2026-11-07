package qlearning_test

import (
	"math"
	"testing"

	"github.com/shna/selfscale-rl/manager/learning/qlearning"
	"github.com/shna/selfscale-rl/manager/mapek/knowledge"
)

var (
	testPolicy = knowledge.Policy{MinReplicas: 1, MaxReplicas: 5}
	testParams = knowledge.LearningParams{Alpha: 0.1, Gamma: 0.9}
)

func state(b knowledge.LatencyBand, r int) knowledge.State {
	return knowledge.State{Band: b, Replicas: r}
}

func deltas(cols []int) []knowledge.Delta {
	out := make([]knowledge.Delta, len(cols))
	for i, c := range cols {
		out[i] = knowledge.Actions()[c]
	}
	return out
}

// col devolve a coluna da Q-table correspondente a Δ.
func col(d knowledge.Delta) int {
	for i, x := range knowledge.Actions() {
		if x == d {
			return i
		}
	}
	panic("ação desconhecida")
}

// setQ e getQ acessam a Q-table pela API pública do agente.
func setQ(t *testing.T, a *qlearning.Agent, s knowledge.State, d knowledge.Delta, v float64) {
	t.Helper()
	tbl := a.Table()
	tbl[s.Index(testPolicy)][col(d)] = v
	if err := a.SetTable(tbl); err != nil {
		t.Fatal(err)
	}
}

func getQ(a *qlearning.Agent, s knowledge.State, d knowledge.Delta) float64 {
	return a.Table()[s.Index(testPolicy)][col(d)]
}

func TestValidActions_Mask(t *testing.T) {
	a := qlearning.NewAgent(testPolicy, testParams, qlearning.Greedy{})
	cases := []struct {
		replicas int
		want     []knowledge.Delta
	}{
		{1, []knowledge.Delta{0, 1, 2}},
		{3, []knowledge.Delta{-1, 0, 1, 2}},
		{4, []knowledge.Delta{-1, 0, 1}},
		{5, []knowledge.Delta{-1, 0}},
	}
	for _, c := range cases {
		got := deltas(a.ValidActions(c.replicas))
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
	a := qlearning.NewAgent(testPolicy, testParams, qlearning.Greedy{})
	s, next := state(knowledge.BandHigh, 2), state(knowledge.BandOK, 3)
	setQ(t, a, next, 0, 1.0) // max Q(s′) = 1

	a.Learn(s, +1, 0.7, next)

	want := 0.1 * (0.7 + 0.9*1.0) // Q inicial = 0
	if got := getQ(a, s, +1); math.Abs(got-want) > 1e-9 {
		t.Errorf("Q = %v, quer %v", got, want)
	}
}

func TestLearn_MaxOnlyOverValidActions(t *testing.T) {
	a := qlearning.NewAgent(testPolicy, testParams, qlearning.Greedy{})
	next := state(knowledge.BandOK, 5)
	setQ(t, a, next, +2, 100) // inválida com 5 réplicas
	a.Learn(state(knowledge.BandOK, 4), +1, 0, next)
	if got := getQ(a, state(knowledge.BandOK, 4), +1); got != 0 {
		t.Errorf("Q = %v, quer 0 (ação inválida não entra no max)", got)
	}
}

func TestLearn_IgnoresUnknownDelta(t *testing.T) {
	a := qlearning.NewAgent(testPolicy, testParams, qlearning.Greedy{})
	a.Learn(state(knowledge.BandOK, 1), +3, 10, state(knowledge.BandOK, 4))
	for _, row := range a.Table() {
		for _, v := range row {
			if v != 0 {
				t.Fatal("Q-table não deveria mudar")
			}
		}
	}
}

func TestDecide_GreedyPicksBestAndTieBreaksBySmallestChange(t *testing.T) {
	a := qlearning.NewAgent(testPolicy, testParams, qlearning.Greedy{})
	s := state(knowledge.BandOK, 3)
	if d := a.Decide(s); d != 0 {
		t.Errorf("empate deve manter (Δ=0), veio %d", d)
	}
	setQ(t, a, s, +2, 1)
	if d := a.Decide(s); d != +2 {
		t.Errorf("Decide = %d, quer +2", d)
	}
}

func TestDecide_EpsilonZeroIsGreedy(t *testing.T) {
	a := qlearning.NewAgent(testPolicy, testParams, qlearning.NewEpsilonGreedy(0, 0, 1, 42))
	s := state(knowledge.BandLow, 4)
	setQ(t, a, s, -1, 0.9)
	for range 50 {
		if d := a.Decide(s); d != -1 {
			t.Fatalf("Decide = %d, quer -1", d)
		}
	}
}

func TestDecide_ExplorationOnlyPicksValidActions(t *testing.T) {
	a := qlearning.NewAgent(testPolicy, testParams, qlearning.NewEpsilonGreedy(1, 1, 1, 42))
	for range 200 {
		if d := a.Decide(state(knowledge.BandOK, 5)); d > 0 {
			t.Fatalf("ação inválida %d com 5 réplicas", d)
		}
	}
}

func TestEpsilonGreedy_Decay(t *testing.T) {
	e := qlearning.NewEpsilonGreedy(1, 0.05, 0.5, 1)
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
	a := qlearning.NewAgent(testPolicy, knowledge.LearningParams{PriorInit: true}, qlearning.Greedy{})
	if d := a.Best(state(knowledge.BandHigh, 1)); d != +1 {
		t.Errorf("HIGH/1 → %d, quer +1 (empate +1/+2 resolve pelo menor)", d)
	}
	if d := a.Best(state(knowledge.BandLow, 3)); d != -1 {
		t.Errorf("LOW/3 → %d, quer -1", d)
	}
	if d := a.Best(state(knowledge.BandLow, 1)); d != 0 {
		t.Errorf("LOW/1 → %d, quer 0", d)
	}
	if a.Epsilon() != 0 {
		t.Error("Greedy deve ter ε = 0")
	}
	a.EndEpisode() // sem efeito no Greedy
}

func TestTable_CopyAndSet(t *testing.T) {
	a := qlearning.NewAgent(testPolicy, testParams, qlearning.Greedy{})
	tbl := a.Table()
	tbl[0][0] = 5
	if a.Table()[0][0] != 0 {
		t.Error("Table deve devolver cópia")
	}
	if err := a.SetTable(tbl); err != nil || a.Table()[0][0] != 5 {
		t.Errorf("SetTable falhou: %v", err)
	}
	if err := a.SetTable(qlearning.Table{{1}}); err == nil {
		t.Error("esperava erro de dimensão (estados)")
	}
	bad := a.Table()
	bad[3] = []float64{1}
	if err := a.SetTable(bad); err == nil {
		t.Error("esperava erro de dimensão (ações)")
	}
}
