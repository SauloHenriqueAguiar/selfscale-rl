package mapek

import "fmt"

// Mode descreve o que o laço faz em cada modo de execução.
type Mode struct {
	Name  string
	Learn bool // atualiza a Q-table
	Act   bool // decide e aplica ações
}

// ParseMode converte o nome do modo.
func ParseMode(name string) (Mode, error) {
	switch name {
	case "observe": // só mede e classifica (também usado nos baselines fixo e HPA)
		return Mode{Name: name}, nil
	case "eval": // atua com a política aprendida, sem aprender
		return Mode{Name: name, Act: true}, nil
	case "finetune", "online": // atua e aprende
		return Mode{Name: name, Act: true, Learn: true}, nil
	default:
		return Mode{}, fmt.Errorf("modo desconhecido %q (use observe, finetune, eval ou online)", name)
	}
}
