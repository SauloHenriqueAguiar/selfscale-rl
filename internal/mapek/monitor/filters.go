package monitor

import (
	"math"
	"time"

	"github.com/shna/selfscale-rl/internal/domain"
)

// Processador de Eventos do Monitor, no estilo Pipes and Filters:
// Normalize converte a leitura bruta em sintoma e cada Processor é um filtro.

// Sample é a leitura bruta do Coletor, nas unidades de cada fonte.
type Sample struct {
	T        time.Time
	Cycle    int
	Traffic  domain.Traffic
	CPUPct   float64
	Replicas int
	Ready    int
	Failed   bool // alguma fonte essencial (tráfego ou réplicas) falhou
}

// Processor é um filtro do pipeline.
type Processor func(domain.Symptom) domain.Symptom

// DefaultProcessors é o pipeline padrão, aplicado depois de Normalize.
func DefaultProcessors() []Processor {
	return []Processor{FillIdleLatency, FilterInvalid}
}

// Normalize converte a leitura bruta em sintoma: s → ms e fração → %.
func Normalize(s Sample) domain.Symptom {
	return domain.Symptom{
		T:             s.T,
		Cycle:         s.Cycle,
		P95Ms:         s.Traffic.P95Seconds * 1000,
		ErrorPct:      s.Traffic.ErrorRatio * 100,
		RPS:           s.Traffic.RPS,
		CPUPct:        s.CPUPct,
		Replicas:      s.Replicas,
		ReadyReplicas: s.Ready,
		Valid:         !s.Failed,
	}
}

// FillIdleLatency trata a ausência de tráfego: sem requisições o p95 é
// indefinido (NaN no Prometheus), mas a situação é ociosa e válida.
func FillIdleLatency(s domain.Symptom) domain.Symptom {
	if s.RPS == 0 {
		s.P95Ms, s.ErrorPct = 0, 0
	}
	return s
}

// FilterInvalid marca como inválido o sintoma com valores ausentes ou impossíveis.
func FilterInvalid(s domain.Symptom) domain.Symptom {
	for _, v := range []float64{s.P95Ms, s.ErrorPct, s.RPS} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			s.Valid = false
		}
	}
	if s.Replicas < 0 || s.ReadyReplicas < 0 {
		s.Valid = false
	}
	return s
}
