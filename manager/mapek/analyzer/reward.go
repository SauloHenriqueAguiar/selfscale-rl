package analyzer

import "github.com/shna/selfscale-rl/manager/mapek/knowledge"

// Oscillated indica inversão de direção entre duas ações executadas seguidas.
func Oscillated(current, previous knowledge.Delta) bool {
	return current*previous < 0
}

// maxViolationRatio limita a penalidade a 3× o SLA (p95 de 600 ms ou mais).
const maxViolationRatio = 3

// Reward avalia a ação executada no ciclo anterior a partir do que se observa agora.
// Os pesos codificam a prioridade SLA > custo > estabilidade:
//
//	r = R_sla − c_rep·réplicas − c_osc·oscilou
//
// Na violação, R_sla = SLAViolation·min(p95/SLA, 3): violar por pouco custa
// menos que violar por muito. Com uma penalidade fixa, sob carga alta todas as
// réplicas abaixo da necessária violavam igual, e cada réplica a mais só
// aumentava o custo: o agente aprendia a reduzir com latência alta.
func Reward(p95Ms float64, replicas int, executed, previous knowledge.Delta, g knowledge.Goals, w knowledge.RewardWeights) float64 {
	r := w.SLAOk
	if p95Ms > g.SLAP95Ms {
		r = w.SLAViolation * min(p95Ms/g.SLAP95Ms, maxViolationRatio)
	}
	r -= w.CostPerReplica * float64(replicas)
	if Oscillated(executed, previous) {
		r -= w.OscillationPenalty
	}
	return r
}
