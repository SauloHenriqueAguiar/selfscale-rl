package analyzer

import "github.com/shna/selfscale-rl/manager/mapek/knowledge"

// Portas do Analisador, implementadas pelo agente Q-Learning
// (manager/learning/qlearning) e pelo knowledge.Checkpointer.

// Decider escolhe a ação para um estado.
type Decider interface {
	Decide(s knowledge.State) knowledge.Delta
	Epsilon() float64
}

// Learner atualiza o conhecimento a partir de uma transição observada.
type Learner interface {
	Learn(s knowledge.State, executed knowledge.Delta, reward float64, next knowledge.State)
}

// EpisodeEnder é avisado ao fim de cada episódio (decair ε, salvar Q-table).
type EpisodeEnder interface {
	EndEpisode()
}
