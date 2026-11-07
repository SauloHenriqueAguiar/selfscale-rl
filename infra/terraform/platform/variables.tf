variable "kubeconfig_path" {
  description = "Kubeconfig com o contexto do cluster kind."
  type        = string
  default     = "~/.kube/config"
}

variable "kubeconfig_context" {
  description = "Contexto do cluster."
  type        = string
  default     = "kind-selfscale"
}

variable "namespace" {
  description = "Namespace da loja e do proxy de medição."
  type        = string
  default     = "boutique"
}

variable "frontend_cpu_request" {
  description = "requests.cpu do frontend (calibrar no piloto: o frontend deve saturar antes das dependências)."
  type        = string
  default     = "100m"
}

variable "frontend_cpu_limit" {
  description = "limits.cpu do frontend (calibrar no piloto)."
  type        = string
  default     = "200m"
}

variable "deps_cpu_request" {
  description = "requests.cpu das dependências (folgado)."
  type        = string
  default     = "200m"
}

variable "deps_cpu_limit" {
  description = "limits.cpu das dependências (calibrar no piloto: < 50% de uso no pico)."
  type        = string
  default     = "1000m"
}

variable "probe_image" {
  description = "Imagem do proxy de medição, carregada no kind por `make probe-image`."
  type        = string
  default     = "selfscale/probe:dev"
}

variable "enable_hpa" {
  description = "Liga o baseline HPA (E3). Nunca usar junto com o SelfScale-RL atuando."
  type        = bool
  default     = false
}

variable "prometheus_scrape_interval" {
  description = "Intervalo de coleta do proxy pelo Prometheus."
  type        = string
  default     = "5s"
}
