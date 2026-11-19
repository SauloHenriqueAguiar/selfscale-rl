// Package knowledge é a Base de Conhecimento (K do MAPE-K) compartilhada pelas
// etapas: mensagens, estado discreto, metas do operador e memória de execução.
package knowledge

import (
	"log/slog"
	"math"
	"sync"
)

// Base é a parte mutável da Base de Conhecimento. As etapas rodam em
// goroutines diferentes, por isso todo acesso passa pelo mutex.
type Base struct {
	mu                 sync.Mutex
	history            HistoryWriter
	lastExecuted       Delta
	prevExecuted       Delta
	cyclesSinceScaleUp int
	executing          bool
	finished           *ExecutionResult
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
func (b *Base) RecordDispatch(d Delta) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prevExecuted, b.lastExecuted = b.lastExecuted, d
	if d > 0 {
		b.cyclesSinceScaleUp = 0
	}
	b.executing = d != 0
}

// LastExecuted devolve as ações executadas nos dois últimos ciclos (para a recompensa).
func (b *Base) LastExecuted() (last, previous Delta) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastExecuted, b.prevExecuted
}

// FinishExecution registra o fim do workflow (prontidão confirmada ou plano B).
func (b *Base) FinishExecution(r ExecutionResult) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.executing = false
	b.finished = &r
}

// CyclesSinceScaleUp devolve os ciclos desde o último aumento, sem contar mais um.
func (b *Base) CyclesSinceScaleUp() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cyclesSinceScaleUp
}

// Executing indica se há execução em andamento.
func (b *Base) Executing() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.executing
}

// RecordCycle grava o registro do ciclo no histórico. A execução que terminou
// desde o registro anterior (se houver) vai junto e é consumida.
func (b *Base) RecordCycle(plan AdaptationPlan, executed Delta) {
	b.mu.Lock()
	finished := b.finished
	b.finished = nil
	b.mu.Unlock()

	r := CycleRecord{Plan: plan, Executed: executed, Finished: finished}
	if err := b.history.Write(r); err != nil {
		slog.Error("falha ao gravar histórico", "ciclo", plan.Request.Symptom.Cycle, "err", err)
	}
}
