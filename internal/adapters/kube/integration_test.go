//go:build integration

package kube

import (
	"context"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

// Requer o cluster de `make cluster-up && make deploy`, sem HPA no frontend.
func newClusterDeployment(t *testing.T) *Deployment {
	t.Helper()
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{CurrentContext: "kind-selfscale"}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	if err != nil {
		t.Skipf("cluster indisponível: %v", err)
	}
	return NewDeployment(kubernetes.NewForConfigOrDie(cfg), metricsclient.NewForConfigOrDie(cfg), ns, name, selector)
}

func waitReady(ctx context.Context, t *testing.T, d *Deployment, want int) {
	t.Helper()
	for {
		if _, ready, err := d.Replicas(ctx); err == nil && ready >= want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("frontend não ficou com %d réplicas prontas", want)
		case <-time.After(2 * time.Second):
		}
	}
}

func TestIntegration_ScaleRoundTrip(t *testing.T) {
	d := newClusterDeployment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	if has, err := d.HasHPA(ctx); err != nil || has {
		t.Fatalf("HPA no frontend (%v): rode make hpa-off", err)
	}
	original, _, err := d.Replicas(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Scale(context.Background(), original) })

	if err := d.Scale(ctx, 2); err != nil {
		t.Fatal(err)
	}
	waitReady(ctx, t, d, 2)
	if _, err := d.CPUPercent(ctx); err != nil {
		t.Logf("CPU ainda indisponível (metrics-server leva ~1 min): %v", err)
	}
}
