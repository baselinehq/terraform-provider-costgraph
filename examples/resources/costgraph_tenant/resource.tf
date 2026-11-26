resource "costgraph_tenant" "production" {
  name = "Production"
}

output "production_tenant_id" {
  value = costgraph_tenant.production.id
}
