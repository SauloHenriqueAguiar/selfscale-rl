package knowledge

import (
	"log/slog"

	"github.com/shna/selfscale-rl/internal/domain/analysis/qlearning"
)

// TableAgent é o agente visto pelo Checkpointer.
type TableAgent interface {
	EndEpisode()
	Table() qlearning.Table
}

// Checkpointer decai ε e salva a Q-table ao fim de cada episódio
// (implementa analyzer.EpisodeEnder).
type Checkpointer struct {
	Agent TableAgent
	Store QTableStore
}

// EndEpisode decai ε e persiste a Q-table.
func (c Checkpointer) EndEpisode() {
	c.Agent.EndEpisode()
	if err := c.Save(); err != nil {
		slog.Error("falha ao salvar a Q-table", "err", err)
	}
}

// Save persiste a Q-table atual.
func (c Checkpointer) Save() error { return c.Store.Save(c.Agent.Table()) }
