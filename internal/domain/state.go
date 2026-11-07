package domain

import "fmt"

// LatencyBand é a faixa discreta do p95 (a discretização fica em domain/analysis).
type LatencyBand int

// Faixas de latência. A ordem define o índice do estado.
const (
	BandLow LatencyBand = iota
	BandOK
	BandHigh
)

// NumLatencyBands é o número de faixas de latência.
const NumLatencyBands = 3

func (b LatencyBand) String() string {
	return [...]string{"LOW", "OK", "HIGH"}[b]
}

// State é o estado discreto do agente: (faixa de p95, réplicas).
type State struct {
	Band     LatencyBand
	Replicas int
}

func (s State) String() string { return fmt.Sprintf("%s/%d", s.Band, s.Replicas) }

// Index devolve a linha da Q-table: band·span + (réplicas − mínimo).
func (s State) Index(p Policy) int {
	return int(s.Band)*p.ReplicaSpan() + s.Replicas - p.MinReplicas
}

// NumStates é o total de estados para a política dada (15 com 1..5 réplicas).
func NumStates(p Policy) int { return NumLatencyBands * p.ReplicaSpan() }

// StateFromIndex é o inverso de Index.
func StateFromIndex(i int, p Policy) State {
	return State{Band: LatencyBand(i / p.ReplicaSpan()), Replicas: i%p.ReplicaSpan() + p.MinReplicas}
}
