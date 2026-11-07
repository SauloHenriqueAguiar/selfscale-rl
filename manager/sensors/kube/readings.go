package kube

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Replicas devolve spec.replicas e status.readyReplicas.
func (d *Sensor) Replicas(ctx context.Context) (desired, ready int, err error) {
	dep, err := d.client.AppsV1().Deployments(d.namespace).Get(ctx, d.name, metav1.GetOptions{})
	if err != nil {
		return 0, 0, fmt.Errorf("lendo deployment %s/%s: %w", d.namespace, d.name, err)
	}
	if dep.Spec.Replicas != nil {
		desired = int(*dep.Spec.Replicas)
	}
	return desired, int(dep.Status.ReadyReplicas), nil
}

// CPUPercent devolve a média, entre os pods, de uso/requests.cpu em %.
func (d *Sensor) CPUPercent(ctx context.Context) (float64, error) {
	requests, err := d.cpuRequestsMilli(ctx)
	if err != nil {
		return 0, err
	}
	list, err := d.metrics.MetricsV1beta1().PodMetricses(d.namespace).List(ctx, metav1.ListOptions{LabelSelector: d.podSelector})
	if err != nil {
		return 0, fmt.Errorf("lendo métricas de pods: %w", err)
	}
	var sum float64
	var n int
	for _, pm := range list.Items {
		req, ok := requests[pm.Name]
		if !ok || req == 0 {
			continue // pod sem requests (ou recém-criado): não dá para normalizar
		}
		var usage int64
		for _, c := range pm.Containers {
			usage += c.Usage.Cpu().MilliValue()
		}
		sum += 100 * float64(usage) / float64(req)
		n++
	}
	if n == 0 {
		return 0, errors.New("nenhum pod com métricas de CPU")
	}
	return sum / float64(n), nil
}

// cpuRequestsMilli devolve requests.cpu (em milicores) de cada pod do frontend.
func (d *Sensor) cpuRequestsMilli(ctx context.Context) (map[string]int64, error) {
	pods, err := d.client.CoreV1().Pods(d.namespace).List(ctx, metav1.ListOptions{LabelSelector: d.podSelector})
	if err != nil {
		return nil, fmt.Errorf("listando pods: %w", err)
	}
	out := make(map[string]int64, len(pods.Items))
	for _, p := range pods.Items {
		for _, c := range p.Spec.Containers {
			out[p.Name] += c.Resources.Requests.Cpu().MilliValue()
		}
	}
	return out, nil
}
