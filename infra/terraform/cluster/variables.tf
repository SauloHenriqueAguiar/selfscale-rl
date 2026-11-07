variable "cluster_name" {
  description = "Nome do cluster kind (o contexto do kubeconfig será kind-<nome>)."
  type        = string
  default     = "selfscale"
}

variable "node_image" {
  description = "Imagem do nó kind (versão do Kubernetes fixada)."
  type        = string
  default     = "kindest/node:v1.31.2"
}

variable "kubeconfig_path" {
  description = "Kubeconfig onde o contexto do cluster é gravado."
  type        = string
  default     = "~/.kube/config"
}
