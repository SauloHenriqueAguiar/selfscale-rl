// Package kube é o adaptador do Kubernetes (client-go) para o Deployment
// gerenciado. Cada arquivo corresponde a um papel no MAPE-K:
//
//	sensors.go   sensores do Monitor (réplicas e CPU)
//	effector.go  efetuador do Executor (escala)
//	hpa_guard.go garante que HPA e SelfScale-RL nunca atuem juntos
package kube

import (
	"k8s.io/client-go/kubernetes"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

// Deployment é o adaptador para o Deployment gerenciado.
type Deployment struct {
	client      kubernetes.Interface
	metrics     metricsclient.Interface
	namespace   string
	name        string
	podSelector string // ex.: "app=frontend"
}

// NewDeployment cria o adaptador.
func NewDeployment(c kubernetes.Interface, m metricsclient.Interface, namespace, name, podSelector string) *Deployment {
	return &Deployment{client: c, metrics: m, namespace: namespace, name: name, podSelector: podSelector}
}
