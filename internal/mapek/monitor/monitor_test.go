package monitor

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/shna/selfscale-rl/internal/domain"
	"github.com/shna/selfscale-rl/internal/mapek/mapektest"
)

func TestNormalize_ConvertsUnits(t *testing.T) {
	s := Normalize(Sample{Traffic: domain.Traffic{P95Seconds: 0.25, RPS: 10, ErrorRatio: 0.02}, Replicas: 2, Ready: 2})
	if s.P95Ms != 250 || math.Abs(s.ErrorPct-2) > 1e-9 || !s.Valid {
		t.Errorf("Normalize = %+v", s)
	}
	if Normalize(Sample{Failed: true}).Valid {
		t.Error("falha de coleta deve gerar sintoma inválido")
	}
}

func TestFilters(t *testing.T) {
	cases := []struct {
		name      string
		in        domain.Symptom
		wantValid bool
		wantP95   float64
	}{
		{"sem tráfego é ocioso e válido", domain.Symptom{Valid: true, P95Ms: math.NaN(), RPS: 0}, true, 0},
		{"p95 NaN com tráfego é inválido", domain.Symptom{Valid: true, P95Ms: math.NaN(), RPS: 5}, false, math.NaN()},
		{"valor negativo é inválido", domain.Symptom{Valid: true, P95Ms: 10, RPS: -1}, false, 10},
		{"réplicas negativas é inválido", domain.Symptom{Valid: true, P95Ms: 10, RPS: 1, Replicas: -1}, false, 10},
		{"leitura normal passa", domain.Symptom{Valid: true, P95Ms: 120, RPS: 5, Replicas: 1}, true, 120},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.in
			for _, p := range DefaultProcessors() {
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
	cluster := &mapektest.Cluster{Desired: 3, Ready: 2}
	cases := []struct {
		name      string
		traffic   mapektest.Traffic
		cpu       mapektest.CPU
		wantValid bool
	}{
		{"tudo ok", mapektest.Traffic{T: domain.Traffic{P95Seconds: 0.1, RPS: 20}}, mapektest.CPU{Pct: 40}, true},
		{"falha no tráfego invalida", mapektest.Traffic{Err: mapektest.ErrFake}, mapektest.CPU{Pct: 40}, false},
		{"falha na CPU não invalida", mapektest.Traffic{T: domain.Traffic{P95Seconds: 0.1, RPS: 20}}, mapektest.CPU{Err: mapektest.ErrFake}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := New(c.traffic, c.cpu, cluster, DefaultProcessors())
			s := m.Handle(context.Background(), time.Unix(0, 0))
			if s.Valid != c.wantValid || s.Replicas != 3 || s.ReadyReplicas != 2 || s.Cycle != 1 {
				t.Errorf("sintoma = %+v", s)
			}
		})
	}
}

func TestMonitor_ReplicaFailureInvalidates(t *testing.T) {
	m := New(mapektest.Traffic{}, mapektest.CPU{}, &mapektest.Cluster{ReadErr: mapektest.ErrFake}, DefaultProcessors())
	if m.Handle(context.Background(), time.Now()).Valid {
		t.Error("falha nas réplicas deve invalidar")
	}
}

func TestMonitor_RunEmitsOneSymptomPerTick(t *testing.T) {
	m := New(mapektest.Traffic{}, mapektest.CPU{}, &mapektest.Cluster{}, DefaultProcessors())
	ticks, out := make(chan time.Time), make(chan domain.Symptom)
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
