package analysis

import (
	"math"
	"testing"

	"github.com/shna/selfscale-rl/internal/domain"
)

var (
	testGoals   = domain.Goals{SLAP95Ms: 200, LowRatio: 0.5, RiskRatio: 0.8}
	testPolicy  = domain.Policy{MinReplicas: 1, MaxReplicas: 5, ScaleDownMaxStep: 1, ScaleDownCooldownCycles: 2}
	testWeights = domain.RewardWeights{SLAOk: 1, SLAViolation: -2, CostPerReplica: 0.1, OscillationPenalty: 0.5}
)

func TestBandOf_Boundaries(t *testing.T) {
	cases := []struct {
		p95  float64
		want domain.LatencyBand
	}{
		{0, domain.BandLow}, {99.9, domain.BandLow}, {100, domain.BandOK}, {160, domain.BandOK},
		{200, domain.BandOK}, {200.1, domain.BandHigh}, {1000, domain.BandHigh},
	}
	for _, c := range cases {
		if got := BandOf(c.p95, testGoals); got != c.want {
			t.Errorf("BandOf(%v) = %v, quer %v", c.p95, got, c.want)
		}
	}
}

func TestDiscretize_ClampsReplicas(t *testing.T) {
	got := Discretize(domain.Symptom{P95Ms: 250, Replicas: 7}, testGoals, testPolicy)
	if got != (domain.State{Band: domain.BandHigh, Replicas: 5}) {
		t.Errorf("Discretize = %v", got)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		s    domain.Symptom
		want domain.SymptomKind
	}{
		{"inválido", domain.Symptom{Valid: false, P95Ms: 500}, domain.KindInvalid},
		{"violação", domain.Symptom{Valid: true, P95Ms: 201, Replicas: 1}, domain.KindViolation},
		{"risco na fronteira 160", domain.Symptom{Valid: true, P95Ms: 160, Replicas: 1}, domain.KindRisk},
		{"risco no SLA", domain.Symptom{Valid: true, P95Ms: 200, Replicas: 3}, domain.KindRisk},
		{"ocioso", domain.Symptom{Valid: true, P95Ms: 50, Replicas: 3}, domain.KindIdle},
		{"baixo com 1 réplica é ok", domain.Symptom{Valid: true, P95Ms: 50, Replicas: 1}, domain.KindOK},
		{"ok", domain.Symptom{Valid: true, P95Ms: 120, Replicas: 2}, domain.KindOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Classify(c.s, testGoals, testPolicy); got != c.want {
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
		executed, previous domain.Delta
		want               float64
	}{
		{"SLA ok, 3 réplicas, sem oscilar", 150, 3, +1, +1, 0.7},
		{"violação com 1 réplica", 300, 1, 0, 0, -2.1},
		{"oscilação", 150, 2, -1, +1, 1 - 0.2 - 0.5},
		{"manter após aumento não oscila", 150, 2, 0, +1, 0.8},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Reward(c.p95, c.replicas, c.executed, c.previous, testGoals, testWeights)
			if math.Abs(got-c.want) > 1e-9 {
				t.Errorf("Reward = %v, quer %v", got, c.want)
			}
		})
	}
}
