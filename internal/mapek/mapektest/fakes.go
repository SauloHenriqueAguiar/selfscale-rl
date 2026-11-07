// Package mapektest reúne os fakes (escritos à mão, sem framework de mock)
// usados nos testes das etapas MAPE-K.
package mapektest

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/shna/selfscale-rl/internal/domain"
	"github.com/shna/selfscale-rl/internal/domain/analysis/qlearning"
)

// ErrFake é o erro devolvido pelos fakes configurados para falhar.
var ErrFake = errors.New("falha simulada")

// Settings são configurações de teste equivalentes ao config.yaml de exemplo.
func Settings() domain.Settings {
	return domain.Settings{
		Goals: domain.Goals{SLAP95Ms: 200, LowRatio: 0.5, RiskRatio: 0.8},
		Policy: domain.Policy{MinReplicas: 1, MaxReplicas: 5, ScaleDownMaxStep: 1,
			ScaleDownCooldownCycles: 2, ReadinessTimeout: time.Second},
		Reward: domain.RewardWeights{SLAOk: 1, SLAViolation: -2, CostPerReplica: 0.1, OscillationPenalty: 0.5},
	}
}

// Traffic implementa monitor.TrafficSource.
type Traffic struct {
	T   domain.Traffic
	Err error
}

// Traffic devolve a leitura configurada.
func (f Traffic) Traffic(context.Context) (domain.Traffic, error) { return f.T, f.Err }

// CPU implementa monitor.CPUReader.
type CPU struct {
	Pct float64
	Err error
}

// CPUPercent devolve a leitura configurada.
func (f CPU) CPUPercent(context.Context) (float64, error) { return f.Pct, f.Err }

// Cluster implementa ReplicaReader e Scaler; com InstantReady as réplicas
// ficam prontas assim que escaladas.
type Cluster struct {
	mu            sync.Mutex
	Desired       int
	Ready         int
	InstantReady  bool
	ScaleErr      error
	ReadErr       error
	scaleRequests []int
}

// Replicas devolve desejadas e prontas.
func (f *Cluster) Replicas(context.Context) (int, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Desired, f.Ready, f.ReadErr
}

// Scale registra a chamada e aplica a escala (salvo se ScaleErr).
func (f *Cluster) Scale(_ context.Context, n int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scaleRequests = append(f.scaleRequests, n)
	if f.ScaleErr != nil {
		return f.ScaleErr
	}
	f.Desired = n
	if f.InstantReady {
		f.Ready = n
	}
	return nil
}

// ScaleCalls devolve os valores pedidos a Scale, em ordem.
func (f *Cluster) ScaleCalls() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int(nil), f.scaleRequests...)
}

// DesiredReplicas lê Desired com segurança.
func (f *Cluster) DesiredReplicas() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Desired
}

// Decider devolve sempre o mesmo Δ.
type Decider struct{ Delta domain.Delta }

// Decide devolve o Δ configurado.
func (f Decider) Decide(domain.State) domain.Delta { return f.Delta }

// Epsilon é fixo.
func (Decider) Epsilon() float64 { return 0.1 }

// LearnCall registra uma chamada a Learn.
type LearnCall struct {
	S, Next  domain.State
	Executed domain.Delta
	Reward   float64
}

// Learner registra as chamadas a Learn.
type Learner struct{ Calls []LearnCall }

// Learn registra a transição.
func (f *Learner) Learn(s domain.State, d domain.Delta, r float64, n domain.State) {
	f.Calls = append(f.Calls, LearnCall{S: s, Executed: d, Reward: r, Next: n})
}

// Episodes conta os fins de episódio.
type Episodes struct{ Ended int }

// EndEpisode conta mais um episódio.
func (f *Episodes) EndEpisode() { f.Ended++ }

// History guarda os registros em memória.
type History struct {
	mu      sync.Mutex
	records []domain.CycleRecord
	Err     error
}

// Write guarda o registro.
func (f *History) Write(r domain.CycleRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = append(f.records, r)
	return f.Err
}

// Records devolve uma cópia dos registros.
func (f *History) Records() []domain.CycleRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]domain.CycleRecord(nil), f.records...)
}

// Count devolve o número de registros.
func (f *History) Count() int { return len(f.Records()) }

// Store guarda a Q-table em memória.
type Store struct {
	Saved qlearning.Table
	Err   error
}

// Load devolve a tabela salva.
func (f *Store) Load() (qlearning.Table, error) { return f.Saved, f.Err }

// Save guarda a tabela.
func (f *Store) Save(t qlearning.Table) error { f.Saved = t; return f.Err }

// WaitUntil espera cond ficar verdadeira (goroutines de fakes terminam rápido)
// sem usar time.Sleep; falha após um prazo generoso.
func WaitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condição não satisfeita a tempo")
		}
		runtime.Gosched()
	}
}
