output "kubeconfig_context" {
  description = "Contexto do kubeconfig criado pelo kind."
  value       = "kind-${var.cluster_name}"
}

output "kubeconfig_path" {
  description = "Arquivo kubeconfig do cluster."
  value       = kind_cluster.this.kubeconfig_path
}
