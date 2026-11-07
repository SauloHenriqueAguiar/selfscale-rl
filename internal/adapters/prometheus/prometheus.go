// Package prometheus é um sensor do Monitor: implementa monitor.TrafficSource
// consultando a API HTTP do Prometheus (sem biblioteca cliente: GET + JSON).
package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/shna/selfscale-rl/internal/domain"
)

// Source consulta as métricas do proxy de medição.
type Source struct {
	baseURL string
	window  string
	client  *http.Client
}

// NewSource cria a fonte; a janela das consultas é o período do ciclo.
func NewSource(baseURL string, window time.Duration, client *http.Client) *Source {
	return &Source{baseURL: baseURL, window: fmt.Sprintf("%ds", int(window.Seconds())), client: client}
}

// Consultas PromQL na janela do ciclo (a taxa de erros sai como fração; o Monitor converte para %).
func (s *Source) queries() (p95, rps, errRatio string) {
	w := s.window
	p95 = fmt.Sprintf(`histogram_quantile(0.95, sum(rate(probe_request_duration_seconds_bucket[%s])) by (le))`, w)
	rps = fmt.Sprintf(`sum(rate(probe_requests_total[%s]))`, w)
	errRatio = fmt.Sprintf(`sum(rate(probe_requests_total{code=~"5xx|error"}[%s])) / sum(rate(probe_requests_total[%s]))`, w, w)
	return p95, rps, errRatio
}

// Traffic implementa monitor.TrafficSource. Resultado vazio significa "sem
// requisições": vazão e erros viram 0 e o p95 fica NaN (o Monitor decide).
func (s *Source) Traffic(ctx context.Context) (domain.Traffic, error) {
	qp95, qrps, qerr := s.queries()
	var t domain.Traffic
	var err error
	if t.P95Seconds, err = s.query(ctx, qp95); err != nil {
		return t, err
	}
	if t.RPS, err = s.query(ctx, qrps); err != nil {
		return t, err
	}
	if t.ErrorRatio, err = s.query(ctx, qerr); err != nil {
		return t, err
	}
	t.RPS, t.ErrorRatio = zeroIfNaN(t.RPS), zeroIfNaN(t.ErrorRatio)
	return t, nil
}

type apiResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		Result []struct {
			Value [2]any `json:"value"` // [timestamp, "valor"]
		} `json:"result"`
	} `json:"data"`
}

// query executa uma consulta instantânea e devolve o primeiro valor (NaN se vazio).
func (s *Source) query(ctx context.Context, promQL string) (float64, error) {
	u := s.baseURL + "/api/v1/query?query=" + url.QueryEscape(promQL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, fmt.Errorf("montando consulta: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("consultando prometheus: %w", err)
	}
	defer resp.Body.Close()

	var body apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, fmt.Errorf("decodificando resposta (HTTP %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK || body.Status != "success" {
		return 0, fmt.Errorf("prometheus respondeu HTTP %d: %s", resp.StatusCode, body.Error)
	}
	if len(body.Data.Result) == 0 {
		return math.NaN(), nil
	}
	return parseValue(body.Data.Result[0].Value[1])
}

func parseValue(v any) (float64, error) {
	str, ok := v.(string)
	if !ok {
		return 0, fmt.Errorf("valor inesperado %v", v)
	}
	f, err := strconv.ParseFloat(str, 64) // aceita "NaN" e "+Inf"
	if err != nil {
		return 0, fmt.Errorf("convertendo %q: %w", str, err)
	}
	return f, nil
}

func zeroIfNaN(x float64) float64 {
	if math.IsNaN(x) {
		return 0
	}
	return x
}
