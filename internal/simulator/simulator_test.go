package simulator

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/shna/selfscale-rl/internal/domain"
	"github.com/shna/selfscale-rl/internal/domain/analysis/qlearning"
)

const period = 15 * time.Second

var settings = domain.Settings{
	Goals:    domain.Goals{SLAP95Ms: 200, LowRatio: 0.5, RiskRatio: 0.8},
	Policy:   domain.Policy{MinReplicas: 1, MaxReplicas: 5, ScaleDownMaxStep: 1, ScaleDownCooldownCycles: 2, ReadinessTimeout: time.Second},
	Reward:   domain.RewardWeights{SLAOk: 1, SLAViolation: -2, CostPerReplica: 0.1, OscillationPenalty: 0.5},
	Learning: domain.LearningParams{Alpha: 0.1, Gamma: 0.9, EpsilonStart: 1, EpsilonMin: 0.05, EpsilonDecay: 0.99, Seed: 42},
}

func TestModel_P95GrowsWithLoadAndSaturates(t *testing.T) {
	m := DefaultModel()
	low, high := m.P95Ms(10, 2), m.P95Ms(35, 2)
	if !(low < high && high < m.MaxMs) {
		t.Errorf("p95 baixo=%v alto=%v", low, high)
	}
	if m.P95Ms(100, 2) != m.MaxMs || m.P95Ms(10, 0) != m.MaxMs {
		t.Error("saturação deve devolver MaxMs")
	}
}

func TestScenarios_Shapes(t *testing.T) {
	cases := []struct {
		name  string
		cycle int
		want  float64
	}{
		{"C0", 0, 10}, {"C0", 8, 100}, {"C0", 28, 10},
		{"C1", 20, 10},
		{"C2", 0, 10}, {"C2", 20, 100}, {"C2", 30, 55},
		{"C3", 0, 10}, {"C3", 8, 100}, {"C3", 12, 10},
		{"C4", 0, 10}, {"C4", 8, 100},
	}
	for _, c := range cases {
		s, err := ScenarioByName(c.name)
		if err != nil {
			t.Fatal(err)
		}
		if got := s.Load(10, 100)(c.cycle); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s ciclo %d = %v, quer %v", c.name, c.cycle, got, c.want)
		}
	}
	if _, err := ScenarioByName("C9"); err == nil {
		t.Error("esperava erro para cenário desconhecido")
	}
}

func TestCluster_ScaleUpWaitsReadinessScaleDownIsImmediate(t *testing.T) {
	m := DefaultModel()
	m.Sigma, m.MinReadyS, m.MaxReadyS = 0, 20, 20 // 2 ciclos
	c := NewCluster(m, func(int) float64 { return 30 }, period, 1, rand.New(rand.NewSource(1)))
	ctx := context.Background()

	p95 := func() float64 { tr, _ := c.Traffic(ctx); return tr.P95Seconds * 1000 }
	if p95() != m.MaxMs {
		t.Fatal("1 réplica deveria saturar com 30 req/s")
	}
	_ = c.Scale(ctx, 3)
	if d, r, _ := c.Replicas(ctx); d != 3 || r != 1 {
		t.Errorf("Monitor deve ver 3 desejadas e 1 pronta, viu %d/%d", d, r)
	}
	if d, r, _ := c.ExecutorView().Replicas(ctx); d != 3 || r != 3 {
		t.Errorf("Executor deve ver a escala aceita, viu %d/%d", d, r)
	}
	c.Advance()
	if p95() != m.MaxMs {
		t.Error("réplicas novas não deveriam atender antes de ficarem prontas")
	}
	c.Advance()
	if p95() >= m.MaxMs {
		t.Error("após 2 ciclos as réplicas deveriam atender")
	}
	_ = c.Scale(ctx, 2)
	if d, r, _ := c.Replicas(ctx); d != 2 || r != 2 || c.Cycle() != 2 {
		t.Errorf("réplicas = %d/%d", d, r)
	}
	if err := c.Scale(ctx, 0); err == nil {
		t.Error("esperava erro para 0 réplicas")
	}
	if cpu, _ := c.CPUPercent(ctx); cpu != 75 {
		t.Errorf("CPU = %v, quer 75", cpu)
	}
}

func TestCluster_NoTrafficIsIdle(t *testing.T) {
	c := NewCluster(DefaultModel(), func(int) float64 { return 0 }, period, 1, rand.New(rand.NewSource(1)))
	tr, _ := c.Traffic(context.Background())
	if !math.IsNaN(tr.P95Seconds) || tr.RPS != 0 {
		t.Errorf("Traffic = %+v", tr)
	}
}

func TestCalibrate_RecoversModel(t *testing.T) {
	truth := DefaultModel()
	truth.MuRPS, truth.BaseMs, truth.KMs = 25, 50, 15
	var b strings.Builder
	b.WriteString("rps,replicas,p95_ms,cpu_frontend\n")
	for n := 1; n <= 5; n++ {
		for _, rho := range []float64{0.2, 0.4, 0.6, 0.8, 1.2} {
			rps := rho * float64(n) * truth.MuRPS
			fmt.Fprintf(&b, "%.4f,%d,%.4f,0\n", rps, n, truth.P95Ms(rps, n))
		}
	}
	points, err := ReadPilotCSV(strings.NewReader(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Calibrate(points, DefaultModel())
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got.MuRPS-25) > 0.5 || math.Abs(got.BaseMs-50) > 2 || math.Abs(got.KMs-15) > 2 {
		t.Errorf("modelo = %+v", got)
	}
}

func TestCalibrate_Errors(t *testing.T) {
	if _, err := ReadPilotCSV(strings.NewReader("rps,replicas\n1,1\n")); err == nil {
		t.Error("coluna ausente deveria falhar")
	}
	if _, err := ReadPilotCSV(strings.NewReader("rps,replicas,p95_ms\n")); err == nil {
		t.Error("csv vazio deveria falhar")
	}
	if _, err := ReadPilotCSV(strings.NewReader("rps,replicas,p95_ms\nx,1,1\n")); err == nil {
		t.Error("número inválido deveria falhar")
	}
	if _, err := Calibrate([]PilotPoint{{RPS: 1, Replicas: 1, P95Ms: 50}}, DefaultModel()); err == nil {
		t.Error("poucos pontos deveriam falhar")
	}
}

// Teste determinístico (semente fixa) sem conhecimento prévio: o agente deve
// aprender sozinho a aumentar com latência alta e reduzir com latência baixa.
func TestTrain_LearnsSensiblePolicy(t *testing.T) {
	cfg := TrainConfig{
		Settings: settings, Model: DefaultModel(), Scenarios: Scenarios(),
		BaseRPS: 5, PeakRPS: 50, Period: period, MaxEpisodes: 400, Window: 20, Tolerance: 0.05,
	}
	sel := qlearning.NewEpsilonGreedy(1, 0.05, 0.99, 42)
	agent := qlearning.NewAgent(settings.Policy, settings.Learning, sel)

	stats := Train(cfg, agent, rand.New(rand.NewSource(42)))

	if len(stats) == 0 {
		t.Fatal("nenhum episódio")
	}
	var csvOut strings.Builder
	if err := WriteLearningCSV(&csvOut, stats); err != nil || strings.Count(csvOut.String(), "\n") != len(stats)+1 {
		t.Errorf("curva de aprendizado: err=%v", err)
	}
	if d := agent.Best(domain.State{Band: domain.BandHigh, Replicas: 1}); d <= 0 {
		t.Errorf("HIGH/1 → %d, quer aumentar", d)
	}
	if d := agent.Best(domain.State{Band: domain.BandLow, Replicas: 5}); d >= 0 {
		t.Errorf("LOW/5 → %d, quer reduzir", d)
	}
}

func TestConverged(t *testing.T) {
	stable := make([]EpisodeStats, 40)
	for i := range stable {
		stable[i] = EpisodeStats{MeanReward: 0.5}
	}
	if !Converged(stable, 20, 0.05) {
		t.Error("deveria convergir")
	}
	stable[35].PolicyChanges = 1
	if Converged(stable, 20, 0.05) {
		t.Error("mudança de política impede convergência")
	}
	if Converged(stable[:10], 20, 0.05) {
		t.Error("poucos episódios não convergem")
	}
}
