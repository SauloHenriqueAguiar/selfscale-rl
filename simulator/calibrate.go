package simulator

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
)

// PilotPoint é uma medição do piloto com réplicas fixas.
type PilotPoint struct {
	RPS      float64
	Replicas int
	P95Ms    float64
}

// ReadPilotCSV lê as colunas rps, replicas e p95_ms (demais colunas são ignoradas).
func ReadPilotCSV(r io.Reader) ([]PilotPoint, error) {
	rows, err := csv.NewReader(r).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("lendo csv do piloto: %w", err)
	}
	if len(rows) < 2 {
		return nil, errors.New("csv do piloto sem dados")
	}
	col := map[string]int{}
	for i, name := range rows[0] {
		col[name] = i
	}
	for _, name := range []string{"rps", "replicas", "p95_ms"} {
		if _, ok := col[name]; !ok {
			return nil, fmt.Errorf("coluna %q ausente no csv do piloto", name)
		}
	}
	points := make([]PilotPoint, 0, len(rows)-1)
	for i, row := range rows[1:] {
		p, err := parsePoint(row, col)
		if err != nil {
			return nil, fmt.Errorf("linha %d: %w", i+2, err)
		}
		points = append(points, p)
	}
	return points, nil
}

func parsePoint(row []string, col map[string]int) (PilotPoint, error) {
	rps, err1 := strconv.ParseFloat(row[col["rps"]], 64)
	replicas, err2 := strconv.Atoi(row[col["replicas"]])
	p95, err3 := strconv.ParseFloat(row[col["p95_ms"]], 64)
	if err := errors.Join(err1, err2, err3); err != nil {
		return PilotPoint{}, err
	}
	return PilotPoint{RPS: rps, Replicas: replicas, P95Ms: p95}, nil
}

// Faixa e passo da busca por μ (req/s por réplica).
const (
	muSearchMin  = 1.0
	muSearchMax  = 500.0
	muSearchStep = 0.5
)

// Calibrate ajusta μ, base, k e σ ao piloto. Para cada μ candidato, o modelo
// p95 = base + k·x (x = ρ/(1−ρ)) é linear, então base e k saem de mínimos
// quadrados simples; fica o μ de menor erro. Pontos observados em saturação
// (p95 ≥ MaxMs) não entram, e um μ que preveja saturação para algum ponto
// não saturado é descartado — assim todos os candidatos usam os mesmos pontos.
func Calibrate(points []PilotPoint, start Model) (Model, error) {
	points = start.unsaturated(points)
	if len(points) < 3 {
		return start, errors.New("dados insuficientes para calibrar (≥ 3 pontos não saturados)")
	}
	best, bestSSE := start, math.Inf(1)
	for mu := muSearchMin; mu <= muSearchMax; mu += muSearchStep {
		m := start
		m.MuRPS = mu
		xs, ok := m.queueFactors(points)
		if !ok {
			continue
		}
		base, k, sse := linearFit(xs, p95s(points))
		if k > 0 && base > 0 && sse < bestSSE {
			m.BaseMs, m.KMs = base, k
			best, bestSSE = m, sse
		}
	}
	if math.IsInf(bestSSE, 1) {
		return start, errors.New("nenhum μ ajustou os dados do piloto")
	}
	best.Sigma = relativeNoise(best, points)
	return best, nil
}

func (m Model) unsaturated(points []PilotPoint) []PilotPoint {
	var out []PilotPoint
	for _, p := range points {
		if p.P95Ms < m.MaxMs {
			out = append(out, p)
		}
	}
	return out
}

// queueFactors devolve x = ρ/(1−ρ) de cada ponto; ok = false se algum saturar.
func (m Model) queueFactors(points []PilotPoint) ([]float64, bool) {
	xs := make([]float64, len(points))
	for i, p := range points {
		rho := m.Utilization(p.RPS, p.Replicas)
		if rho >= m.MaxRho {
			return nil, false
		}
		xs[i] = queueFactor(rho)
	}
	return xs, true
}

func p95s(points []PilotPoint) []float64 {
	ys := make([]float64, len(points))
	for i, p := range points {
		ys[i] = p.P95Ms
	}
	return ys
}

// linearFit devolve intercepto, inclinação e soma dos quadrados dos resíduos.
func linearFit(xs, ys []float64) (a, b, sse float64) {
	n := float64(len(xs))
	var sx, sy, sxx, sxy float64
	for i := range xs {
		sx, sy, sxx, sxy = sx+xs[i], sy+ys[i], sxx+xs[i]*xs[i], sxy+xs[i]*ys[i]
	}
	den := n*sxx - sx*sx
	if den == 0 {
		return 0, 0, math.Inf(1)
	}
	b = (n*sxy - sx*sy) / den
	a = (sy - b*sx) / n
	for i := range xs {
		r := ys[i] - (a + b*xs[i])
		sse += r * r
	}
	return a, b, sse
}

// relativeNoise é o desvio-padrão de observado/previsto (ruído multiplicativo).
func relativeNoise(m Model, points []PilotPoint) float64 {
	var ratios []float64
	for _, p := range points {
		if pred := m.P95Ms(p.RPS, p.Replicas); pred < m.MaxMs {
			ratios = append(ratios, p.P95Ms/pred)
		}
	}
	if len(ratios) < 2 {
		return m.Sigma
	}
	var mean, ss float64
	for _, r := range ratios {
		mean += r / float64(len(ratios))
	}
	for _, r := range ratios {
		ss += (r - mean) * (r - mean)
	}
	return math.Sqrt(ss / float64(len(ratios)-1))
}
