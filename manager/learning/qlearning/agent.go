// Package qlearning implementa o agente Q-Learning tabular (sem bibliotecas de ML).
package qlearning

import (
	"fmt"
	"math"

	"github.com/shna/selfscale-rl/manager/mapek/knowledge"
)

// Valores do conhecimento prévio: aumentar quando a latência está alta e
// reduzir quando está baixa. Só aceleram o começo do aprendizado.
const (
	priorScaleUp   = 0.5
	priorScaleDown = 0.3
)

// Table é a Q-table, definida na Base de Conhecimento.
type Table = knowledge.QTable

// Agent decide e aprende sobre a Q-table. Não é seguro para uso concorrente:
// só a goroutine do Analisador o usa.
type Agent struct {
	q        Table
	actions  []knowledge.Delta
	policy   knowledge.Policy
	params   knowledge.LearningParams
	selector ActionSelector
}

// NewAgent cria o agente com a Q-table zerada ou com conhecimento prévio.
func NewAgent(p knowledge.Policy, lp knowledge.LearningParams, sel ActionSelector) *Agent {
	a := &Agent{actions: knowledge.Actions(), policy: p, params: lp, selector: sel}
	a.q = a.newTable()
	if lp.PriorInit {
		a.applyPrior()
	}
	return a
}

func (a *Agent) newTable() Table {
	t := make(Table, knowledge.NumStates(a.policy))
	for i := range t {
		t[i] = make([]float64, len(a.actions))
	}
	return t
}

func (a *Agent) applyPrior() {
	for i := range a.q {
		s := knowledge.StateFromIndex(i, a.policy)
		switch {
		case s.Band == knowledge.BandHigh:
			a.q[i][a.column(+1)] = priorScaleUp
			a.q[i][a.column(+2)] = priorScaleUp
		case s.Band == knowledge.BandLow && s.Replicas > a.policy.MinReplicas:
			a.q[i][a.column(-1)] = priorScaleDown
		}
	}
}

// column devolve o índice da ação ou −1 se Δ não pertence ao conjunto.
func (a *Agent) column(d knowledge.Delta) int {
	for i, x := range a.actions {
		if x == d {
			return i
		}
	}
	return -1
}

// ValidActions é a máscara: colunas cujo Δ mantém as réplicas dentro dos limites.
func (a *Agent) ValidActions(replicas int) []int {
	var valid []int
	for i, d := range a.actions {
		next := replicas + int(d)
		if next >= a.policy.MinReplicas && next <= a.policy.MaxReplicas {
			valid = append(valid, i)
		}
	}
	return valid
}

// Decide escolhe Δ para o estado usando a estratégia de seleção.
func (a *Agent) Decide(s knowledge.State) knowledge.Delta {
	valid := a.ValidActions(s.Replicas)
	best := a.bestColumn(s, valid)
	return a.actions[a.selector.Select(valid, best)]
}

// Best devolve a ação gulosa para o estado (usada para inspecionar a política).
func (a *Agent) Best(s knowledge.State) knowledge.Delta {
	return a.actions[a.bestColumn(s, a.ValidActions(s.Replicas))]
}

// bestColumn devolve a ação válida de maior Q; no empate, a de menor |Δ|
// (preferir manter a configuração atual).
func (a *Agent) bestColumn(s knowledge.State, valid []int) int {
	row := a.q[s.Index(a.policy)]
	best := valid[0]
	for _, c := range valid[1:] {
		if row[c] > row[best] || (row[c] == row[best] && abs(a.actions[c]) < abs(a.actions[best])) {
			best = c
		}
	}
	return best
}

// Learn aplica Q(s,a) ← Q(s,a) + α·[r + γ·max_a′ Q(s′,a′) − Q(s,a)],
// com o máximo restrito às ações válidas em s′. Δ fora do conjunto é ignorado.
func (a *Agent) Learn(s knowledge.State, executed knowledge.Delta, reward float64, next knowledge.State) {
	col := a.column(executed)
	if col < 0 {
		return
	}
	row := a.q[s.Index(a.policy)]
	target := reward + a.params.Gamma*a.maxQ(next)
	row[col] += a.params.Alpha * (target - row[col])
}

func (a *Agent) maxQ(s knowledge.State) float64 {
	row := a.q[s.Index(a.policy)]
	m := math.Inf(-1)
	for _, c := range a.ValidActions(s.Replicas) {
		m = max(m, row[c])
	}
	return m
}

// Epsilon devolve o ε atual da estratégia de seleção.
func (a *Agent) Epsilon() float64 { return a.selector.Epsilon() }

// EndEpisode decai ε (quando a estratégia o faz).
func (a *Agent) EndEpisode() { a.selector.EndEpisode() }

// Table devolve uma cópia da Q-table (para persistir sem expor o estado interno).
func (a *Agent) Table() Table {
	c := a.newTable()
	for i := range a.q {
		copy(c[i], a.q[i])
	}
	return c
}

// SetTable substitui a Q-table, validando as dimensões.
func (a *Agent) SetTable(t Table) error {
	if len(t) != len(a.q) {
		return fmt.Errorf("q-table com %d estados, esperado %d", len(t), len(a.q))
	}
	for i, row := range t {
		if len(row) != len(a.actions) {
			return fmt.Errorf("estado %d com %d ações, esperado %d", i, len(row), len(a.actions))
		}
	}
	a.q = t
	return nil
}

func abs(d knowledge.Delta) knowledge.Delta {
	if d < 0 {
		return -d
	}
	return d
}
