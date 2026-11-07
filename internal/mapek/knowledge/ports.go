package knowledge

import (
	"github.com/shna/selfscale-rl/internal/domain"
	"github.com/shna/selfscale-rl/internal/domain/analysis/qlearning"
)

// QTableStore persiste a Q-table (Repository).
type QTableStore interface {
	Load() (qlearning.Table, error)
	Save(t qlearning.Table) error
}

// HistoryWriter grava um registro por ciclo (Repository).
type HistoryWriter interface {
	Write(r domain.CycleRecord) error
}
