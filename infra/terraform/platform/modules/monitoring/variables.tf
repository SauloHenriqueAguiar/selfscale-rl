variable "namespace" {
  description = "Namespace onde está o Service do proxy (alvo da coleta)."
  type        = string
}

variable "scrape_interval" {
  type = string
}
