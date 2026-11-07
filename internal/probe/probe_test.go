package probe

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func newProxy(t *testing.T, backend http.Handler) (*httptest.Server, *Metrics) {
	t.Helper()
	srv := httptest.NewServer(backend)
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	m := NewMetrics(prometheus.NewRegistry())
	proxy := httptest.NewServer(NewHandler(target, NewTransport(10), m))
	t.Cleanup(proxy.Close)
	return proxy, m
}

func TestHandler_ForwardsRequestAndKeepsResponse(t *testing.T) {
	proxy, m := newProxy(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Path", r.URL.Path+"?"+r.URL.RawQuery)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "olá")
	}))

	resp, err := http.Get(proxy.URL + "/product/1?x=2")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated || string(body) != "olá" || resp.Header.Get("X-Path") != "/product/1?x=2" {
		t.Errorf("resposta alterada: %d %q %q", resp.StatusCode, body, resp.Header.Get("X-Path"))
	}
	if got := testutil.ToFloat64(m.requests.WithLabelValues("2xx")); got != 1 {
		t.Errorf("probe_requests_total{code=2xx} = %v", got)
	}
	if n := testutil.CollectAndCount(m.duration); n != 1 {
		t.Errorf("histograma não registrado")
	}
}

func TestHandler_ClassifiesStatus(t *testing.T) {
	proxy, m := newProxy(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "fail") {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	for _, p := range []string{"/", "/fail", "/fail"} {
		resp, err := http.Get(proxy.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if got := testutil.ToFloat64(m.requests.WithLabelValues("5xx")); got != 2 {
		t.Errorf("5xx = %v, quer 2", got)
	}
}

func TestHandler_UnreachableBackendCountsAsError(t *testing.T) {
	target, _ := url.Parse("http://127.0.0.1:1")
	m := NewMetrics(prometheus.NewRegistry())
	proxy := httptest.NewServer(NewHandler(target, NewTransport(1), m))
	defer proxy.Close()

	resp, err := http.Get(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway || testutil.ToFloat64(m.requests.WithLabelValues(codeError)) != 1 {
		t.Errorf("status=%d", resp.StatusCode)
	}
}
