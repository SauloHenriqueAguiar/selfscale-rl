package kube_test

import (
	"context"
	"testing"

	"github.com/shna/selfscale-rl/manager/effectors/kube"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const ns, name = "boutique", "frontend"

func deployment(desired, ready int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       appsv1.DeploymentSpec{Replicas: &desired},
		Status:     appsv1.DeploymentStatus{ReadyReplicas: ready},
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
	d := kube.NewEffector(client, ns, name)

	if err := d.Scale(context.Background(), 4); err != nil || updated != 4 {
		t.Errorf("Scale: err=%v réplicas=%d", err, updated)
	}
}

func TestScale_ErrorWhenDeploymentMissing(t *testing.T) {
	d := kube.NewEffector(fake.NewSimpleClientset(), ns, name)
	if err := d.Scale(context.Background(), 2); err == nil {
		t.Error("esperava erro")
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
			d := kube.NewEffector(fake.NewSimpleClientset(c.objects...), ns, name)
			got, err := d.HasHPA(context.Background())
			if err != nil || got != c.want {
				t.Errorf("HasHPA = %v, %v; quer %v", got, err, c.want)
			}
		})
	}
}
