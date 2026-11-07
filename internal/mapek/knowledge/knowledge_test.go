package knowledge

import (
	"sync"
	"testing"

	"github.com/shna/selfscale-rl/internal/domain"
	"github.com/shna/selfscale-rl/internal/domain/analysis/qlearning"
	"github.com/shna/selfscale-rl/internal/mapek/mapektest"
)

func TestBase_CyclesSinceScaleUp(t *testing.T) {
	b := New(&mapektest.History{})
	if b.TickCycle() < 2 {
		t.Error("no início, reduções não devem ser bloqueadas")
	}
	b.RecordDispatch(+1)
	if b.TickCycle() != 1 || b.TickCycle() != 2 {
		t.Error("contagem após aumento deve recomeçar em 1")
	}
	b.RecordDispatch(-1)
	if last, prev := b.LastExecuted(); last != -1 || prev != +1 {
		t.Errorf("last=%d prev=%d", last, prev)
	}
}

func TestBase_RecordCycleAttachesFinishedExecutionOnce(t *testing.T) {
	h := &mapektest.History{}
	b := New(h)
	b.FinishExecution(domain.ExecutionResult{Success: true})

	b.RecordCycle(domain.AdaptationPlan{Empty: true}, 0)
	b.RecordCycle(domain.AdaptationPlan{Empty: true}, 0)

	recs := h.Records()
	if recs[0].Finished == nil || recs[1].Finished != nil {
		t.Error("a execução concluída deve aparecer em um único registro")
	}
	h.Err = mapektest.ErrFake
	b.RecordCycle(domain.AdaptationPlan{}, 0) // erro só vai para o log
}

// Sem asserções: o objetivo é o detector de data race (go test -race) não acusar nada.
func TestBase_ConcurrentAccess(_ *testing.T) {
	b := New(&mapektest.History{})
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.RecordDispatch(domain.Delta(i%3 - 1))
			b.TickCycle()
			b.LastExecuted()
			b.FinishExecution(domain.ExecutionResult{})
			b.RecordCycle(domain.AdaptationPlan{}, 0)
			b.Executing()
		}()
	}
	wg.Wait()
}

func TestCheckpointer_SavesTableAtEndOfEpisode(t *testing.T) {
	agent := qlearning.NewAgent(mapektest.Settings().Policy, domain.LearningParams{PriorInit: true}, qlearning.Greedy{})
	store := &mapektest.Store{}
	Checkpointer{Agent: agent, Store: store}.EndEpisode()
	if len(store.Saved) != 15 {
		t.Errorf("Q-table salva com %d estados", len(store.Saved))
	}
	store.Err = mapektest.ErrFake
	Checkpointer{Agent: agent, Store: store}.EndEpisode() // erro só vai para o log
}
