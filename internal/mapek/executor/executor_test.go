package executor

import (
	"context"
	"testing"
	"time"

	"github.com/shna/selfscale-rl/internal/domain"
	"github.com/shna/selfscale-rl/internal/domain/planning"
	"github.com/shna/selfscale-rl/internal/mapek/knowledge"
	"github.com/shna/selfscale-rl/internal/mapek/mapektest"
)

const target = "boutique/frontend"

func newTestExecutor(c *mapektest.Cluster, allow ...string) (*Executor, *knowledge.Base, *mapektest.History) {
	h := &mapektest.History{}
	k := knowledge.New(h)
	opts := Options{Target: target, Allowlist: allow, PollInterval: time.Millisecond}
	return New(c, c, k, opts), k, h
}

func scalePlan(from, to int, timeout time.Duration) domain.AdaptationPlan {
	pol := mapektest.Settings().Policy
	pol.ReadinessTimeout = timeout
	view := planning.ClusterView{Replicas: from, ReadyReplicas: from, CyclesSinceScaleUp: 99}
	return planning.Plan(domain.ChangeRequest{Delta: domain.Delta(to - from)}, view, pol)
}

func TestExecutor_EmptyPlanDoesNotScale(t *testing.T) {
	c := &mapektest.Cluster{Desired: 2, Ready: 2}
	e, k, h := newTestExecutor(c, target)

	got := e.Dispatch(context.Background(), domain.AdaptationPlan{Empty: true, Target: 2, Fallback: 2})

	if got != 0 || len(c.ScaleCalls()) != 0 || k.Executing() || h.Count() != 1 {
		t.Errorf("executed=%d scale=%v executing=%v registros=%d", got, c.ScaleCalls(), k.Executing(), h.Count())
	}
}

func TestExecutor_RefusesTargetOutsideAllowlist(t *testing.T) {
	c := &mapektest.Cluster{Desired: 1, Ready: 1}
	e, _, _ := newTestExecutor(c, "outro/deploy")
	if got := e.Dispatch(context.Background(), scalePlan(1, 2, time.Second)); got != 0 || len(c.ScaleCalls()) != 0 {
		t.Errorf("deveria recusar: executed=%d scale=%v", got, c.ScaleCalls())
	}
}

func TestExecutor_SuccessfulScale(t *testing.T) {
	c := &mapektest.Cluster{Desired: 1, Ready: 1, InstantReady: true}
	e, k, h := newTestExecutor(c, target)

	got := e.Dispatch(context.Background(), scalePlan(1, 3, time.Second))

	last, _ := k.LastExecuted()
	if got != +2 || last != +2 || k.Executing() {
		t.Errorf("executed=%d last=%d executing=%v", got, last, k.Executing())
	}
	if rec := h.Records()[0]; rec.Finished == nil || !rec.Finished.Success || rec.Finished.RolledBack {
		t.Errorf("registro = %+v", rec.Finished)
	}
}

func TestExecutor_TimeoutTriggersFallback(t *testing.T) {
	c := &mapektest.Cluster{Desired: 2, Ready: 2} // novas réplicas nunca ficam prontas
	e, _, _ := newTestExecutor(c, target)

	res := e.Execute(context.Background(), scalePlan(2, 3, 5*time.Millisecond))

	if res.Success || !res.RolledBack {
		t.Errorf("resultado = %+v", res)
	}
	if calls := c.ScaleCalls(); len(calls) != 2 || calls[0] != 3 || calls[1] != 2 {
		t.Errorf("chamadas de Scale = %v, quer [3 2]", calls)
	}
}

func TestExecutor_ScaleErrorTriggersFallback(t *testing.T) {
	c := &mapektest.Cluster{Desired: 2, Ready: 2, ScaleErr: mapektest.ErrFake}
	e, _, _ := newTestExecutor(c, target)
	if res := e.Execute(context.Background(), scalePlan(2, 3, time.Second)); !res.RolledBack {
		t.Errorf("resultado = %+v", res)
	}
}

func TestExecutor_UnknownStepFails(t *testing.T) {
	c := &mapektest.Cluster{Desired: 1, Ready: 1}
	e, _, _ := newTestExecutor(c, target)
	plan := domain.AdaptationPlan{Target: 2, Fallback: 1, Timeout: time.Second, Steps: []domain.Step{{Kind: "reboot"}}}
	if res := e.Execute(context.Background(), plan); !res.RolledBack {
		t.Errorf("passo desconhecido deve falhar: %+v", res)
	}
}

func TestExecutor_ShutdownDoesNotRollBack(t *testing.T) {
	c := &mapektest.Cluster{Desired: 1, Ready: 1}
	e, _, _ := newTestExecutor(c, target)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if res := e.Execute(ctx, scalePlan(1, 2, time.Second)); res.RolledBack {
		t.Error("encerramento não deve acionar plano B")
	}
}

func TestExecutor_AsyncReportsFinishedInLaterCycle(t *testing.T) {
	c := &mapektest.Cluster{Desired: 1, Ready: 1, InstantReady: true}
	e, k, h := newTestExecutor(c, target)
	e.opts.Async = true

	e.Dispatch(context.Background(), scalePlan(1, 2, time.Second))
	mapektest.WaitUntil(t, func() bool { return !k.Executing() })
	e.Dispatch(context.Background(), domain.AdaptationPlan{Empty: true})

	if rec := h.Records()[1]; rec.Finished == nil || !rec.Finished.Success {
		t.Errorf("o ciclo seguinte deveria registrar a execução concluída")
	}
}

func TestConfirmStep(t *testing.T) {
	cases := []struct {
		name    string
		c       *mapektest.Cluster
		wantErr bool
	}{
		{"confirma", &mapektest.Cluster{Desired: 3, Ready: 3}, false},
		{"desejadas diferentes", &mapektest.Cluster{Desired: 2, Ready: 3}, true},
		{"erro de leitura", &mapektest.Cluster{ReadErr: mapektest.ErrFake}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ConfirmStep{Reader: c.c, Replicas: 3}.Execute(context.Background())
			if (err != nil) != c.wantErr {
				t.Errorf("err = %v", err)
			}
		})
	}
}
