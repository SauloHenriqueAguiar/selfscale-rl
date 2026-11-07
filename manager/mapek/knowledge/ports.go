package knowledge

// QTable é a Q-table: linhas = estados, colunas = Actions(). Fica na Base de
// Conhecimento porque é a "informação da adaptação" que o sistema aprende.
type QTable [][]float64

// QTableStore persiste a Q-table (Repository).
type QTableStore interface {
	Load() (QTable, error)
	Save(t QTable) error
}

// HistoryWriter grava um registro por ciclo (Repository).
type HistoryWriter interface {
	Write(r CycleRecord) error
}
