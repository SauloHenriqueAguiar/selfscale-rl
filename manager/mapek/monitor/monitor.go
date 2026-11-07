// Package monitor é a etapa M do MAPE-K ("o que está acontecendo?"):
// coleta as métricas a cada ciclo, normaliza, filtra e emite o Sintoma.
//
// Arquivos: ports.go (sensores de que precisa), filters.go (processamento) e
// monitor.go (Coletor, Gerador e Emissor de Sintoma).
package monitor

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/shna/selfscale-rl/manager/mapek/knowledge"
)

// Monitor coleta as métricas e emite o sintoma.
type Monitor struct {
	traffic    TrafficSource
	cpu        CPUReader
	replicas   ReplicaReader
	processors []Processor
	cycle      int
}

// New cria o Monitor com o pipeline de processadores informado.
func New(t TrafficSource, c CPUReader, r ReplicaReader, processors []Processor) *Monitor {
	return &Monitor{traffic: t, cpu: c, replicas: r, processors: processors}
}

// Handle executa um ciclo de monitoramento: coleta → normaliza → filtra.
func (m *Monitor) Handle(ctx context.Context, now time.Time) knowledge.Symptom {
	m.cycle++
	s := Normalize(m.collect(ctx, now))
	for _, p := range m.processors {
		s = p(s)
	}
	return s
}

// collect lê todas as fontes. A CPU só é registrada (não entra no estado),
// por isso a falha dela não invalida o ciclo.
func (m *Monitor) collect(ctx context.Context, now time.Time) Sample {
	s := Sample{T: now, Cycle: m.cycle}
	var err error
	if s.Traffic, err = m.traffic.Traffic(ctx); err != nil {
		slog.Warn("falha ao ler tráfego", "err", err)
		s.Failed = true
	}
	if s.Replicas, s.Ready, err = m.replicas.Replicas(ctx); err != nil {
		slog.Warn("falha ao ler réplicas", "err", err)
		s.Failed = true
	}
	if s.CPUPct, err = m.cpu.CPUPercent(ctx); err != nil {
		slog.Warn("falha ao ler CPU", "err", err)
		s.CPUPct = math.NaN()
	}
	return s
}

// Run emite um sintoma em out (canal chMA) a cada tick, até ctx terminar.
func (m *Monitor) Run(ctx context.Context, ticks <-chan time.Time, out chan<- knowledge.Symptom) {
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticks:
			select {
			case out <- m.Handle(ctx, now):
			case <-ctx.Done():
				return
			}
		}
	}
}
