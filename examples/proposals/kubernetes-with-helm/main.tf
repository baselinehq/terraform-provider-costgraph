terraform {
  required_providers {
    costgraph = {
      source = "baselinehq/costgraph"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "3.3.0"
    }
  }
}

provider "costgraph" {}

provider "helm" {
  kubernetes = {
    config_path    = pathexpand(var.kubeconfig_path)
    config_context = var.kube_context
  }
}

variable "costgraph_tenant_id" {
  description = "Existing tenant associated with the collector credential."
  type        = string
}

variable "kubeconfig_path" {
  description = "Path to the kubeconfig used to install the operator."
  type        = string
  default     = "~/.kube/config"
}

variable "kube_context" {
  description = "Explicit kubeconfig context for the target cluster."
  type        = string
}

variable "collector_namespace" {
  description = "Existing namespace containing the collector Secret."
  type        = string
  default     = "costgraph"
}

variable "collector_secret_name" {
  description = "Existing Secret containing the tenant's collector credential under apiKey."
  type        = string
  default     = "costgraph-collector"
}

variable "collector_api_url" {
  description = "CostGraph ingestion API URL accessible from the Kubernetes cluster."
  type        = string
  default     = "https://api.costgraph.ai"
}

data "costgraph_tenant" "production" {
  id = var.costgraph_tenant_id
}

resource "costgraph_kubernetes_cluster" "production" {
  name      = "production-aks"
  tenant_id = data.costgraph_tenant.production.id
}

resource "helm_release" "costgraph_operator" {
  name       = "costgraph-operator"
  repository = "https://charts.costgraph.ai"
  chart      = "costgraph-operator"
  version    = "0.8.27"
  namespace  = var.collector_namespace

  values = [yamlencode({
    global = {
      clusterName       = costgraph_kubernetes_cluster.production.name
      existingSecret    = var.collector_secret_name
      existingSecretKey = "apiKey"
      backendURL        = var.collector_api_url
    }
  })]
}
