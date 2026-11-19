// Package probe é o sensor de latência implantado no sistema gerenciado: um
// reverse proxy na frente do frontend que mede a latência vista pelo cliente,
// sem alterar requisições nem respostas. O Prometheus coleta essas métricas.
package probe

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Buckets de 5 ms a 5 s, densos perto do SLA (0.1, 0.2, 0.3) para o
// histogram_quantile interpolar bem o p95.
func latencyBuckets() []float64 {
	return []float64{0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.15, 0.2, 0.25, 0.3, 0.4, 0.5, 0.75, 1, 2.5, 5}
}

// Classe de status usada quando o frontend não respondeu (erro de conexão).
const codeError = "error"

// Metrics agrupa as métricas expostas pelo proxy.
type Metrics struct {
	duration prometheus.Histogram
	requests *prometheus.CounterVec
}

// NewMetrics cria e registra as métricas no registrador informado.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		duration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "probe_request_duration_seconds", Help: "Duração das requisições ao frontend.",
			Buckets: latencyBuckets(),
		}),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "probe_requests_total", Help: "Requisições por classe de status.",
		}, []string{"code"}),
	}
	reg.MustRegister(m.duration, m.requests)
	return m
}

// NewHandler devolve o proxy instrumentado para o alvo.
func NewHandler(target *url.URL, transport http.RoundTripper, m *Metrics) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = transport
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		if rec, ok := w.(*statusRecorder); ok {
			rec.failed = true
		}
		w.WriteHeader(http.StatusBadGateway)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		proxy.ServeHTTP(rec, r)
		m.duration.Observe(time.Since(start).Seconds())
		m.requests.WithLabelValues(rec.class()).Inc()
	})
}

// statusRecorder guarda o status escrito e se o frontend ficou inacessível.
type statusRecorder struct {
	http.ResponseWriter
	status int
	failed bool
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// class devolve "2xx".."5xx" ou "error" quando o frontend não respondeu.
func (s *statusRecorder) class() string {
	if s.failed {
		return codeError
	}
	return strconv.Itoa(s.status/100) + "xx"
}

// NewTransport cria o transporte até o frontend sem keep-alive: cada requisição
// abre uma conexão nova. O Service do Kubernetes escolhe o pod por conexão;
// com conexões reaproveitadas, as abertas antes de uma escala ficavam presas
// aos pods antigos e os pods novos não recebiam tráfego (medido: 1 pod com
// 197m de CPU e 7 com 1m). Dentro do cluster, abrir a conexão custa < 1 ms.
func NewTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DisableKeepAlives = true
	return t
}
