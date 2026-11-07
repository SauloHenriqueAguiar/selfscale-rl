// Comando probe: proxy de medição que roda no cluster na frente do frontend.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/shna/selfscale-rl/manager/sensors/probe"
)

const shutdownGrace = 5 * time.Second

func main() {
	target := flag.String("target", "http://frontend.boutique.svc.cluster.local:80", "URL do frontend")
	listen := flag.String("listen", ":8080", "endereço do proxy")
	metricsAddr := flag.String("metrics", ":9100", "endereço das métricas")
	maxIdle := flag.Int("max-idle-conns", 512, "conexões ociosas mantidas com o frontend")
	flag.Parse()

	if err := run(*target, *listen, *metricsAddr, *maxIdle); err != nil {
		slog.Error("probe encerrado com erro", "err", err)
		os.Exit(1)
	}
}

func run(target, listen, metricsAddr string, maxIdle int) error {
	u, err := url.Parse(target)
	if err != nil {
		return err
	}
	reg := prometheus.NewRegistry()
	m := probe.NewMetrics(reg)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	servers := []*http.Server{
		{Addr: listen, Handler: probe.NewHandler(u, probe.NewTransport(maxIdle), m), ReadHeaderTimeout: 10 * time.Second},
		{Addr: metricsAddr, Handler: mux, ReadHeaderTimeout: 10 * time.Second},
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errs := make(chan error, len(servers))
	for _, s := range servers {
		go func() { errs <- s.ListenAndServe() }()
	}
	slog.Info("probe no ar", "listen", listen, "metrics", metricsAddr, "target", target)

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	var all []error
	for _, s := range servers {
		all = append(all, s.Shutdown(shutdownCtx))
	}
	return errors.Join(all...)
}
