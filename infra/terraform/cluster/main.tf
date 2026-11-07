# Cluster kind de um nó. As portas NodePort são expostas no host:
#   30080 -> localhost:8080  (loja, via proxy de medição; alvo do Locust)
#   30090 -> localhost:9090  (Prometheus; consultado pelo gerenciador)
resource "kind_cluster" "this" {
  name           = var.cluster_name
  node_image     = var.node_image
  wait_for_ready = true
  # O kind mescla o contexto kind-<nome> neste arquivo; o gerenciador (no host)
  # e o stack platform usam o mesmo kubeconfig.
  kubeconfig_path = pathexpand(var.kubeconfig_path)

  kind_config {
    kind        = "Cluster"
    api_version = "kind.x-k8s.io/v1alpha4"

    node {
      role = "control-plane"

      extra_port_mappings {
        container_port = 30080
        host_port      = 8080
      }
      extra_port_mappings {
        container_port = 30090
        host_port      = 9090
      }
    }
  }
}
