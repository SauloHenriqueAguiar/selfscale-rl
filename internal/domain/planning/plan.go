// Package planning contém as regras do Planejador: transformar a decisão do
// Analisador ("o que fazer") em um plano seguro ("como fazer").
package planning

import "github.com/shna/selfscale-rl/internal/domain"

// ClusterView é o que o Planejador precisa saber do sistema gerenciado.
type ClusterView struct {
	Replicas           int
	ReadyReplicas      int
	Executing          bool // há execução em andamento no Executor
	CyclesSinceScaleUp int
}

// Plan aplica as regras de segurança, na ordem de prioridade:
// dados inválidos → pendência → limites → redução conservadora (passo e cooldown).
func Plan(req domain.ChangeRequest, cur ClusterView, pol domain.Policy) domain.AdaptationPlan {
	empty := domain.AdaptationPlan{T: req.T, Target: cur.Replicas, Fallback: cur.Replicas, Empty: true, Request: req}

	switch {
	case req.Reason == domain.ReasonInvalidData:
		empty.BlockedBy = domain.BlockedInvalidData
		return empty
	case cur.Executing || cur.ReadyReplicas < cur.Replicas:
		empty.BlockedBy = domain.BlockedPending
		return empty
	case req.Delta == 0:
		return empty
	}

	target, blockedBy := nextTarget(req.Delta, cur, pol)
	if blockedBy != "" {
		empty.BlockedBy = blockedBy
		return empty
	}
	return domain.AdaptationPlan{
		T:      req.T,
		Target: target,
		Steps: []domain.Step{
			{Kind: domain.StepScale, Replicas: target},
			{Kind: domain.StepAwaitReady, Replicas: target},
			{Kind: domain.StepConfirm, Replicas: target},
		},
		Timeout:  pol.ReadinessTimeout,
		Fallback: cur.Replicas,
		Request:  req,
	}
}

// nextTarget calcula o alvo ou o motivo do bloqueio.
func nextTarget(delta domain.Delta, cur ClusterView, pol domain.Policy) (int, string) {
	if delta < 0 {
		if cur.CyclesSinceScaleUp < pol.ScaleDownCooldownCycles {
			return 0, domain.BlockedCooldown
		}
		delta = max(delta, domain.Delta(-pol.ScaleDownMaxStep))
	}
	target := min(max(cur.Replicas+int(delta), pol.MinReplicas), pol.MaxReplicas)
	if target == cur.Replicas {
		return 0, domain.BlockedBounds
	}
	return target, ""
}
