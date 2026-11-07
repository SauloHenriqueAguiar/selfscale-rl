package kube_test

import (
	"context"
	"testing"

	"github.com/shna/selfscale-rl/manager/sensors/kube"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

const ns, name, selector = "boutique", "frontend", "app=frontend"

func deployment(desired, ready int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       appsv1.DeploymentSpec{Replicas: &desired},
		Status:     appsv1.DeploymentStatus{ReadyReplicas: ready},
	}
}

func pod(podName, cpuRequest string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: ns, Labels: map[string]string{"app": "frontend"}},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name:      "server",
			Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(cpuRequest)}},
		}}},
	}
}

func podMetrics(podName, usage string) metricsv1beta1.PodMetrics {
	return metricsv1beta1.PodMetrics{
		ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: ns, Labels: map[string]string{"app": "frontend"}},
		Containers: []metricsv1beta1.ContainerMetrics{{
			Name: "server", Usage: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(usage)},
		}},
	}
}

func TestReplicas(t *testing.T) {
	d := kube.NewSensor(fake.NewSimpleClientset(deployment(3, 2)), metricsfake.NewSimpleClientset(), ns, name, selector)
	desired, ready, err := d.Replicas(context.Background())
	if err != nil || desired != 3 || ready != 2 {
		t.Errorf("Replicas = %d, %d, %v", desired, ready, err)
	}
	missing := kube.NewSensor(fake.NewSimpleClientset(), metricsfake.NewSimpleClientset(), ns, name, selector)
	if _, _, err := missing.Replicas(context.Background()); err == nil {
		t.Error("esperava erro para deployment inexistente")
	}
}

// metricsWith devolve um fake do metrics-server que lista os PodMetrics dados.
// O tracker do fake não indexa PodMetrics corretamente, por isso o reactor.
func metricsWith(items ...metricsv1beta1.PodMetrics) *metricsfake.Clientset {
	m := metricsfake.NewSimpleClientset()
	m.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &metricsv1beta1.PodMetricsList{Items: items}, nil
	})
	return m
}

func TestCPUPercent_AveragesOverPods(t *testing.T) {
	client := fake.NewSimpleClientset(pod("a", "100m"), pod("b", "100m"), pod("sem-requests", "0"))
	metrics := metricsWith(podMetrics("a", "50m"), podMetrics("b", "150m"), podMetrics("sem-requests", "10m"))
	d := kube.NewSensor(client, metrics, ns, name, selector)

	got, err := d.CPUPercent(context.Background())
	if err != nil || got != 100 {
		t.Errorf("CPUPercent = %v, %v; quer 100", got, err)
	}
}

func TestCPUPercent_NoMetrics(t *testing.T) {
	d := kube.NewSensor(fake.NewSimpleClientset(pod("a", "100m")), metricsfake.NewSimpleClientset(), ns, name, selector)
	if _, err := d.CPUPercent(context.Background()); err == nil {
		t.Error("esperava erro sem métricas")
	}
}
