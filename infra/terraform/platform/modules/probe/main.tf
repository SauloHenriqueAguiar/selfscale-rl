# Proxy de medição na frente do frontend: o Locust entra por localhost:8080
# (NodePort 30080) e o Prometheus coleta as métricas na porta 9100.
locals {
  labels = { app = "probe" }
}

resource "kubernetes_deployment_v1" "probe" {
  metadata {
    name      = "probe"
    namespace = var.namespace
    labels    = local.labels
  }
  spec {
    replicas = 1
    selector { match_labels = local.labels }
    template {
      metadata { labels = local.labels }
      spec {
        container {
          name              = "probe"
          image             = var.image
          image_pull_policy = "IfNotPresent"
          args              = ["--target=http://frontend.${var.namespace}.svc.cluster.local:80"]
          port {
            name           = "http"
            container_port = 8080
          }
          port {
            name           = "metrics"
            container_port = 9100
          }
          # Recursos folgados: o proxy não pode ser o gargalo da medição.
          resources {
            requests = { cpu = "250m", memory = "64Mi" }
            limits   = { cpu = "2", memory = "256Mi" }
          }
          readiness_probe {
            http_get {
              path = "/healthz"
              port = "metrics"
            }
            period_seconds = 5
          }
        }
      }
    }
  }
}

resource "kubernetes_service_v1" "entry" {
  metadata {
    name      = "probe"
    namespace = var.namespace
  }
  spec {
    type     = "NodePort"
    selector = local.labels
    port {
      port        = 80
      target_port = "http"
      node_port   = 30080
    }
  }
}

resource "kubernetes_service_v1" "metrics" {
  metadata {
    name      = "probe-metrics"
    namespace = var.namespace
  }
  spec {
    selector = local.labels
    port {
      port        = 9100
      target_port = "metrics"
    }
  }
}
