package simulator

import (
	"fmt"
	"math"
)

// Scenario é um perfil de carga em ciclos (mesmos perfis de experiments/locust/shapes.py).
type Scenario struct {
	Name   string
	Cycles int
	shape  func(cycle int) float64 // fração da distância entre base e pico, em [0, 1]
}

// Load devolve a vazão (req/s) no ciclo, entre base e peak.
func (s Scenario) Load(base, peak float64) func(cycle int) float64 {
	return func(cycle int) float64 { return base + (peak-base)*s.shape(cycle) }
}

// Durações em ciclos de 15 s.
const (
	cycles10min = 40
	cycles12min = 48
	stepStart   = 8  // 2 min
	stepEnd     = 28 // 7 min
	spikeEvery  = 12 // 3 min
	spikeLength = 4  // 1 min
	wavePeriod  = 16 // 4 min
)

// Scenarios devolve C0–C4 (C5 é só para o modo online no cluster).
func Scenarios() []Scenario {
	return []Scenario{
		{"C0", cycles10min, func(c int) float64 { return boolToFrac(c >= stepStart && c < stepEnd) }},
		{"C1", cycles10min, func(int) float64 { return 0 }},
		{"C2", cycles10min, func(c int) float64 { return 1 - math.Abs(float64(c)-cycles10min/2)/(cycles10min/2) }},
		{"C3", cycles12min, func(c int) float64 { return boolToFrac(c%spikeEvery >= spikeEvery-spikeLength) }},
		{"C4", cycles12min, func(c int) float64 { return (1 - math.Cos(2*math.Pi*float64(c)/wavePeriod)) / 2 }},
	}
}

// ScenarioByName procura um cenário.
func ScenarioByName(name string) (Scenario, error) {
	for _, s := range Scenarios() {
		if s.Name == name {
			return s, nil
		}
	}
	return Scenario{}, fmt.Errorf("cenário desconhecido %q", name)
}

func boolToFrac(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
