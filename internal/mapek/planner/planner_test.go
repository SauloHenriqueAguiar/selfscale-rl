package planner

import (
	"context"
	"testing"

	"github.com/shna/selfscale-rl/internal/domain"
	"github.com/shna/selfscale-rl/internal/mapek/knowledge"
	"github.com/shna/selfscale-rl/internal/mapek/mapektest"
)

func request(delta domain.Delta, replicas, ready int) domain.ChangeRequest {
	return domain.ChangeRequest{Delta: delta, Symptom: domain.Symptom{Replicas: replicas, ReadyReplicas: ready}}
}

func TestPlanner_UsesKnowledgeForCooldownAndPending(t *testing.T) {
	k := knowledge.New(&mapektest.History{})
	p := New(k, mapektest.Settings().Policy)

	if plan := p.Handle(request(+1, 2, 2)); plan.Empty || plan.Target != 3 {
		t.Fatalf("aumento deveria passar: %+v", plan)
	}
	k.RecordDispatch(+1) // aumento recém-executado
	k.FinishExecution(domain.ExecutionResult{Success: true})
	if plan := p.Handle(request(-1, 3, 3)); plan.BlockedBy != domain.BlockedCooldown {
		t.Errorf("BlockedBy = %q, quer cooldown", plan.BlockedBy)
	}
	k.RecordDispatch(+1) // execução em andamento
	if plan := p.Handle(request(+1, 4, 4)); plan.BlockedBy != domain.BlockedPending {
		t.Errorf("BlockedBy = %q, quer pending", plan.BlockedBy)
	}
}

func TestPlanner_RunForwardsPlansUntilCancelled(t *testing.T) {
	p := New(knowledge.New(&mapektest.History{}), mapektest.Settings().Policy)
	in, out := make(chan domain.ChangeRequest), make(chan domain.AdaptationPlan)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Run(ctx, in, out); close(done) }()

	in <- request(+1, 1, 1)
	if plan := <-out; plan.Target != 2 {
		t.Errorf("Target = %d", plan.Target)
	}
	in <- request(+1, 1, 1) // ninguém lê a saída: o cancelamento deve destravar
	cancel()
	<-done
}
