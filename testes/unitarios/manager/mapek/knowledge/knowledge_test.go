package knowledge_test

import (
	"sync"
	"testing"

	"github.com/shna/selfscale-rl/manager/learning/qlearning"
	"github.com/shna/selfscale-rl/manager/mapek/knowledge"
	"github.com/shna/selfscale-rl/testes/fakes"
)

func TestBase_CyclesSinceScaleUp(t *testing.T) {
	b := knowledge.New(&fakes.History{})
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
	h := &fakes.History{}
	b := knowledge.New(h)
	b.FinishExecution(knowledge.ExecutionResult{Success: true})

	b.RecordCycle(knowledge.AdaptationPlan{Empty: true}, 0)
	b.RecordCycle(knowledge.AdaptationPlan{Empty: true}, 0)

	recs := h.Records()
	if recs[0].Finished == nil || recs[1].Finished != nil {
		t.Error("a execução concluída deve aparecer em um único registro")
	}
	h.Err = fakes.ErrFake
	b.RecordCycle(knowledge.AdaptationPlan{}, 0) // erro só vai para o log
}

// Sem asserções: o objetivo é o detector de data race (go test -race) não acusar nada.
func TestBase_ConcurrentAccess(_ *testing.T) {
	b := knowledge.New(&fakes.History{})
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.RecordDispatch(knowledge.Delta(i%3 - 1))
			b.TickCycle()
			b.LastExecuted()
			b.FinishExecution(knowledge.ExecutionResult{})
			b.RecordCycle(knowledge.AdaptationPlan{}, 0)
			b.Executing()
		}()
	}
	wg.Wait()
}

func TestCheckpointer_SavesTableAtEndOfEpisode(t *testing.T) {
	agent := qlearning.NewAgent(fakes.Settings().Policy, knowledge.LearningParams{PriorInit: true}, qlearning.Greedy{})
	store := &fakes.Store{}
	knowledge.Checkpointer{Agent: agent, Store: store}.EndEpisode()
	if len(store.Saved) != 15 {
		t.Errorf("Q-table salva com %d estados", len(store.Saved))
	}
	store.Err = fakes.ErrFake
	knowledge.Checkpointer{Agent: agent, Store: store}.EndEpisode() // erro só vai para o log
}
