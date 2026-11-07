// Package simulator implementa as portas TrafficSource, CPUReader,
// ReplicaReader e Scaler com um modelo de filas simples, para pré-treinar o
// agente em minutos usando o mesmo Analisador, Planejador e Agente do cluster.
package simulator

import "math"

// Model é o modelo de latência calibrado com o piloto:
//
//	ρ = λ / (n·μ);  p95 ≈ base + k·ρ/(1−ρ)  (ρ ≥ MaxRho ⇒ p95 = MaxMs)
type Model struct {
	MuRPS     float64 `json:"mu_rps"`  // capacidade por réplica (req/s)
	BaseMs    float64 `json:"base_ms"` // latência sem fila
	KMs       float64 `json:"k_ms"`    // sensibilidade à fila
	MaxMs     float64 `json:"max_ms"`  // latência na saturação
	MaxRho    float64 `json:"max_rho"`
	Sigma     float64 `json:"sigma"`       // desvio do ruído multiplicativo
	MinReadyS float64 `json:"min_ready_s"` // tempo até um pod novo ficar pronto
	MaxReadyS float64 `json:"max_ready_s"`
}

// DefaultModel são valores iniciais plausíveis, usados até haver piloto.
func DefaultModel() Model {
	return Model{MuRPS: 20, BaseMs: 40, KMs: 20, MaxMs: 2000, MaxRho: 0.98, Sigma: 0.1, MinReadyS: 5, MaxReadyS: 20}
}

// Utilization devolve ρ para a vazão e as réplicas prontas.
func (m Model) Utilization(rps float64, ready int) float64 {
	if ready <= 0 {
		return math.Inf(1)
	}
	return rps / (float64(ready) * m.MuRPS)
}

// P95Ms devolve o p95 esperado (sem ruído).
func (m Model) P95Ms(rps float64, ready int) float64 {
	rho := m.Utilization(rps, ready)
	if rho >= m.MaxRho {
		return m.MaxMs
	}
	return min(m.BaseMs+m.KMs*queueFactor(rho), m.MaxMs)
}

// queueFactor é o termo ρ/(1−ρ) que cresce rapidamente perto da saturação.
func queueFactor(rho float64) float64 { return rho / (1 - rho) }
