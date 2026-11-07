package knowledge

import "time"

// Goals reúne a meta de latência e os limiares derivados dela.
type Goals struct {
	SLAP95Ms  float64
	LowRatio  float64 // p95 < LowRatio·SLA => faixa LOW
	RiskRatio float64 // p95 ≥ RiskRatio·SLA (e ≤ SLA) => sintoma "risk"
}

// Policy reúne as restrições de segurança aplicadas pelo Planejador.
type Policy struct {
	MinReplicas             int
	MaxReplicas             int
	ScaleDownMaxStep        int
	ScaleDownCooldownCycles int
	ReadinessTimeout        time.Duration
}

// ReplicaSpan é o número de valores possíveis de réplicas.
func (p Policy) ReplicaSpan() int { return p.MaxReplicas - p.MinReplicas + 1 }

// RewardWeights são os pesos da função de recompensa.
type RewardWeights struct {
	SLAOk              float64
	SLAViolation       float64
	CostPerReplica     float64
	OscillationPenalty float64
}

// LearningParams são os hiperparâmetros do Q-Learning.
type LearningParams struct {
	Alpha        float64
	Gamma        float64
	EpsilonStart float64
	EpsilonMin   float64
	EpsilonDecay float64
	PriorInit    bool
	Seed         int64
}

// Settings agrupa tudo o que o operador configura e o sistema só lê.
type Settings struct {
	Goals    Goals
	Policy   Policy
	Reward   RewardWeights
	Learning LearningParams
}
