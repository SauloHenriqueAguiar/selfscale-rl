package kube

import (
	"context"
	"fmt"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HasHPA indica se algum HPA aponta para o Deployment. O gerenciador se recusa
// a atuar nesse caso: HPA e SelfScale-RL nunca escalam o mesmo Deployment.
func (d *Effector) HasHPA(ctx context.Context) (bool, error) {
	list, err := d.client.AutoscalingV1().HorizontalPodAutoscalers(d.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("listando HPAs: %w", err)
	}
	for _, h := range list.Items {
		if targets(h, d.name) {
			return true, nil
		}
	}
	return false, nil
}

func targets(h autoscalingv1.HorizontalPodAutoscaler, deployment string) bool {
	ref := h.Spec.ScaleTargetRef
	return ref.Kind == "Deployment" && ref.Name == deployment
}
