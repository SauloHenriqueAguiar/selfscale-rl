package analyzer

import "github.com/shna/selfscale-rl/internal/domain"

// Portas do Analisador, implementadas pelo agente Q-Learning
// (domain/analysis/qlearning) e pelo knowledge.Checkpointer.

// Decider escolhe a ação para um estado.
type Decider interface {
	Decide(s domain.State) domain.Delta
	Epsilon() float64
}

// Learner atualiza o conhecimento a partir de uma transição observada.
type Learner interface {
	Learn(s domain.State, executed domain.Delta, reward float64, next domain.State)
}

// EpisodeEnder é avisado ao fim de cada episódio (decair ε, salvar Q-table).
type EpisodeEnder interface {
	EndEpisode()
}
