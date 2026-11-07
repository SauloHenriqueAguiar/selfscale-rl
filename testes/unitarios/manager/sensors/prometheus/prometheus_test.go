package prometheus_test

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shna/selfscale-rl/manager/sensors/prometheus"
)

func vector(value string) string {
	return fmt.Sprintf(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000.0,"%s"]}]}}`, value)
}

const emptyVector = `{"status":"success","data":{"resultType":"vector","result":[]}}`

// route associa um trecho da consulta à resposta; a primeira que casar vence.
type route struct{ contains, body string }

// server responde conforme o trecho presente na consulta.
func server(t *testing.T, status int, routes []route) *prometheus.Source {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		w.WriteHeader(status)
		for _, rt := range routes {
			if strings.Contains(q, rt.contains) {
				fmt.Fprint(w, rt.body)
				return
			}
		}
		fmt.Fprint(w, emptyVector)
	}))
	t.Cleanup(srv.Close)
	return prometheus.NewSource(srv.URL, 15*time.Second, srv.Client())
}

func TestTraffic_Success(t *testing.T) {
	src := server(t, http.StatusOK, []route{
		{"histogram_quantile", vector("0.180")},
		{"5xx|error", vector("0.01")},
		{"probe_requests_total[15s]", vector("42.5")},
	})
	got, err := src.Traffic(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.P95Seconds != 0.18 || got.RPS != 42.5 || got.ErrorRatio != 0.01 {
		t.Errorf("Traffic = %+v", got)
	}
}

func TestTraffic_EmptyResultMeansNoTraffic(t *testing.T) {
	got, err := server(t, http.StatusOK, nil).Traffic(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !math.IsNaN(got.P95Seconds) || got.RPS != 0 || got.ErrorRatio != 0 {
		t.Errorf("Traffic = %+v", got)
	}
}

func TestTraffic_Errors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"erro HTTP", http.StatusBadRequest, `{"status":"error","error":"bad query"}`},
		{"JSON inválido", http.StatusOK, `{`},
		{"valor não numérico", http.StatusOK, vector("abc")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := server(t, c.status, []route{{"histogram_quantile", c.body}})
			if _, err := src.Traffic(context.Background()); err == nil {
				t.Error("esperava erro")
			}
		})
	}
}

func TestTraffic_Unreachable(t *testing.T) {
	src := prometheus.NewSource("http://127.0.0.1:1", time.Second, http.DefaultClient)
	if _, err := src.Traffic(context.Background()); err == nil {
		t.Error("esperava erro de conexão")
	}
}
