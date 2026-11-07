package analyzer

// Regras puras do Analisador (sem I/O): discretização do estado e
// classificação do sintoma. A recompensa fica em reward.go.

import "github.com/shna/selfscale-rl/manager/mapek/knowledge"

// BandOf discretiza o p95: LOW < LowRatio·SLA ≤ OK ≤ SLA < HIGH.
func BandOf(p95Ms float64, g knowledge.Goals) knowledge.LatencyBand {
	switch {
	case p95Ms > g.SLAP95Ms:
		return knowledge.BandHigh
	case p95Ms < g.LowRatio*g.SLAP95Ms:
		return knowledge.BandLow
	default:
		return knowledge.BandOK
	}
}

// Discretize converte um sintoma em estado, limitando réplicas ao intervalo da política
// (durante uma transição o Deployment pode reportar valores fora dele).
func Discretize(s knowledge.Symptom, g knowledge.Goals, p knowledge.Policy) knowledge.State {
	replicas := min(max(s.Replicas, p.MinReplicas), p.MaxReplicas)
	return knowledge.State{Band: BandOf(s.P95Ms, g), Replicas: replicas}
}

// Classify devolve a categoria do sintoma. A ordem dos casos é a prioridade.
func Classify(s knowledge.Symptom, g knowledge.Goals, p knowledge.Policy) knowledge.SymptomKind {
	switch {
	case !s.Valid:
		return knowledge.KindInvalid
	case s.P95Ms > g.SLAP95Ms:
		return knowledge.KindViolation
	case s.P95Ms >= g.RiskRatio*g.SLAP95Ms:
		return knowledge.KindRisk
	case BandOf(s.P95Ms, g) == knowledge.BandLow && s.Replicas > p.MinReplicas:
		return knowledge.KindIdle
	default:
		return knowledge.KindOK
	}
}
