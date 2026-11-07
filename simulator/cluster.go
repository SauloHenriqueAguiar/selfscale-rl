package simulator

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/shna/selfscale-rl/manager/mapek/knowledge"
)

// Cluster simula o frontend sob uma carga λ(ciclo). O tempo só avança em
// Advance: pods novos só ficam prontos (e reduzem o p95) depois do tempo de
// prontidão sorteado, convertido em ciclos.
type Cluster struct {
	model   Model
	load    func(cycle int) float64
	rng     *rand.Rand
	period  time.Duration
	cycle   int
	desired int
	serving int
	readyAt int // ciclo em que as réplicas novas passam a atender
}

// NewCluster cria o cluster simulado com 'replicas' réplicas atendendo.
func NewCluster(m Model, load func(cycle int) float64, period time.Duration, replicas int, rng *rand.Rand) *Cluster {
	return &Cluster{model: m, load: load, rng: rng, period: period, desired: replicas, serving: replicas}
}

// Advance avança um ciclo; pods cujo tempo de prontidão passou começam a atender.
func (c *Cluster) Advance() {
	c.cycle++
	if c.cycle >= c.readyAt {
		c.serving = c.desired
	}
}

// Cycle devolve o ciclo atual.
func (c *Cluster) Cycle() int { return c.cycle }

// Traffic implementa monitor.TrafficSource.
func (c *Cluster) Traffic(context.Context) (knowledge.Traffic, error) {
	rps := c.load(c.cycle)
	if rps <= 0 {
		return knowledge.Traffic{P95Seconds: math.NaN()}, nil
	}
	noise := max(0, 1+c.rng.NormFloat64()*c.model.Sigma)
	return knowledge.Traffic{P95Seconds: c.model.P95Ms(rps, c.serving) * noise / 1000, RPS: rps}, nil
}

// CPUPercent aproxima a CPU pela utilização (limitada a 100%).
func (c *Cluster) CPUPercent(context.Context) (float64, error) {
	return 100 * min(1, c.model.Utilization(c.load(c.cycle), c.serving)), nil
}

// Replicas implementa monitor.ReplicaReader: desejadas e prontas (atendendo).
func (c *Cluster) Replicas(context.Context) (int, int, error) { return c.desired, c.serving, nil }

// ExecutorView é o ReplicaReader entregue ao Executor síncrono do simulador.
// Como o tempo não anda durante um ciclo, aguardar a prontidão travaria o
// treino; por isso o Executor vê a escala como concluída assim que aceita.
// O Monitor continua vendo as prontas reais (e o Planejador bloqueia por pendência).
func (c *Cluster) ExecutorView() AcceptedReplicas { return AcceptedReplicas{c} }

// AcceptedReplicas reporta as réplicas desejadas também como prontas.
type AcceptedReplicas struct{ c *Cluster }

// Replicas implementa executor.ReplicaReader.
func (a AcceptedReplicas) Replicas(context.Context) (int, int, error) {
	return a.c.desired, a.c.desired, nil
}

// Scale implementa executor.Scaler. Reduções valem na hora; aumentos só atendem
// depois do tempo de prontidão.
func (c *Cluster) Scale(_ context.Context, n int) error {
	if n < 1 {
		return fmt.Errorf("réplicas inválidas: %d", n)
	}
	if n <= c.serving {
		c.desired, c.serving, c.readyAt = n, n, c.cycle
		return nil
	}
	c.desired = n
	c.readyAt = c.cycle + c.readyCycles()
	return nil
}

// readyCycles converte o tempo de prontidão sorteado em ciclos (arredondando para cima).
func (c *Cluster) readyCycles() int {
	m := c.model
	readyS := m.MinReadyS + c.rng.Float64()*(m.MaxReadyS-m.MinReadyS)
	return int(math.Ceil(readyS / c.period.Seconds()))
}
