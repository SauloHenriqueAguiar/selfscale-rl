package storage

// Configuração do operador (configs/config.yaml): metas, políticas, pesos da
// recompensa e hiperparâmetros, convertidos para knowledge.Settings.

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/shna/selfscale-rl/manager/mapek/knowledge"
)

// Config espelha o arquivo YAML.
type Config struct {
	Target struct {
		KubeconfigContext string   `yaml:"kubeconfig_context"`
		Namespace         string   `yaml:"namespace"`
		Deployment        string   `yaml:"deployment"`
		PodSelector       string   `yaml:"pod_selector"`
		Allowlist         []string `yaml:"allowlist"`
	} `yaml:"target"`
	Sensors struct {
		PrometheusURL string        `yaml:"prometheus_url"`
		Period        time.Duration `yaml:"period"`
	} `yaml:"sensors"`
	Goals struct {
		SLAP95Ms float64 `yaml:"sla_p95_ms"`
	} `yaml:"goals"`
	State struct {
		LowRatio  float64 `yaml:"low_ratio"`
		RiskRatio float64 `yaml:"risk_ratio"`
	} `yaml:"state"`
	Policy struct {
		MinReplicas             int           `yaml:"min_replicas"`
		MaxReplicas             int           `yaml:"max_replicas"`
		ScaleDownMaxStep        int           `yaml:"scale_down_max_step"`
		ScaleDownCooldownCycles int           `yaml:"scale_down_cooldown_cycles"`
		ReadinessTimeout        time.Duration `yaml:"readiness_timeout"`
	} `yaml:"policy"`
	Reward struct {
		SLAOk              float64 `yaml:"sla_ok"`
		SLAViolation       float64 `yaml:"sla_violation"`
		CostPerReplica     float64 `yaml:"cost_per_replica"`
		OscillationPenalty float64 `yaml:"oscillation_penalty"`
	} `yaml:"reward"`
	Learning struct {
		Alpha           float64 `yaml:"alpha"`
		Gamma           float64 `yaml:"gamma"`
		EpsilonStart    float64 `yaml:"epsilon_start"`
		EpsilonFinetune float64 `yaml:"epsilon_finetune"`
		EpsilonEval     float64 `yaml:"epsilon_eval"`
		EpsilonMin      float64 `yaml:"epsilon_min"`
		EpsilonDecay    float64 `yaml:"epsilon_decay"`
		PriorInit       bool    `yaml:"prior_init"`
		QTablePath      string  `yaml:"qtable_path"`
		Seed            int64   `yaml:"seed"`
	} `yaml:"learning"`
	Run struct {
		HistoryDir    string `yaml:"history_dir"`
		EpisodeCycles int    `yaml:"episode_cycles"`
	} `yaml:"run"`
}

// LoadConfig lê e valida o arquivo. Campos desconhecidos são erro (evita erro de digitação silencioso).
func LoadConfig(path string) (Config, error) {
	var c Config
	raw, err := os.ReadFile(path)
	if err != nil {
		return c, fmt.Errorf("lendo configuração: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return c, fmt.Errorf("interpretando %s: %w", path, err)
	}
	if err := c.Validate(); err != nil {
		return c, fmt.Errorf("configuração inválida em %s: %w", path, err)
	}
	return c, nil
}

// rule é uma restrição da configuração e a mensagem mostrada se for violada.
type rule struct {
	ok  bool
	msg string
}

// Validate confere as restrições que, se violadas, quebrariam o laço.
// Devolve todos os problemas de uma vez, para o operador corrigir de uma só vez.
func (c Config) Validate() error {
	var errs []error
	for _, r := range slices.Concat(c.generalRules(), c.policyRules(), c.learningRules()) {
		if !r.ok {
			errs = append(errs, errors.New(r.msg))
		}
	}
	return errors.Join(errs...)
}

func (c Config) generalRules() []rule {
	return []rule{
		{c.Target.Namespace != "" && c.Target.Deployment != "", "target.namespace e target.deployment são obrigatórios"},
		{c.Sensors.Period > 0, "sensors.period deve ser > 0"},
		{c.Goals.SLAP95Ms > 0, "goals.sla_p95_ms deve ser > 0"},
		{0 < c.State.LowRatio && c.State.LowRatio < c.State.RiskRatio && c.State.RiskRatio <= 1,
			"exige 0 < state.low_ratio < state.risk_ratio ≤ 1"},
		{c.Run.EpisodeCycles > 0, "run.episode_cycles deve ser > 0"},
	}
}

func (c Config) policyRules() []rule {
	p := c.Policy
	return []rule{
		{p.MinReplicas >= 1 && p.MinReplicas < p.MaxReplicas, "exige 1 ≤ policy.min_replicas < policy.max_replicas"},
		{p.ScaleDownMaxStep >= 1, "policy.scale_down_max_step deve ser ≥ 1"},
		{p.ScaleDownCooldownCycles >= 0, "policy.scale_down_cooldown_cycles deve ser ≥ 0"},
		{p.ReadinessTimeout > 0, "policy.readiness_timeout deve ser > 0"},
	}
}

func (c Config) learningRules() []rule {
	l := c.Learning
	return []rule{
		{0 < l.Alpha && l.Alpha <= 1, "learning.alpha deve estar em (0, 1]"},
		{0 <= l.Gamma && l.Gamma < 1, "learning.gamma deve estar em [0, 1)"},
		{inUnit(l.EpsilonStart) && inUnit(l.EpsilonFinetune) && inUnit(l.EpsilonEval) && inUnit(l.EpsilonMin),
			"valores de epsilon devem estar em [0, 1]"},
		{0 < l.EpsilonDecay && l.EpsilonDecay <= 1, "learning.epsilon_decay deve estar em (0, 1]"},
	}
}

func inUnit(x float64) bool { return 0 <= x && x <= 1 }

// Settings converte a configuração nas structs do domínio.
func (c Config) Settings() knowledge.Settings {
	p, r, l := c.Policy, c.Reward, c.Learning
	return knowledge.Settings{
		Goals: knowledge.Goals{SLAP95Ms: c.Goals.SLAP95Ms, LowRatio: c.State.LowRatio, RiskRatio: c.State.RiskRatio},
		Policy: knowledge.Policy{
			MinReplicas: p.MinReplicas, MaxReplicas: p.MaxReplicas, ScaleDownMaxStep: p.ScaleDownMaxStep,
			ScaleDownCooldownCycles: p.ScaleDownCooldownCycles, ReadinessTimeout: p.ReadinessTimeout,
		},
		Reward: knowledge.RewardWeights{
			SLAOk: r.SLAOk, SLAViolation: r.SLAViolation, CostPerReplica: r.CostPerReplica, OscillationPenalty: r.OscillationPenalty,
		},
		Learning: knowledge.LearningParams{
			Alpha: l.Alpha, Gamma: l.Gamma, EpsilonStart: l.EpsilonStart, EpsilonMin: l.EpsilonMin,
			EpsilonDecay: l.EpsilonDecay, PriorInit: l.PriorInit, Seed: l.Seed,
		},
	}
}

// TargetName é a identificação "namespace/deployment" usada na allowlist.
func (c Config) TargetName() string { return c.Target.Namespace + "/" + c.Target.Deployment }
