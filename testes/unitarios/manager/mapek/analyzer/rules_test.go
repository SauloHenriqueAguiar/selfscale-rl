package analyzer_test

import (
	"math"
	"testing"

	"github.com/shna/selfscale-rl/manager/mapek/analyzer"
	"github.com/shna/selfscale-rl/manager/mapek/knowledge"
)

var (
	testGoals   = knowledge.Goals{SLAP95Ms: 200, LowRatio: 0.5, RiskRatio: 0.8}
	testPolicy  = knowledge.Policy{MinReplicas: 1, MaxReplicas: 5, ScaleDownMaxStep: 1, ScaleDownCooldownCycles: 2}
	testWeights = knowledge.RewardWeights{SLAOk: 1, SLAViolation: -2, CostPerReplica: 0.1, OscillationPenalty: 0.5}
)

func TestBandOf_Boundaries(t *testing.T) {
	cases := []struct {
		p95  float64
		want knowledge.LatencyBand
	}{
		{0, knowledge.BandLow}, {99.9, knowledge.BandLow}, {100, knowledge.BandOK}, {160, knowledge.BandOK},
		{200, knowledge.BandOK}, {200.1, knowledge.BandHigh}, {1000, knowledge.BandHigh},
	}
	for _, c := range cases {
		if got := analyzer.BandOf(c.p95, testGoals); got != c.want {
			t.Errorf("BandOf(%v) = %v, quer %v", c.p95, got, c.want)
		}
	}
}

func TestDiscretize_ClampsReplicas(t *testing.T) {
	got := analyzer.Discretize(knowledge.Symptom{P95Ms: 250, Replicas: 7}, testGoals, testPolicy)
	if got != (knowledge.State{Band: knowledge.BandHigh, Replicas: 5}) {
		t.Errorf("Discretize = %v", got)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		s    knowledge.Symptom
		want knowledge.SymptomKind
	}{
		{"inválido", knowledge.Symptom{Valid: false, P95Ms: 500}, knowledge.KindInvalid},
		{"violação", knowledge.Symptom{Valid: true, P95Ms: 201, Replicas: 1}, knowledge.KindViolation},
		{"risco na fronteira 160", knowledge.Symptom{Valid: true, P95Ms: 160, Replicas: 1}, knowledge.KindRisk},
		{"risco no SLA", knowledge.Symptom{Valid: true, P95Ms: 200, Replicas: 3}, knowledge.KindRisk},
		{"ocioso", knowledge.Symptom{Valid: true, P95Ms: 50, Replicas: 3}, knowledge.KindIdle},
		{"baixo com 1 réplica é ok", knowledge.Symptom{Valid: true, P95Ms: 50, Replicas: 1}, knowledge.KindOK},
		{"ok", knowledge.Symptom{Valid: true, P95Ms: 120, Replicas: 2}, knowledge.KindOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := analyzer.Classify(c.s, testGoals, testPolicy); got != c.want {
				t.Errorf("Classify = %v, quer %v", got, c.want)
			}
		})
	}
}

func TestReward(t *testing.T) {
	cases := []struct {
		name               string
		p95                float64
		replicas           int
		executed, previous knowledge.Delta
		want               float64
	}{
		{"SLA ok, 3 réplicas, sem oscilar", 150, 3, +1, +1, 0.7},
		{"violação com 1 réplica", 300, 1, 0, 0, -2.1},
		{"oscilação", 150, 2, -1, +1, 1 - 0.2 - 0.5},
		{"manter após aumento não oscila", 150, 2, 0, +1, 0.8},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := analyzer.Reward(c.p95, c.replicas, c.executed, c.previous, testGoals, testWeights)
			if math.Abs(got-c.want) > 1e-9 {
				t.Errorf("Reward = %v, quer %v", got, c.want)
			}
		})
	}
}
