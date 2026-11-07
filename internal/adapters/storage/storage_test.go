package storage

import (
	"encoding/csv"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shna/selfscale-rl/internal/domain"
	"github.com/shna/selfscale-rl/internal/domain/analysis/qlearning"
)

func TestJSONQTable_SaveLoadRoundTrip(t *testing.T) {
	store := JSONQTable{Path: filepath.Join(t.TempDir(), "sub", "q.json")}
	want := qlearning.Table{{0.1, -0.2, 0, 0.5}, {1, 2, 3, 4}}

	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for i := range want {
		for j := range want[i] {
			if got[i][j] != want[i][j] {
				t.Fatalf("got %v, quer %v", got, want)
			}
		}
	}
}

func TestJSONQTable_LoadErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := (JSONQTable{Path: filepath.Join(dir, "x.json")}).Load(); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("arquivo ausente: err = %v", err)
	}
	bad := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(bad, []byte("{"), 0o600)
	if _, err := (JSONQTable{Path: bad}).Load(); err == nil {
		t.Error("JSON inválido deveria falhar")
	}
}

func TestCSVHistory_HeaderAndRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.csv")
	h, err := NewCSVHistory(path, "eval", "C0")
	if err != nil {
		t.Fatal(err)
	}
	rec := domain.CycleRecord{
		Plan: domain.AdaptationPlan{Target: 3, Request: domain.ChangeRequest{
			Delta: 1, Reason: "policy", Reward: math.NaN(),
			State:   domain.State{Band: domain.BandHigh, Replicas: 2},
			Symptom: domain.Symptom{T: time.Unix(0, 0).UTC(), Cycle: 7, P95Ms: 250, Replicas: 2, Valid: true, Kind: "violation"},
		}},
		Executed: 1,
		Finished: &domain.ExecutionResult{Success: true, TimeToReady: 2 * time.Second},
	}
	if err := h.Write(rec); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}

	f, _ := os.Open(path)
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil || len(rows) != 2 {
		t.Fatalf("linhas = %d, err = %v", len(rows), err)
	}
	if strings.Join(rows[0], ",") != strings.Join(historyHeader, ",") {
		t.Errorf("cabeçalho = %v", rows[0])
	}
	row := map[string]string{}
	for i, col := range rows[0] {
		row[col] = rows[1][i]
	}
	if row["mode"] != "eval" || row["scenario"] != "C0" || row["state"] != "HIGH/2" ||
		row["executed_delta"] != "1" || row["success"] != "true" || row["time_to_ready_s"] != "2.000" || row["reward"] != "NaN" {
		t.Errorf("linha = %v", row)
	}
}

func TestNewCSVHistory_InvalidPath(t *testing.T) {
	file := filepath.Join(t.TempDir(), "arquivo")
	_ = os.WriteFile(file, nil, 0o600)
	if _, err := NewCSVHistory(filepath.Join(file, "h.csv"), "m", "s"); err == nil {
		t.Error("esperava erro ao criar dentro de um arquivo")
	}
}
