// Package analyzer é a etapa A do MAPE-K ("o que fazer?"): classifica o
// sintoma, calcula a recompensa da ação executada no ciclo anterior, aprende
// e decide Δ ∈ {−1, 0, +1, +2} com Q-Learning.
package analyzer

import (
	"context"
	"log/slog"
	"math"

	"github.com/shna/selfscale-rl/manager/mapek/knowledge"
)

// Options define o comportamento do Analisador conforme o modo de execução.
type Options struct {
	Learn         bool // atualiza a Q-table (finetune, online)
	Act           bool // decide ações (todos os modos exceto observe)
	EpisodeCycles int  // ciclos por episódio (decaimento de ε e checkpoint); 0 desliga
}

// Analyzer é o Analisador.
type Analyzer struct {
	decider  Decider
	learner  Learner
	episodes EpisodeEnder
	k        *knowledge.Base
	settings knowledge.Settings
	opts     Options

	prev        knowledge.State
	hasPrev     bool
	prevPending bool // o estado anterior estava em transição (pods ainda não prontos)
	cycles      int
}

// New cria o Analisador.
func New(d Decider, l Learner, e EpisodeEnder, k *knowledge.Base, s knowledge.Settings, o Options) *Analyzer {
	return &Analyzer{decider: d, learner: l, episodes: e, k: k, settings: s, opts: o}
}

// Handle processa um sintoma e devolve a solicitação de mudança.
func (a *Analyzer) Handle(sym knowledge.Symptom) knowledge.ChangeRequest {
	g, p := a.settings.Goals, a.settings.Policy
	sym.Kind = Classify(sym, g, p)
	req := knowledge.ChangeRequest{T: sym.T, Symptom: sym, Reward: math.NaN(), Epsilon: a.decider.Epsilon()}

	// Validador de política: sem dados confiáveis não se aprende nem se decide,
	// e a cadeia de transições é interrompida.
	if !sym.Valid {
		a.hasPrev = false
		req.Reason = knowledge.ReasonInvalidData
		return req
	}

	req.State = Discretize(sym, g, p)
	if a.hasPrev {
		req.Reward = a.learnFromLastAction(sym, req.State)
	}
	a.prev, a.hasPrev, a.prevPending = req.State, true, sym.ReadyReplicas < sym.Replicas

	req.Reason = knowledge.ReasonObserve
	if a.opts.Act {
		req.Delta, req.Reason = a.decider.Decide(req.State), knowledge.ReasonPolicy
	}
	a.countCycle()
	return req
}

// learnFromLastAction avalia a ação de fato executada (não a proposta: o
// Planejador pode tê-la bloqueado) e atualiza a Q-table se o modo permitir.
// Não aprende a partir de um estado em transição: ali o Planejador força
// "manter" e as réplicas desejadas ainda não atendem; a melhora que vem em
// seguida seria creditada a "manter com latência alta".
func (a *Analyzer) learnFromLastAction(sym knowledge.Symptom, next knowledge.State) float64 {
	executed, previous := a.k.LastExecuted()
	r := Reward(sym.P95Ms, next.Replicas, executed, previous, a.settings.Goals, a.settings.Reward)
	if a.opts.Learn && !a.prevPending {
		a.learner.Learn(a.prev, executed, r, next)
	}
	return r
}

func (a *Analyzer) countCycle() {
	a.cycles++
	if a.opts.Learn && a.opts.EpisodeCycles > 0 && a.cycles%a.opts.EpisodeCycles == 0 {
		slog.Info("fim de episódio", "ciclos", a.cycles, "epsilon", a.decider.Epsilon())
		a.episodes.EndEpisode()
	}
}

// Run lê sintomas de in (chMA) e escreve solicitações em out (chAP).
func (a *Analyzer) Run(ctx context.Context, in <-chan knowledge.Symptom, out chan<- knowledge.ChangeRequest) {
	for {
		select {
		case <-ctx.Done():
			return
		case s := <-in:
			select {
			case out <- a.Handle(s):
			case <-ctx.Done():
				return
			}
		}
	}
}
