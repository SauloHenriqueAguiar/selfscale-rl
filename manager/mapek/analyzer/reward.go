package analyzer

import "github.com/shna/selfscale-rl/manager/mapek/knowledge"

// Oscillated indica inversão de direção entre duas ações executadas seguidas.
func Oscillated(current, previous knowledge.Delta) bool {
	return current*previous < 0
}

// Reward avalia a ação executada no ciclo anterior a partir do que se observa agora.
// Os pesos codificam a prioridade SLA > custo > estabilidade:
//
//	r = R_sla − c_rep·réplicas − c_osc·oscilou
func Reward(p95Ms float64, replicas int, executed, previous knowledge.Delta, g knowledge.Goals, w knowledge.RewardWeights) float64 {
	r := w.SLAOk
	if p95Ms > g.SLAP95Ms {
		r = w.SLAViolation
	}
	r -= w.CostPerReplica * float64(replicas)
	if Oscillated(executed, previous) {
		r -= w.OscillationPenalty
	}
	return r
}
