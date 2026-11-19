package simulator

import (
	"fmt"
	"math"
)

// Scenario é um perfil de carga em ciclos (mesmos perfis de experiments/locust/shapes.py).
// A cauda sem carga (WithIdleTail) existe só no simulador.
type Scenario struct {
	Name     string
	Cycles   int
	shape    func(cycle int) float64 // fração da distância entre base e pico, em [0, 1]
	idleFrom int                     // a partir deste ciclo não há tráfego (0 = sem cauda)
}

// Load devolve a vazão (req/s) no ciclo, entre base e peak (0 na cauda sem carga).
func (s Scenario) Load(base, peak float64) func(cycle int) float64 {
	return func(cycle int) float64 {
		if s.idleFrom > 0 && cycle >= s.idleFrom {
			return 0
		}
		return base + (peak-base)*s.shape(cycle)
	}
}

// WithIdleTail acrescenta n ciclos sem tráfego ao fim do cenário. Sem isso o
// treino nunca vê a carga acabar (p95 = 0, req/s = 0) e não aprende a reduzir.
func (s Scenario) WithIdleTail(n int) Scenario {
	if n <= 0 {
		return s
	}
	s.idleFrom, s.Cycles = s.Cycles, s.Cycles+n
	return s
}

// TrainingScenarios devolve C0–C4, cada um seguido de idleCycles ciclos sem carga.
func TrainingScenarios(idleCycles int) []Scenario {
	out := Scenarios()
	for i := range out {
		out[i] = out[i].WithIdleTail(idleCycles)
	}
	return out
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
		{Name: "C0", Cycles: cycles10min, shape: func(c int) float64 { return boolToFrac(c >= stepStart && c < stepEnd) }},
		{Name: "C1", Cycles: cycles10min, shape: func(int) float64 { return 0 }},
		{Name: "C2", Cycles: cycles10min, shape: func(c int) float64 { return 1 - math.Abs(float64(c)-cycles10min/2)/(cycles10min/2) }},
		{Name: "C3", Cycles: cycles12min, shape: func(c int) float64 { return boolToFrac(c%spikeEvery >= spikeEvery-spikeLength) }},
		{Name: "C4", Cycles: cycles12min, shape: func(c int) float64 { return (1 - math.Cos(2*math.Pi*float64(c)/wavePeriod)) / 2 }},
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
