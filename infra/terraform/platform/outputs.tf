output "store_url" {
  description = "Loja, através do proxy de medição."
  value       = "http://localhost:8080"
}

output "prometheus_url" {
  description = "Prometheus (consultado pelo gerenciador)."
  value       = "http://localhost:9090"
}

output "hpa_enabled" {
  value = var.enable_hpa
}
