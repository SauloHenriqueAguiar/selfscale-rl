variable "namespace" { type = string }

variable "image" {
  description = "Imagem do proxy (carregada no kind, não vem de registry)."
  type        = string
}
