provider "kubernetes" {
  config_path    = pathexpand(var.kubeconfig_path)
  config_context = var.kubeconfig_context
}

provider "helm" {
  kubernetes {
    config_path    = pathexpand(var.kubeconfig_path)
    config_context = var.kubeconfig_context
  }
}

provider "kustomization" {
  kubeconfig_path = pathexpand(var.kubeconfig_path)
  context         = var.kubeconfig_context
}

module "boutique" {
  source               = "./modules/boutique"
  overlay_path         = "${path.root}/../../../managed-system/boutique/overlay"
  frontend_cpu_request = var.frontend_cpu_request
  frontend_cpu_limit   = var.frontend_cpu_limit
  deps_cpu_request     = var.deps_cpu_request
  deps_cpu_limit       = var.deps_cpu_limit
}

module "monitoring" {
  source          = "./modules/monitoring"
  namespace       = var.namespace
  scrape_interval = var.prometheus_scrape_interval
  depends_on      = [module.boutique]
}

module "probe" {
  source     = "./modules/probe"
  namespace  = var.namespace
  image      = var.probe_image
  depends_on = [module.boutique]
}

module "hpa_baseline" {
  source        = "./modules/hpa_baseline"
  count         = var.enable_hpa ? 1 : 0
  manifest_path = "${path.root}/../../../managed-system/hpa/hpa.yaml"
  depends_on    = [module.boutique, module.monitoring]
}
