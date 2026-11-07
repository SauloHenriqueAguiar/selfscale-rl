# Baseline E3: aplica o HPA do frontend (CPU 70%, 1–5 réplicas) a partir do
# manifesto versionado em managed-system/hpa, sem duplicar o YAML.
resource "kubernetes_manifest" "hpa" {
  manifest = yamldecode(file(var.manifest_path))
}
