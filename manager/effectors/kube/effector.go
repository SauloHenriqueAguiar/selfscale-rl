// Package kube contém os efetuadores do Kubernetes (client-go): alteram o
// número de réplicas do Deployment gerenciado e verificam que nenhum HPA atua
// sobre ele ao mesmo tempo. A leitura fica em manager/sensors/kube.
package kube

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Effector atua sobre o Deployment gerenciado.
type Effector struct {
	client    kubernetes.Interface
	namespace string
	name      string
}

// NewEffector cria o efetuador.
func NewEffector(c kubernetes.Interface, namespace, name string) *Effector {
	return &Effector{client: c, namespace: namespace, name: name}
}

// Scale implementa executor.Scaler: altera as réplicas pelo subrecurso scale (o mesmo que o kubectl scale usa).
func (d *Effector) Scale(ctx context.Context, replicas int) error {
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
