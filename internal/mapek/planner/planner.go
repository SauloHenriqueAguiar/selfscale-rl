// Package planner é a etapa P do MAPE-K ("como fazer com segurança?"):
// transforma a Solicitação de Mudança em Plano de Adaptação.
//
// As regras (limites 1–5, redução de 1 em 1, cooldown, pendência) são a
// função pura domain/planning.Plan; aqui só se monta a visão do cluster.
package planner

import (
	"context"

	"github.com/shna/selfscale-rl/internal/domain"
	"github.com/shna/selfscale-rl/internal/domain/planning"
	"github.com/shna/selfscale-rl/internal/mapek/knowledge"
)

// Planner é o Planejador.
type Planner struct {
	k      *knowledge.Base
	policy domain.Policy
}

// New cria o Planejador.
func New(k *knowledge.Base, p domain.Policy) *Planner {
	return &Planner{k: k, policy: p}
}

// Handle transforma a solicitação de mudança em plano de adaptação.
func (p *Planner) Handle(req domain.ChangeRequest) domain.AdaptationPlan {
	view := planning.ClusterView{
		Replicas:           req.Symptom.Replicas,
		ReadyReplicas:      req.Symptom.ReadyReplicas,
		Executing:          p.k.Executing(),
		CyclesSinceScaleUp: p.k.TickCycle(),
	}
	return planning.Plan(req, view, p.policy)
}

// Run lê solicitações de in (chAP) e escreve planos em out (chPE).
func (p *Planner) Run(ctx context.Context, in <-chan domain.ChangeRequest, out chan<- domain.AdaptationPlan) {
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-in:
			select {
			case out <- p.Handle(req):
			case <-ctx.Done():
				return
			}
		}
	}
}
