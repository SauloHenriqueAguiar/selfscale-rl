// Package kube contém os sensores do Kubernetes (client-go): réplicas do
// Deployment gerenciado (spec e prontas) e CPU dos pods (metrics-server).
// A escrita (escala) fica em manager/effectors/kube.
package kube

import (
	"k8s.io/client-go/kubernetes"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

// Sensor lê o estado do Deployment gerenciado.
type Sensor struct {
	client      kubernetes.Interface
	metrics     metricsclient.Interface
	namespace   string
	name        string
	podSelector string // ex.: "app=frontend"
}

// NewSensor cria o sensor.
func NewSensor(c kubernetes.Interface, m metricsclient.Interface, namespace, name, podSelector string) *Sensor {
	return &Sensor{client: c, metrics: m, namespace: namespace, name: name, podSelector: podSelector}
}
