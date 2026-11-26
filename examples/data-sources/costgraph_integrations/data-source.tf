data "costgraph_integrations" "current" {}

output "integration_ids" {
  value = [for integration in data.costgraph_integrations.current.integrations : integration.id]
}
