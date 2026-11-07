//go:build integration

// Testes de integração: rodam contra o cluster kind real.
// Execute com: make test-integration (go test -tags=integration ./testes/integracao/...).
package integracao_test

import (
	"context"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"

	effectors "github.com/shna/selfscale-rl/manager/effectors/kube"
	sensors "github.com/shna/selfscale-rl/manager/sensors/kube"
)

const (
	namespace  = "boutique"
	deployment = "frontend"
	selector   = "app=frontend"
)

// Requer o cluster de `make cluster-up && make deploy`, sem HPA no frontend.
func newClusterClients(t *testing.T) (*effectors.Effector, *sensors.Sensor) {
	t.Helper()
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{CurrentContext: "kind-selfscale"}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	if err != nil {
		t.Skipf("cluster indisponível: %v", err)
	}
	client := kubernetes.NewForConfigOrDie(cfg)
	return effectors.NewEffector(client, namespace, deployment),
		sensors.NewSensor(client, metricsclient.NewForConfigOrDie(cfg), namespace, deployment, selector)
}

func waitReady(ctx context.Context, t *testing.T, s *sensors.Sensor, want int) {
	t.Helper()
	for {
		if _, ready, err := s.Replicas(ctx); err == nil && ready >= want {
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
	effector, sensor := newClusterClients(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	if has, err := effector.HasHPA(ctx); err != nil || has {
		t.Fatalf("HPA no frontend (%v): rode make hpa-off", err)
	}
	original, _, err := sensor.Replicas(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = effector.Scale(context.Background(), original) })

	if err := effector.Scale(ctx, 2); err != nil {
		t.Fatal(err)
	}
	waitReady(ctx, t, sensor, 2)
	if _, err := sensor.CPUPercent(ctx); err != nil {
		t.Logf("CPU ainda indisponível (metrics-server leva ~1 min): %v", err)
	}
}
