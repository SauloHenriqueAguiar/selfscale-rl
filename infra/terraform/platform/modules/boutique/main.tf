# Online Boutique: overlay kustomize (managed-system/boutique/overlay) + patches de CPU
# gerados a partir das variáveis, sem duplicar YAML.
locals {
  dependencies = [
    "adservice", "cartservice", "checkoutservice", "currencyservice", "emailservice",
    "paymentservice", "productcatalogservice", "recommendationservice", "shippingservice", "redis-cart",
  ]
  cpu = merge(
    { frontend = { request = var.frontend_cpu_request, limit = var.frontend_cpu_limit } },
    { for d in local.dependencies : d => { request = var.deps_cpu_request, limit = var.deps_cpu_limit } },
  )
}

data "kustomization_overlay" "boutique" {
  resources = [var.overlay_path]

  dynamic "patches" {
    for_each = local.cpu
    content {
      target {
        kind = "Deployment"
        name = patches.key
      }
      # Todos os Deployments da loja têm um único contêiner (índice 0).
      patch = jsonencode([
        { op = "replace", path = "/spec/template/spec/containers/0/resources/requests/cpu", value = patches.value.request },
        { op = "replace", path = "/spec/template/spec/containers/0/resources/limits/cpu", value = patches.value.limit },
      ])
    }
  }
}

# Aplicação em três prioridades (namespace/CRDs → recursos → webhooks), padrão do provider.
resource "kustomization_resource" "p0" {
  for_each = data.kustomization_overlay.boutique.ids_prio[0]
  manifest = data.kustomization_overlay.boutique.manifests[each.value]
}

resource "kustomization_resource" "p1" {
  for_each   = data.kustomization_overlay.boutique.ids_prio[1]
  manifest   = data.kustomization_overlay.boutique.manifests[each.value]
  depends_on = [kustomization_resource.p0]
}

resource "kustomization_resource" "p2" {
  for_each   = data.kustomization_overlay.boutique.ids_prio[2]
  manifest   = data.kustomization_overlay.boutique.manifests[each.value]
  depends_on = [kustomization_resource.p1]
}
