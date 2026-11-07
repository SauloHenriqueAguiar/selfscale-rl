package monitor

import (
	"context"

	"github.com/shna/selfscale-rl/manager/mapek/knowledge"
)

// Portas de entrada do Monitor (sensores). Implementadas por
// manager/sensors/prometheus, manager/sensors/kube e pelo simulador.

// TrafficSource fornece p95, vazão e taxa de erros da última janela.
type TrafficSource interface {
	Traffic(ctx context.Context) (knowledge.Traffic, error)
}

// CPUReader fornece a CPU média do frontend em % do requests.cpu.
type CPUReader interface {
	CPUPercent(ctx context.Context) (float64, error)
}

// ReplicaReader fornece réplicas desejadas (spec) e prontas (status).
type ReplicaReader interface {
	Replicas(ctx context.Context) (desired, ready int, err error)
}
