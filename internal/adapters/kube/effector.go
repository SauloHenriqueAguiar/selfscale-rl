package kube

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Efetuador (executor.Scaler).

// Scale altera as réplicas pelo subrecurso scale (o mesmo que o kubectl scale usa).
func (d *Deployment) Scale(ctx context.Context, replicas int) error {
	deployments := d.client.AppsV1().Deployments(d.namespace)
	scale, err := deployments.GetScale(ctx, d.name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("lendo scale de %s: %w", d.name, err)
	}
	scale.Spec.Replicas = int32(replicas)
	if _, err := deployments.UpdateScale(ctx, d.name, scale, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("atualizando scale de %s para %d: %w", d.name, replicas, err)
	}
	return nil
}
