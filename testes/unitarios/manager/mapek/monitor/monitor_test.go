package monitor_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/shna/selfscale-rl/manager/mapek/knowledge"
	"github.com/shna/selfscale-rl/manager/mapek/monitor"
	"github.com/shna/selfscale-rl/testes/fakes"
)

func TestNormalize_ConvertsUnits(t *testing.T) {
	s := monitor.Normalize(monitor.Sample{Traffic: knowledge.Traffic{P95Seconds: 0.25, RPS: 10, ErrorRatio: 0.02}, Replicas: 2, Ready: 2})
	if s.P95Ms != 250 || math.Abs(s.ErrorPct-2) > 1e-9 || !s.Valid {
		t.Errorf("Normalize = %+v", s)
	}
	if monitor.Normalize(monitor.Sample{Failed: true}).Valid {
		t.Error("falha de coleta deve gerar sintoma inválido")
	}
}

func TestFilters(t *testing.T) {
	cases := []struct {
		name      string
		in        knowledge.Symptom
		wantValid bool
		wantP95   float64
	}{
		{"sem tráfego é ocioso e válido", knowledge.Symptom{Valid: true, P95Ms: math.NaN(), RPS: 0}, true, 0},
		{"p95 NaN com tráfego é inválido", knowledge.Symptom{Valid: true, P95Ms: math.NaN(), RPS: 5}, false, math.NaN()},
		{"valor negativo é inválido", knowledge.Symptom{Valid: true, P95Ms: 10, RPS: -1}, false, 10},
		{"réplicas negativas é inválido", knowledge.Symptom{Valid: true, P95Ms: 10, RPS: 1, Replicas: -1}, false, 10},
		{"leitura normal passa", knowledge.Symptom{Valid: true, P95Ms: 120, RPS: 5, Replicas: 1}, true, 120},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.in
			for _, p := range monitor.DefaultProcessors() {
				got = p(got)
			}
			if got.Valid != c.wantValid {
				t.Errorf("Valid = %v, quer %v", got.Valid, c.wantValid)
			}
			if !(math.IsNaN(c.wantP95) && math.IsNaN(got.P95Ms)) && got.P95Ms != c.wantP95 {
				t.Errorf("P95Ms = %v, quer %v", got.P95Ms, c.wantP95)
			}
		})
	}
}

func TestMonitor_Handle(t *testing.T) {
	cluster := &fakes.Cluster{Desired: 3, Ready: 2}
	cases := []struct {
		name      string
		traffic   fakes.Traffic
		cpu       fakes.CPU
		wantValid bool
	}{
		{"tudo ok", fakes.Traffic{T: knowledge.Traffic{P95Seconds: 0.1, RPS: 20}}, fakes.CPU{Pct: 40}, true},
		{"falha no tráfego invalida", fakes.Traffic{Err: fakes.ErrFake}, fakes.CPU{Pct: 40}, false},
		{"falha na CPU não invalida", fakes.Traffic{T: knowledge.Traffic{P95Seconds: 0.1, RPS: 20}}, fakes.CPU{Err: fakes.ErrFake}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := monitor.New(c.traffic, c.cpu, cluster, monitor.DefaultProcessors())
			s := m.Handle(context.Background(), time.Unix(0, 0))
			if s.Valid != c.wantValid || s.Replicas != 3 || s.ReadyReplicas != 2 || s.Cycle != 1 {
				t.Errorf("sintoma = %+v", s)
			}
		})
	}
}

func TestMonitor_ReplicaFailureInvalidates(t *testing.T) {
	m := monitor.New(fakes.Traffic{}, fakes.CPU{}, &fakes.Cluster{ReadErr: fakes.ErrFake}, monitor.DefaultProcessors())
	if m.Handle(context.Background(), time.Now()).Valid {
		t.Error("falha nas réplicas deve invalidar")
	}
}

func TestMonitor_RunEmitsOneSymptomPerTick(t *testing.T) {
	m := monitor.New(fakes.Traffic{}, fakes.CPU{}, &fakes.Cluster{}, monitor.DefaultProcessors())
	ticks, out := make(chan time.Time), make(chan knowledge.Symptom)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx, ticks, out); close(done) }()

	ticks <- time.Now()
	if s := <-out; s.Cycle != 1 {
		t.Errorf("ciclo = %d", s.Cycle)
	}
	cancel()
	<-done
}
