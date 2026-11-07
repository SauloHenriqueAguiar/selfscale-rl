// Package knowledge é a Base de Conhecimento (K do MAPE-K): a memória
// compartilhada pelas etapas. Guarda o estado de execução usado pelo
// Analisador e pelo Planejador e grava o histórico de cada ciclo.
//
// Metas e políticas (config.yaml) ficam em domain.Settings e são somente
// leitura; a Q-table é persistida pelo Checkpointer.
package knowledge

import (
	"log/slog"
	"math"
	"sync"

	"github.com/shna/selfscale-rl/internal/domain"
)

// Base é a parte mutável da Base de Conhecimento. As etapas rodam em
// goroutines diferentes, por isso todo acesso passa pelo mutex.
type Base struct {
	mu                 sync.Mutex
	history            HistoryWriter
	lastExecuted       domain.Delta
	prevExecuted       domain.Delta
	cyclesSinceScaleUp int
	executing          bool
	finished           *domain.ExecutionResult
}

// New cria a base sem aumento recente, para não bloquear reduções no início.
func New(history HistoryWriter) *Base {
	return &Base{history: history, cyclesSinceScaleUp: math.MaxInt32}
}

// TickCycle conta mais um ciclo desde o último aumento e devolve o total.
func (b *Base) TickCycle() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cyclesSinceScaleUp < math.MaxInt32 {
		b.cyclesSinceScaleUp++
	}
	return b.cyclesSinceScaleUp
}

// RecordDispatch registra a ação executada no ciclo (0 para plano vazio).
func (b *Base) RecordDispatch(d domain.Delta) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prevExecuted, b.lastExecuted = b.lastExecuted, d
	if d > 0 {
		b.cyclesSinceScaleUp = 0
	}
	b.executing = d != 0
}

// LastExecuted devolve as ações executadas nos dois últimos ciclos (para a recompensa).
func (b *Base) LastExecuted() (last, previous domain.Delta) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastExecuted, b.prevExecuted
}

// FinishExecution registra o fim do workflow (prontidão confirmada ou plano B).
func (b *Base) FinishExecution(r domain.ExecutionResult) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.executing = false
	b.finished = &r
}

// Executing indica se há execução em andamento.
func (b *Base) Executing() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.executing
}

// RecordCycle grava o registro do ciclo no histórico. A execução que terminou
// desde o registro anterior (se houver) vai junto e é consumida.
func (b *Base) RecordCycle(plan domain.AdaptationPlan, executed domain.Delta) {
	b.mu.Lock()
	finished := b.finished
	b.finished = nil
	b.mu.Unlock()

	r := domain.CycleRecord{Plan: plan, Executed: executed, Finished: finished}
	if err := b.history.Write(r); err != nil {
		slog.Error("falha ao gravar histórico", "ciclo", plan.Request.Symptom.Cycle, "err", err)
	}
}
