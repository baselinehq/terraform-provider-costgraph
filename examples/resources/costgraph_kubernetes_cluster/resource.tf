provider "costgraph" {}

variable "tenant_id" {
  description = "Tenant UUID associated with the operator's collection credential."
  type        = string
}

resource "costgraph_kubernetes_cluster" "production" {
  name      = "production-aks"
  tenant_id = var.tenant_id
}
