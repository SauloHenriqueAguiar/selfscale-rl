package kube

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
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
	d := NewDeployment(fake.NewSimpleClientset(deployment(3, 2)), metricsfake.NewSimpleClientset(), ns, name, selector)
	desired, ready, err := d.Replicas(context.Background())
	if err != nil || desired != 3 || ready != 2 {
		t.Errorf("Replicas = %d, %d, %v", desired, ready, err)
	}
	missing := NewDeployment(fake.NewSimpleClientset(), metricsfake.NewSimpleClientset(), ns, name, selector)
	if _, _, err := missing.Replicas(context.Background()); err == nil {
		t.Error("esperava erro para deployment inexistente")
	}
}

// O fake clientset não implementa o subrecurso scale; os reactors simulam
// GetScale/UpdateScale e registram o valor pedido.
func TestScale_UsesScaleSubresource(t *testing.T) {
	client := fake.NewSimpleClientset(deployment(1, 1))
	var updated int32
	client.PrependReactor("get", "deployments", func(a k8stesting.Action) (bool, runtime.Object, error) {
		if a.GetSubresource() != "scale" {
			return false, nil, nil
		}
		return true, &autoscalingv1.Scale{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}, nil
	})
	client.PrependReactor("update", "deployments", func(a k8stesting.Action) (bool, runtime.Object, error) {
		s := a.(k8stesting.UpdateAction).GetObject().(*autoscalingv1.Scale)
		updated = s.Spec.Replicas
		return true, s, nil
	})
	d := NewDeployment(client, metricsfake.NewSimpleClientset(), ns, name, selector)

	if err := d.Scale(context.Background(), 4); err != nil || updated != 4 {
		t.Errorf("Scale: err=%v réplicas=%d", err, updated)
	}
}

func TestScale_ErrorWhenDeploymentMissing(t *testing.T) {
	d := NewDeployment(fake.NewSimpleClientset(), metricsfake.NewSimpleClientset(), ns, name, selector)
	if err := d.Scale(context.Background(), 2); err == nil {
		t.Error("esperava erro")
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
	d := NewDeployment(client, metrics, ns, name, selector)

	got, err := d.CPUPercent(context.Background())
	if err != nil || got != 100 {
		t.Errorf("CPUPercent = %v, %v; quer 100", got, err)
	}
}

func TestCPUPercent_NoMetrics(t *testing.T) {
	d := NewDeployment(fake.NewSimpleClientset(pod("a", "100m")), metricsfake.NewSimpleClientset(), ns, name, selector)
	if _, err := d.CPUPercent(context.Background()); err == nil {
		t.Error("esperava erro sem métricas")
	}
}

func TestHasHPA(t *testing.T) {
	hpa := func(target string) *autoscalingv1.HorizontalPodAutoscaler {
		return &autoscalingv1.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: "h-" + target, Namespace: ns},
			Spec: autoscalingv1.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{Kind: "Deployment", Name: target},
			},
		}
	}
	cases := []struct {
		name    string
		objects []runtime.Object
		want    bool
	}{
		{"sem HPA", nil, false},
		{"HPA de outro deployment", []runtime.Object{hpa("cartservice")}, false},
		{"HPA no frontend", []runtime.Object{hpa("cartservice"), hpa(name)}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := NewDeployment(fake.NewSimpleClientset(c.objects...), metricsfake.NewSimpleClientset(), ns, name, selector)
			got, err := d.HasHPA(context.Background())
			if err != nil || got != c.want {
				t.Errorf("HasHPA = %v, %v; quer %v", got, err, c.want)
			}
		})
	}
}
