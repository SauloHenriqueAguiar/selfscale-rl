package domain

import "testing"

var testPolicy = Policy{MinReplicas: 1, MaxReplicas: 5, ScaleDownMaxStep: 1, ScaleDownCooldownCycles: 2}

func TestState_IndexCoversAllStatesOnce(t *testing.T) {
	seen := map[int]bool{}
	for b := BandLow; b <= BandHigh; b++ {
		for r := 1; r <= 5; r++ {
			s := State{Band: b, Replicas: r}
			i := s.Index(testPolicy)
			if seen[i] || i < 0 || i >= NumStates(testPolicy) {
				t.Fatalf("índice inválido ou repetido %d para %v", i, s)
			}
			seen[i] = true
			if back := StateFromIndex(i, testPolicy); back != s {
				t.Errorf("StateFromIndex(%d) = %v, quer %v", i, back, s)
			}
		}
	}
	if NumStates(testPolicy) != 15 {
		t.Errorf("NumStates = %d, quer 15", NumStates(testPolicy))
	}
}

func TestLatencyBand_String(t *testing.T) {
	if s := (State{Band: BandHigh, Replicas: 2}).String(); s != "HIGH/2" {
		t.Errorf("String = %q", s)
	}
}

func TestActions_ReturnsFreshSlice(t *testing.T) {
	a := Actions()
	a[0] = 99
	if Actions()[0] != -1 {
		t.Error("Actions não deve compartilhar estado")
	}
}
