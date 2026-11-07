package knowledge_test

import (
	"testing"

	"github.com/shna/selfscale-rl/manager/mapek/knowledge"
)

var testPolicy = knowledge.Policy{MinReplicas: 1, MaxReplicas: 5, ScaleDownMaxStep: 1, ScaleDownCooldownCycles: 2}

func TestState_IndexCoversAllStatesOnce(t *testing.T) {
	seen := map[int]bool{}
	for b := knowledge.BandLow; b <= knowledge.BandHigh; b++ {
		for r := 1; r <= 5; r++ {
			s := knowledge.State{Band: b, Replicas: r}
			i := s.Index(testPolicy)
			if seen[i] || i < 0 || i >= knowledge.NumStates(testPolicy) {
				t.Fatalf("índice inválido ou repetido %d para %v", i, s)
			}
			seen[i] = true
			if back := knowledge.StateFromIndex(i, testPolicy); back != s {
				t.Errorf("StateFromIndex(%d) = %v, quer %v", i, back, s)
			}
		}
	}
	if knowledge.NumStates(testPolicy) != 15 {
		t.Errorf("NumStates = %d, quer 15", knowledge.NumStates(testPolicy))
	}
}

func TestLatencyBand_String(t *testing.T) {
	if s := (knowledge.State{Band: knowledge.BandHigh, Replicas: 2}).String(); s != "HIGH/2" {
		t.Errorf("String = %q", s)
	}
}

func TestActions_ReturnsFreshSlice(t *testing.T) {
	a := knowledge.Actions()
	a[0] = 99
	if knowledge.Actions()[0] != -1 {
		t.Error("Actions não deve compartilhar estado")
	}
}
