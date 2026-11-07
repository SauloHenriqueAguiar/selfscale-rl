package probe_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/shna/selfscale-rl/manager/sensors/probe"
)

// newProxy sobe um proxy na frente de backend e devolve o registro de métricas,
// lido como o Prometheus leria.
func newProxy(t *testing.T, backend http.Handler) (*httptest.Server, *prometheus.Registry) {
	t.Helper()
	srv := httptest.NewServer(backend)
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	return newProxyTo(t, target)
}

func newProxyTo(t *testing.T, target *url.URL) (*httptest.Server, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	proxy := httptest.NewServer(probe.NewHandler(target, probe.NewTransport(10), probe.NewMetrics(reg)))
	t.Cleanup(proxy.Close)
	return proxy, reg
}

// requests devolve probe_requests_total{code=...}.
func requests(t *testing.T, reg *prometheus.Registry, code string) float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() != "probe_requests_total" {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "code" && l.GetValue() == code {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

// observations devolve quantas durações o histograma registrou.
func observations(t *testing.T, reg *prometheus.Registry) uint64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() == "probe_request_duration_seconds" {
			return f.GetMetric()[0].GetHistogram().GetSampleCount()
		}
	}
	return 0
}

func get(t *testing.T, u string) *http.Response {
	t.Helper()
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestHandler_ForwardsRequestAndKeepsResponse(t *testing.T) {
	proxy, reg := newProxy(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Path", r.URL.Path+"?"+r.URL.RawQuery)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "olá")
	}))

	resp := get(t, proxy.URL+"/product/1?x=2")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated || string(body) != "olá" || resp.Header.Get("X-Path") != "/product/1?x=2" {
		t.Errorf("resposta alterada: %d %q %q", resp.StatusCode, body, resp.Header.Get("X-Path"))
	}
	if got := requests(t, reg, "2xx"); got != 1 {
		t.Errorf("probe_requests_total{code=2xx} = %v", got)
	}
	if n := observations(t, reg); n != 1 {
		t.Errorf("histograma com %d observações, quer 1", n)
	}
}

func TestHandler_ClassifiesStatus(t *testing.T) {
	proxy, reg := newProxy(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "fail") {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	for _, p := range []string{"/", "/fail", "/fail"} {
		get(t, proxy.URL+p).Body.Close()
	}
	if got := requests(t, reg, "5xx"); got != 2 {
		t.Errorf("5xx = %v, quer 2", got)
	}
}

func TestHandler_UnreachableBackendCountsAsError(t *testing.T) {
	target, _ := url.Parse("http://127.0.0.1:1")
	proxy, reg := newProxyTo(t, target)

	resp := get(t, proxy.URL)
	resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway || requests(t, reg, "error") != 1 {
		t.Errorf("status=%d error=%v", resp.StatusCode, requests(t, reg, "error"))
	}
}
