package qlearning

import "math/rand"

// ActionSelector escolhe uma ação (índice de coluna) entre as válidas.
// É o ponto de variação (Strategy) entre treino (exploração) e avaliação.
type ActionSelector interface {
	Select(valid []int, best int) int
	Epsilon() float64
	EndEpisode()
}

// Greedy sempre escolhe a melhor ação.
type Greedy struct{}

// Select devolve a melhor ação.
func (Greedy) Select(_ []int, best int) int { return best }

// Epsilon é sempre zero.
func (Greedy) Epsilon() float64 { return 0 }

// EndEpisode não faz nada.
func (Greedy) EndEpisode() {}

// EpsilonGreedy explora com probabilidade ε e decai ε a cada episódio.
// Com decay = 1, ε fica fixo (modo eval).
type EpsilonGreedy struct {
	epsilon, min, decay float64
	rng                 *rand.Rand
}

// NewEpsilonGreedy cria o seletor com gerador próprio (semente fixa => reprodutível).
func NewEpsilonGreedy(start, minEpsilon, decay float64, seed int64) *EpsilonGreedy {
	return &EpsilonGreedy{epsilon: start, min: minEpsilon, decay: decay, rng: rand.New(rand.NewSource(seed))}
}

// Select explora uma ação válida aleatória com probabilidade ε; senão, a melhor.
func (e *EpsilonGreedy) Select(valid []int, best int) int {
	if e.rng.Float64() < e.epsilon {
		return valid[e.rng.Intn(len(valid))]
	}
	return best
}

// Epsilon devolve o ε atual.
func (e *EpsilonGreedy) Epsilon() float64 { return e.epsilon }

// EndEpisode aplica ε ← max(ε_min, ε·decay).
func (e *EpsilonGreedy) EndEpisode() { e.epsilon = max(e.min, e.epsilon*e.decay) }
