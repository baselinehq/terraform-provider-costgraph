terraform {
  required_version = ">= 1.11.0"

  required_providers {
    costgraph = {
      source = "baselinehq/costgraph"
    }
  }
}

provider "costgraph" {}

variable "azure_tenant_id" {
  description = "Microsoft Entra tenant ID, distinct from the CostGraph tenant ID."
  type        = string
}

variable "azure_client_id" {
  description = "Client ID of the service principal allowed to read the billing export."
  type        = string
}

variable "azure_client_secret" {
  description = "Service principal secret supplied through the execution environment."
  type        = string
  sensitive   = true
  ephemeral   = true
}

resource "costgraph_tenant" "production" {
  name = "Production"
}

resource "costgraph_azure_integration" "production" {
  tenant_id        = costgraph_tenant.production.id
  display_name     = "Production Azure"
  azure_tenant_id  = var.azure_tenant_id
  client_id        = var.azure_client_id
  client_secret_wo = var.azure_client_secret
  secret_version   = 1
  storage_account  = "costgraphbilling"
  container        = "focus-exports"
  export_name      = "costgraph-focus"
}
