package monitor

import (
	"context"

	"github.com/shna/selfscale-rl/internal/domain"
)

// Portas de entrada do Monitor (sensores). Implementadas por
// adapters/prometheus, adapters/kube e pelo simulador.

// TrafficSource fornece p95, vazão e taxa de erros da última janela.
type TrafficSource interface {
	Traffic(ctx context.Context) (domain.Traffic, error)
}

// CPUReader fornece a CPU média do frontend em % do requests.cpu.
type CPUReader interface {
	CPUPercent(ctx context.Context) (float64, error)
}

// ReplicaReader fornece réplicas desejadas (spec) e prontas (status).
type ReplicaReader interface {
	Replicas(ctx context.Context) (desired, ready int, err error)
}
