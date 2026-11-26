variable "tenant_id" {
  type = string
}

data "costgraph_tenant" "current" {
  id = var.tenant_id
}

output "tenant_name" {
  value = data.costgraph_tenant.current.name
}
