// Package storage é a persistência da Base de Conhecimento: implementa
// knowledge.QTableStore (Q-table em JSON) e knowledge.HistoryWriter (CSV).
package storage

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/shna/selfscale-rl/internal/domain"
	"github.com/shna/selfscale-rl/internal/domain/analysis/qlearning"
)

// JSONQTable guarda a Q-table em um arquivo JSON.
type JSONQTable struct{ Path string }

// Load lê a Q-table. Se o arquivo não existir, o erro satisfaz errors.Is(err, os.ErrNotExist).
func (s JSONQTable) Load() (qlearning.Table, error) {
	raw, err := os.ReadFile(s.Path)
	if err != nil {
		return nil, fmt.Errorf("lendo q-table: %w", err)
	}
	var t qlearning.Table
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("interpretando q-table %s: %w", s.Path, err)
	}
	return t, nil
}

// Save grava a Q-table de forma atômica (arquivo temporário + rename), para
// não corromper o arquivo se o processo cair no meio da escrita.
func (s JSONQTable) Save(t qlearning.Table) error {
	raw, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("serializando q-table: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return fmt.Errorf("criando diretório da q-table: %w", err)
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("gravando q-table: %w", err)
	}
	return os.Rename(tmp, s.Path)
}

// historyHeader são as colunas do histórico (um registro por ciclo).
var historyHeader = []string{
	"t", "cycle", "mode", "scenario", "p95_ms", "error_pct", "rps", "cpu_pct", "replicas", "ready",
	"valid", "kind", "state", "decision", "reason", "epsilon", "plan_target", "blocked_by",
	"executed_delta", "success", "time_to_ready_s", "rolled_back", "reward",
}

// CSVHistory grava um registro por ciclo. Modo e cenário são fixos por execução.
type CSVHistory struct {
	mu       sync.Mutex
	file     *os.File
	w        *csv.Writer
	mode     string
	scenario string
}

// NewCSVHistory cria o arquivo e escreve o cabeçalho.
func NewCSVHistory(path, mode, scenario string) (*CSVHistory, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("criando diretório do histórico: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("criando histórico: %w", err)
	}
	h := &CSVHistory{file: f, w: csv.NewWriter(f), mode: mode, scenario: scenario}
	if err := h.writeRow(historyHeader); err != nil {
		_ = f.Close()
		return nil, err
	}
	return h, nil
}

// Write grava o registro e descarrega no disco (o histórico deve sobreviver a quedas).
func (h *CSVHistory) Write(r domain.CycleRecord) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.writeRow(h.row(r))
}

func (h *CSVHistory) writeRow(row []string) error {
	if err := h.w.Write(row); err != nil {
		return fmt.Errorf("gravando linha do histórico: %w", err)
	}
	h.w.Flush()
	return h.w.Error()
}

func (h *CSVHistory) row(r domain.CycleRecord) []string {
	req := r.Plan.Request
	s := req.Symptom
	success, ttr, rolled := "", "", ""
	if f := r.Finished; f != nil {
		success, ttr, rolled = strconv.FormatBool(f.Success), num(f.TimeToReady.Seconds()), strconv.FormatBool(f.RolledBack)
	}
	return []string{
		s.T.Format(time.RFC3339), strconv.Itoa(s.Cycle), h.mode, h.scenario,
		num(s.P95Ms), num(s.ErrorPct), num(s.RPS), num(s.CPUPct),
		strconv.Itoa(s.Replicas), strconv.Itoa(s.ReadyReplicas), strconv.FormatBool(s.Valid), string(s.Kind),
		req.State.String(), strconv.Itoa(int(req.Delta)), req.Reason, num(req.Epsilon),
		strconv.Itoa(r.Plan.Target), r.Plan.BlockedBy, strconv.Itoa(int(r.Executed)),
		success, ttr, rolled, num(req.Reward),
	}
}

// Close fecha o arquivo.
func (h *CSVHistory) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.file.Close()
}

func num(x float64) string { return strconv.FormatFloat(x, 'f', 3, 64) }
