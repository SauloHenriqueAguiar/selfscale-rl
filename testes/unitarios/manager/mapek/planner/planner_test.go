package planner_test

import (
	"context"
	"testing"

	"github.com/shna/selfscale-rl/manager/mapek/knowledge"
	"github.com/shna/selfscale-rl/manager/mapek/planner"
	"github.com/shna/selfscale-rl/testes/fakes"
)

func request(delta knowledge.Delta, replicas, ready int) knowledge.ChangeRequest {
	return knowledge.ChangeRequest{Delta: delta, Symptom: knowledge.Symptom{Replicas: replicas, ReadyReplicas: ready}}
}

func TestPlanner_UsesKnowledgeForCooldownAndPending(t *testing.T) {
	k := knowledge.New(&fakes.History{})
	p := planner.New(k, fakes.Settings().Policy)

	if plan := p.Handle(request(+1, 2, 2)); plan.Empty || plan.Target != 3 {
		t.Fatalf("aumento deveria passar: %+v", plan)
	}
	k.RecordDispatch(+1) // aumento recém-executado
	k.FinishExecution(knowledge.ExecutionResult{Success: true})
	if plan := p.Handle(request(-1, 3, 3)); plan.BlockedBy != knowledge.BlockedCooldown {
		t.Errorf("BlockedBy = %q, quer cooldown", plan.BlockedBy)
	}
	k.RecordDispatch(+1) // execução em andamento
	if plan := p.Handle(request(+1, 4, 4)); plan.BlockedBy != knowledge.BlockedPending {
		t.Errorf("BlockedBy = %q, quer pending", plan.BlockedBy)
	}
}

func TestPlanner_RunForwardsPlansUntilCancelled(t *testing.T) {
	p := planner.New(knowledge.New(&fakes.History{}), fakes.Settings().Policy)
	in, out := make(chan knowledge.ChangeRequest), make(chan knowledge.AdaptationPlan)
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
