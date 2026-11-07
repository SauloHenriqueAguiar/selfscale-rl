package planner

// Regras puras do Planejador (sem I/O): transformam a decisão do Analisador
// ("o que fazer") em um plano seguro ("como fazer").

import "github.com/shna/selfscale-rl/manager/mapek/knowledge"

// ClusterView é o que o Planejador precisa saber do sistema gerenciado.
type ClusterView struct {
	Replicas           int
	ReadyReplicas      int
	Executing          bool // há execução em andamento no Executor
	CyclesSinceScaleUp int
}

// Plan aplica as regras de segurança, na ordem de prioridade:
// dados inválidos → pendência → limites → redução conservadora (passo e cooldown).
func Plan(req knowledge.ChangeRequest, cur ClusterView, pol knowledge.Policy) knowledge.AdaptationPlan {
	empty := knowledge.AdaptationPlan{T: req.T, Target: cur.Replicas, Fallback: cur.Replicas, Empty: true, Request: req}

	switch {
	case req.Reason == knowledge.ReasonInvalidData:
		empty.BlockedBy = knowledge.BlockedInvalidData
		return empty
	case cur.Executing || cur.ReadyReplicas < cur.Replicas:
		empty.BlockedBy = knowledge.BlockedPending
		return empty
	case req.Delta == 0:
		return empty
	}

	target, blockedBy := nextTarget(req.Delta, cur, pol)
	if blockedBy != "" {
		empty.BlockedBy = blockedBy
		return empty
	}
	return knowledge.AdaptationPlan{
		T:      req.T,
		Target: target,
		Steps: []knowledge.Step{
			{Kind: knowledge.StepScale, Replicas: target},
			{Kind: knowledge.StepAwaitReady, Replicas: target},
			{Kind: knowledge.StepConfirm, Replicas: target},
		},
		Timeout:  pol.ReadinessTimeout,
		Fallback: cur.Replicas,
		Request:  req,
	}
}

// nextTarget calcula o alvo ou o motivo do bloqueio.
func nextTarget(delta knowledge.Delta, cur ClusterView, pol knowledge.Policy) (int, string) {
	if delta < 0 {
		if cur.CyclesSinceScaleUp < pol.ScaleDownCooldownCycles {
			return 0, knowledge.BlockedCooldown
		}
		delta = max(delta, knowledge.Delta(-pol.ScaleDownMaxStep))
	}
	target := min(max(cur.Replicas+int(delta), pol.MinReplicas), pol.MaxReplicas)
	if target == cur.Replicas {
		return 0, knowledge.BlockedBounds
	}
	return target, ""
}
