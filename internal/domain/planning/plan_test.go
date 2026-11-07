package planning

import (
	"testing"
	"time"

	"github.com/shna/selfscale-rl/internal/domain"
)

var testPolicy = domain.Policy{
	MinReplicas: 1, MaxReplicas: 5, ScaleDownMaxStep: 1,
	ScaleDownCooldownCycles: 2, ReadinessTimeout: 60 * time.Second,
}

func view(replicas, sinceUp int) ClusterView {
	return ClusterView{Replicas: replicas, ReadyReplicas: replicas, CyclesSinceScaleUp: sinceUp}
}

func TestPlan(t *testing.T) {
	cases := []struct {
		name       string
		req        domain.ChangeRequest
		cur        ClusterView
		wantTarget int
		wantEmpty  bool
		wantBlock  string
	}{
		{"aumento normal", domain.ChangeRequest{Delta: +1}, view(2, 0), 3, false, ""},
		{"+2 limitado a 5", domain.ChangeRequest{Delta: +2}, view(4, 9), 5, false, ""},
		{"no máximo vira bloqueio por limite", domain.ChangeRequest{Delta: +1}, view(5, 9), 5, true, domain.BlockedBounds},
		{"redução bloqueada por cooldown", domain.ChangeRequest{Delta: -1}, view(3, 1), 3, true, domain.BlockedCooldown},
		{"redução liberada após 2 ciclos", domain.ChangeRequest{Delta: -1}, view(3, 2), 2, false, ""},
		{"redução limitada a 1 por vez", domain.ChangeRequest{Delta: -3}, view(5, 9), 4, false, ""},
		{"pendência por réplicas não prontas", domain.ChangeRequest{Delta: +1},
			ClusterView{Replicas: 3, ReadyReplicas: 2, CyclesSinceScaleUp: 9}, 3, true, domain.BlockedPending},
		{"pendência por execução em andamento", domain.ChangeRequest{Delta: +1},
			ClusterView{Replicas: 3, ReadyReplicas: 3, Executing: true}, 3, true, domain.BlockedPending},
		{"dados inválidos", domain.ChangeRequest{Reason: domain.ReasonInvalidData}, view(2, 9), 2, true, domain.BlockedInvalidData},
		{"Δ = 0 é plano vazio sem bloqueio", domain.ChangeRequest{Delta: 0}, view(2, 9), 2, true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := Plan(c.req, c.cur, testPolicy)
			if p.Target != c.wantTarget || p.Empty != c.wantEmpty || p.BlockedBy != c.wantBlock {
				t.Errorf("Plan = {Target:%d Empty:%v BlockedBy:%q}, quer {%d %v %q}",
					p.Target, p.Empty, p.BlockedBy, c.wantTarget, c.wantEmpty, c.wantBlock)
			}
			if p.Fallback != c.cur.Replicas {
				t.Errorf("Fallback = %d, quer %d", p.Fallback, c.cur.Replicas)
			}
		})
	}
}

func TestPlan_NonEmptyHasStepsAndTimeout(t *testing.T) {
	p := Plan(domain.ChangeRequest{Delta: +1}, view(1, 0), testPolicy)
	want := []domain.StepKind{domain.StepScale, domain.StepAwaitReady, domain.StepConfirm}
	if len(p.Steps) != len(want) {
		t.Fatalf("Steps = %v", p.Steps)
	}
	for i, k := range want {
		if p.Steps[i].Kind != k || p.Steps[i].Replicas != 2 {
			t.Errorf("Steps[%d] = %+v", i, p.Steps[i])
		}
	}
	if p.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v", p.Timeout)
	}
}
