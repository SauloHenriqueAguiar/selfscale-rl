# metrics-server (CPU dos pods, usada pelo HPA e registrada pelo gerenciador)
# e um Prometheus mínimo que só coleta o proxy de medição.
resource "helm_release" "metrics_server" {
  name       = "metrics-server"
  namespace  = "kube-system"
  repository = "https://kubernetes-sigs.github.io/metrics-server/"
  chart      = "metrics-server"
  version    = "3.12.2"

  values = [yamlencode({
    args = ["--kubelet-insecure-tls"] # necessário no kind (certificados autoassinados)
  })]
}

resource "helm_release" "prometheus" {
  name             = "prometheus"
  namespace        = "monitoring"
  create_namespace = true
  repository       = "https://prometheus-community.github.io/helm-charts"
  chart            = "prometheus"
  version          = "25.30.1"

  values = [yamlencode({
    alertmanager             = { enabled = false }
    prometheus-pushgateway   = { enabled = false }
    prometheus-node-exporter = { enabled = false }
    kube-state-metrics       = { enabled = false }
    server = {
      persistentVolume = { enabled = false }
      # O timeout padrão do chart (10s) não pode exceder o intervalo de coleta.
      global = {
        scrape_interval     = var.scrape_interval
        scrape_timeout      = var.scrape_interval
        evaluation_interval = var.scrape_interval
      }
      service = { type = "NodePort", nodePort = 30090 }
    }
    serverFiles = {
      "prometheus.yml" = {
        scrape_configs = [{
          job_name       = "probe"
          static_configs = [{ targets = ["probe-metrics.${var.namespace}.svc:9100"] }]
        }]
      }
    }
  })]
}
