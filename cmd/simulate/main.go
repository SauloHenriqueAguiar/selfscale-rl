// Comando simulate: calibra o simulador com o piloto e pré-treina o agente.
//
//	simulate calibrate --pilot data/pilot.csv --model data/sim_model.json
//	simulate train     --config configs/config.yaml --model data/sim_model.json
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/shna/selfscale-rl/manager/learning/qlearning"
	"github.com/shna/selfscale-rl/manager/mapek/knowledge"
	"github.com/shna/selfscale-rl/manager/storage"
	"github.com/shna/selfscale-rl/simulator"
)

// Critério de convergência: janela de episódios e variação relativa máxima da recompensa.
const (
	convergenceWindow    = 20
	convergenceTolerance = 0.05
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "uso: simulate <calibrate|train> [flags]")
		os.Exit(2)
	}
	// O Executor registra cada adaptação em Info; no treino isso vira ruído.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))

	var err error
	switch os.Args[1] {
	case "calibrate":
		err = calibrate(os.Args[2:])
	case "train":
		err = train(os.Args[2:])
	default:
		err = fmt.Errorf("subcomando desconhecido %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func calibrate(args []string) error {
	fs := flag.NewFlagSet("calibrate", flag.ExitOnError)
	pilot := fs.String("pilot", "data/pilot.csv", "CSV do piloto (rps, replicas, p95_ms)")
	out := fs.String("model", "data/sim_model.json", "arquivo do modelo calibrado")
	_ = fs.Parse(args)

	f, err := os.Open(*pilot)
	if err != nil {
		return fmt.Errorf("abrindo piloto: %w", err)
	}
	defer f.Close()
	points, err := simulator.ReadPilotCSV(f)
	if err != nil {
		return err
	}
	model, err := simulator.Calibrate(points, simulator.DefaultModel())
	if err != nil {
		return err
	}
	fmt.Printf("modelo calibrado: μ=%.1f req/s, base=%.1f ms, k=%.1f ms, σ=%.3f\n", model.MuRPS, model.BaseMs, model.KMs, model.Sigma)
	return writeJSON(*out, model)
}

func train(args []string) error {
	fs := flag.NewFlagSet("train", flag.ExitOnError)
	cfgPath := fs.String("config", "configs/config.yaml", "arquivo de configuração")
	modelPath := fs.String("model", "data/sim_model.json", "modelo calibrado (usa o padrão se não existir)")
	curve := fs.String("curve", "data/sim_learning.csv", "curva de aprendizado")
	base := fs.Float64("base", 7, "vazão base dos cenários (req/s): 10 usuários no piloto")
	peak := fs.Float64("peak", 20, "vazão de pico dos cenários (req/s): 30 usuários no piloto")
	episodes := fs.Int("episodes", 2000, "máximo de episódios")
	_ = fs.Parse(args)

	cfg, err := storage.LoadConfig(*cfgPath)
	if err != nil {
		return err
	}
	model, err := loadModel(*modelPath)
	if err != nil {
		return err
	}
	s := cfg.Settings()
	agent := newTrainingAgent(s)
	stats := simulator.Train(simulator.TrainConfig{
		Settings: s, Model: model, Scenarios: simulator.Scenarios(), BaseRPS: *base, PeakRPS: *peak,
		Period: cfg.Sensors.Period, MaxEpisodes: *episodes, Window: convergenceWindow, Tolerance: convergenceTolerance,
	}, agent, rand.New(rand.NewSource(s.Learning.Seed)))

	report(stats, agent, s.Policy)
	if err := (storage.JSONQTable{Path: cfg.Learning.QTablePath}).Save(agent.Table()); err != nil {
		return err
	}
	return writeCurve(*curve, stats)
}

func newTrainingAgent(s knowledge.Settings) *qlearning.Agent {
	l := s.Learning
	sel := qlearning.NewEpsilonGreedy(l.EpsilonStart, l.EpsilonMin, l.EpsilonDecay, l.Seed)
	return qlearning.NewAgent(s.Policy, l, sel)
}

func loadModel(path string) (simulator.Model, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		slog.Warn("modelo calibrado não encontrado; usando o padrão", "path", path)
		return simulator.DefaultModel(), nil
	}
	if err != nil {
		return simulator.Model{}, fmt.Errorf("lendo modelo: %w", err)
	}
	m := simulator.DefaultModel()
	return m, json.Unmarshal(raw, &m)
}

// report imprime o resumo do treino e a política gulosa (estado → ação).
func report(stats []simulator.EpisodeStats, agent *qlearning.Agent, p knowledge.Policy) {
	converged := simulator.Converged(stats, convergenceWindow, convergenceTolerance)
	last := stats[len(stats)-1]
	fmt.Printf("episódios=%d convergiu=%v recompensa_média_final=%.3f ε=%.3f\n", len(stats), converged, last.MeanReward, last.Epsilon)
	fmt.Println("política gulosa (réplicas 1..5):")
	for band := knowledge.BandLow; band <= knowledge.BandHigh; band++ {
		fmt.Printf("  %-4s", band)
		for r := p.MinReplicas; r <= p.MaxReplicas; r++ {
			fmt.Printf(" %+d", agent.Best(knowledge.State{Band: band, Replicas: r}))
		}
		fmt.Println()
	}
}

func writeCurve(path string, stats []simulator.EpisodeStats) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("criando curva: %w", err)
	}
	defer f.Close()
	return simulator.WriteLearningCSV(f, stats)
}

func writeJSON(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}
