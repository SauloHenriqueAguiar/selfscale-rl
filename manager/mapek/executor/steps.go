package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/shna/selfscale-rl/manager/mapek/knowledge"
)

// Command é um passo executável do plano (padrão Command): o Executor só
// percorre a lista, sem conhecer o que cada passo faz.
type Command interface {
	Execute(ctx context.Context) error
}

// ScaleStep altera o número de réplicas.
type ScaleStep struct {
	Scaler   Scaler
	Replicas int
}

// Execute chama o Scaler.
func (s ScaleStep) Execute(ctx context.Context) error {
	if err := s.Scaler.Scale(ctx, s.Replicas); err != nil {
		return fmt.Errorf("escalando para %d: %w", s.Replicas, err)
	}
	return nil
}

// AwaitReadyStep espera até haver Replicas pods prontos (ou o contexto expirar).
type AwaitReadyStep struct {
	Reader   ReplicaReader
	Replicas int
	Poll     time.Duration
}

// Execute consulta as réplicas prontas a cada Poll.
func (s AwaitReadyStep) Execute(ctx context.Context) error {
	ticker := time.NewTicker(s.Poll)
	defer ticker.Stop()
	for {
		if _, ready, err := s.Reader.Replicas(ctx); err == nil && ready >= s.Replicas {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("aguardando %d réplicas prontas: %w", s.Replicas, ctx.Err())
		case <-ticker.C:
		}
	}
}

// ConfirmStep confere que o Deployment ficou exatamente no alvo.
type ConfirmStep struct {
	Reader   ReplicaReader
	Replicas int
}

// Execute verifica réplicas desejadas e prontas.
func (s ConfirmStep) Execute(ctx context.Context) error {
	desired, ready, err := s.Reader.Replicas(ctx)
	if err != nil {
		return fmt.Errorf("confirmando réplicas: %w", err)
	}
	if desired != s.Replicas || ready < s.Replicas {
		return fmt.Errorf("confirmação falhou: desejadas=%d prontas=%d alvo=%d", desired, ready, s.Replicas)
	}
	return nil
}

// buildCommands traduz os passos do plano (dados) em comandos executáveis.
func (e *Executor) buildCommands(steps []knowledge.Step) ([]Command, error) {
	cmds := make([]Command, 0, len(steps))
	for _, st := range steps {
		switch st.Kind {
		case knowledge.StepScale:
			cmds = append(cmds, ScaleStep{Scaler: e.scaler, Replicas: st.Replicas})
		case knowledge.StepAwaitReady:
			cmds = append(cmds, AwaitReadyStep{Reader: e.reader, Replicas: st.Replicas, Poll: e.opts.PollInterval})
		case knowledge.StepConfirm:
			cmds = append(cmds, ConfirmStep{Reader: e.reader, Replicas: st.Replicas})
		default:
			return nil, fmt.Errorf("passo desconhecido %q", st.Kind)
		}
	}
	return cmds, nil
}
