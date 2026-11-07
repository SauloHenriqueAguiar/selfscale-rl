package executor

import "context"

// Portas do Executor (efetuadores). Implementadas por adapters/kube e pelo simulador.

// Scaler altera o número de réplicas. Contrato: escala para exatamente n ou
// devolve erro; nunca aplica parcialmente em silêncio.
type Scaler interface {
	Scale(ctx context.Context, replicas int) error
}

// ReplicaReader fornece réplicas desejadas e prontas (para confirmar a prontidão).
type ReplicaReader interface {
	Replicas(ctx context.Context) (desired, ready int, err error)
}
