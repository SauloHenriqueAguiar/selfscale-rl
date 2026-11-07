package simulator

import (
	"context"
	"encoding/csv"
	"io"
	"math"
	"math/rand"
	"strconv"
	"time"

	"github.com/shna/selfscale-rl/manager/learning/qlearning"
	"github.com/shna/selfscale-rl/manager/mapek"
	"github.com/shna/selfscale-rl/manager/mapek/analyzer"
	"github.com/shna/selfscale-rl/manager/mapek/executor"
	"github.com/shna/selfscale-rl/manager/mapek/knowledge"
	"github.com/shna/selfscale-rl/manager/mapek/monitor"
	"github.com/shna/selfscale-rl/manager/mapek/planner"
)

// TrainConfig parametriza o pré-treino.
type TrainConfig struct {
	Settings    knowledge.Settings
	Model       Model
	Scenarios   []Scenario
	BaseRPS     float64
	PeakRPS     float64
	Period      time.Duration
	MaxEpisodes int
	Window      int     // episódios observados no critério de convergência (20)
	Tolerance   float64 // variação relativa máxima da recompensa média (0.05)
}

// EpisodeStats é uma linha da curva de aprendizado.
type EpisodeStats struct {
	Episode       int
	Scenario      string
	MeanReward    float64
	Epsilon       float64
	PolicyChanges int // estados cuja ação gulosa mudou neste episódio
}

// Train roda episódios (um cenário por episódio, em rodízio) até convergir ou
// atingir MaxEpisodes. Usa os mesmos Monitor, Analisador, Planejador e Executor
// do cluster, ligados às portas simuladas.
func Train(cfg TrainConfig, agent *qlearning.Agent, rng *rand.Rand) []EpisodeStats {
	var stats []EpisodeStats
	policy := greedyPolicy(agent, cfg.Settings.Policy)
	for ep := 0; ep < cfg.MaxEpisodes; ep++ {
		sc := cfg.Scenarios[ep%len(cfg.Scenarios)]
		mean := runEpisode(cfg, sc, agent, rng)
		agent.EndEpisode()

		next := greedyPolicy(agent, cfg.Settings.Policy)
		stats = append(stats, EpisodeStats{
			Episode: ep + 1, Scenario: sc.Name, MeanReward: mean,
			Epsilon: agent.Epsilon(), PolicyChanges: countChanges(policy, next),
		})
		policy = next
		// Só faz sentido falar em convergência depois que a exploração acabou.
		if agent.Epsilon() <= cfg.Settings.Learning.EpsilonMin && Converged(stats, cfg.Window, cfg.Tolerance) {
			break
		}
	}
	return stats
}

// runEpisode simula um cenário inteiro e devolve a recompensa média.
func runEpisode(cfg TrainConfig, sc Scenario, agent *qlearning.Agent, rng *rand.Rand) float64 {
	pol := cfg.Settings.Policy
	cluster := NewCluster(cfg.Model, sc.Load(cfg.BaseRPS, cfg.PeakRPS), cfg.Period, pol.MinReplicas, rng)
	loop := newLoop(cfg.Settings, cluster, agent)
	ctx := context.Background()
	start := time.Unix(0, 0)

	var sum float64
	var n int
	for c := 0; c < sc.Cycles; c++ {
		plan := loop.Step(ctx, start.Add(time.Duration(c)*cfg.Period))
		if r := plan.Request.Reward; !math.IsNaN(r) {
			sum, n = sum+r, n+1
		}
		cluster.Advance()
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// newLoop monta um laço novo por episódio (Knowledge e Analisador zerados),
// para que transições não atravessem o reinício do cluster.
func newLoop(s knowledge.Settings, cluster *Cluster, agent *qlearning.Agent) *mapek.Loop {
	k := knowledge.New(Discard{})
	const target = "sim/frontend"
	execOpts := executor.Options{Target: target, Allowlist: []string{target}, PollInterval: time.Millisecond}
	return &mapek.Loop{
		Monitor:  monitor.New(cluster, cluster, cluster, monitor.DefaultProcessors()),
		Analyzer: analyzer.New(agent, agent, nil, k, s, analyzer.Options{Learn: true, Act: true}),
		Planner:  planner.New(k, s.Policy),
		Executor: executor.New(cluster, cluster.ExecutorView(), k, execOpts),
	}
}

// greedyPolicy devolve a ação gulosa de cada estado.
func greedyPolicy(agent *qlearning.Agent, p knowledge.Policy) []knowledge.Delta {
	out := make([]knowledge.Delta, knowledge.NumStates(p))
	for i := range out {
		out[i] = agent.Best(knowledge.StateFromIndex(i, p))
	}
	return out
}

func countChanges(a, b []knowledge.Delta) int {
	n := 0
	for i := range a {
		if a[i] != b[i] {
			n++
		}
	}
	return n
}

// Converged aplica o critério de convergência sobre os últimos 'window' episódios:
// recompensa média estável (variação relativa < tol) e política gulosa inalterada.
func Converged(stats []EpisodeStats, window int, tol float64) bool {
	if window <= 0 || len(stats) < 2*window {
		return false
	}
	recent, before := stats[len(stats)-window:], stats[len(stats)-2*window:len(stats)-window]
	for _, s := range recent {
		if s.PolicyChanges != 0 {
			return false
		}
	}
	a, b := meanReward(recent), meanReward(before)
	return math.Abs(a-b) <= tol*math.Abs(b)
}

func meanReward(stats []EpisodeStats) float64 {
	var sum float64
	for _, s := range stats {
		sum += s.MeanReward
	}
	return sum / float64(len(stats))
}

// Discard é um HistoryWriter que não grava nada (o simulador só registra a curva).
type Discard struct{}

// Write descarta o registro.
func (Discard) Write(knowledge.CycleRecord) error { return nil }

// WriteLearningCSV grava a curva de aprendizado (episódio, cenário, recompensa média, ε, mudanças na política).
func WriteLearningCSV(w io.Writer, stats []EpisodeStats) error {
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"episode", "scenario", "mean_reward", "epsilon", "policy_changes"})
	for _, s := range stats {
		_ = cw.Write([]string{
			strconv.Itoa(s.Episode), s.Scenario, strconv.FormatFloat(s.MeanReward, 'f', 4, 64),
			strconv.FormatFloat(s.Epsilon, 'f', 4, 64), strconv.Itoa(s.PolicyChanges),
		})
	}
	cw.Flush()
	return cw.Error()
}
