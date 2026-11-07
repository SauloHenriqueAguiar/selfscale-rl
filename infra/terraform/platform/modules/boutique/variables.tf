variable "overlay_path" {
  description = "Diretório do overlay kustomize da loja."
  type        = string
}

variable "frontend_cpu_request" { type = string }
variable "frontend_cpu_limit" { type = string }
variable "deps_cpu_request" { type = string }
variable "deps_cpu_limit" { type = string }
