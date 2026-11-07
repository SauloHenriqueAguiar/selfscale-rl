// Package planner é a etapa P do MAPE-K ("como fazer com segurança?"):
// transforma a Solicitação de Mudança em Plano de Adaptação.
//
// As regras (limites 1–5, redução de 1 em 1, cooldown, pendência) são a
// função pura Plan, em rules.go; planner.go só monta a visão do cluster.
package planner

import (
	"context"

	"github.com/shna/selfscale-rl/manager/mapek/knowledge"
)

// Planner é o Planejador.
type Planner struct {
	k      *knowledge.Base
	policy knowledge.Policy
}

// New cria o Planejador.
func New(k *knowledge.Base, p knowledge.Policy) *Planner {
	return &Planner{k: k, policy: p}
}

// Handle transforma a solicitação de mudança em plano de adaptação.
func (p *Planner) Handle(req knowledge.ChangeRequest) knowledge.AdaptationPlan {
	view := ClusterView{
		Replicas:           req.Symptom.Replicas,
		ReadyReplicas:      req.Symptom.ReadyReplicas,
		Executing:          p.k.Executing(),
		CyclesSinceScaleUp: p.k.TickCycle(),
	}
	return Plan(req, view, p.policy)
}

// Run lê solicitações de in (chAP) e escreve planos em out (chPE).
func (p *Planner) Run(ctx context.Context, in <-chan knowledge.ChangeRequest, out chan<- knowledge.AdaptationPlan) {
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
