package storage_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shna/selfscale-rl/manager/storage"
)

const examplePath = "../../../../configs/config.yaml"

func TestLoad_ExampleConfig(t *testing.T) {
	c, err := storage.LoadConfig(examplePath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	s := c.Settings()
	if s.Goals.SLAP95Ms != 200 || s.Policy.MaxReplicas != 5 || s.Policy.ReadinessTimeout != 60*time.Second {
		t.Errorf("settings = %+v", s)
	}
	if c.Sensors.Period != 15*time.Second || c.TargetName() != "boutique/frontend" {
		t.Errorf("config = %+v", c)
	}
}

func writeVariant(t *testing.T, old, replacement string) string {
	t.Helper()
	raw, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw), old, replacement, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_InvalidValues(t *testing.T) {
	cases := []struct{ name, old, replacement, wantMsg string }{
		{"min ≥ max", "max_replicas: 5", "max_replicas: 1", "min_replicas"},
		{"alpha fora do intervalo", "alpha: 0.1", "alpha: 1.5", "alpha"},
		{"faixas invertidas", "low_ratio: 0.5", "low_ratio: 0.9", "low_ratio"},
		{"período zero", "period: 15s", "period: 0s", "period"},
		{"campo desconhecido", "sla_p95_ms: 200", "sla_p95_ms: 200\n  sla_p99_ms: 300", "sla_p99_ms"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := storage.LoadConfig(writeVariant(t, c.old, c.replacement))
			if err == nil || !strings.Contains(err.Error(), c.wantMsg) {
				t.Errorf("err = %v, quer mensagem com %q", err, c.wantMsg)
			}
		})
	}
}

func TestLoad_MissingFile(t *testing.T) {
	if _, err := storage.LoadConfig(filepath.Join(t.TempDir(), "nada.yaml")); err == nil {
		t.Error("esperava erro")
	}
}
