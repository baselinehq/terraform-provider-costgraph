data "costgraph_current_organization" "current" {}

output "organization_id" {
  value = data.costgraph_current_organization.current.id
}
