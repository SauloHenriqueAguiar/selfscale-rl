// Package analysis contém as regras puras do Analisador (etapa A do MAPE-K):
// discretização do estado, classificação do sintoma e função de recompensa.
// O agente Q-Learning que decide a ação fica no subpacote qlearning.
package analysis

import "github.com/shna/selfscale-rl/internal/domain"

// BandOf discretiza o p95: LOW < LowRatio·SLA ≤ OK ≤ SLA < HIGH.
func BandOf(p95Ms float64, g domain.Goals) domain.LatencyBand {
	switch {
	case p95Ms > g.SLAP95Ms:
		return domain.BandHigh
	case p95Ms < g.LowRatio*g.SLAP95Ms:
		return domain.BandLow
	default:
		return domain.BandOK
	}
}

// Discretize converte um sintoma em estado, limitando réplicas ao intervalo da política
// (durante uma transição o Deployment pode reportar valores fora dele).
func Discretize(s domain.Symptom, g domain.Goals, p domain.Policy) domain.State {
	replicas := min(max(s.Replicas, p.MinReplicas), p.MaxReplicas)
	return domain.State{Band: BandOf(s.P95Ms, g), Replicas: replicas}
}

// Classify devolve a categoria do sintoma. A ordem dos casos é a prioridade.
func Classify(s domain.Symptom, g domain.Goals, p domain.Policy) domain.SymptomKind {
	switch {
	case !s.Valid:
		return domain.KindInvalid
	case s.P95Ms > g.SLAP95Ms:
		return domain.KindViolation
	case s.P95Ms >= g.RiskRatio*g.SLAP95Ms:
		return domain.KindRisk
	case BandOf(s.P95Ms, g) == domain.BandLow && s.Replicas > p.MinReplicas:
		return domain.KindIdle
	default:
		return domain.KindOK
	}
}
