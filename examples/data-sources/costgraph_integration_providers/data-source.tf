data "costgraph_integration_providers" "available" {}

output "provider_names" {
  value = [for provider in data.costgraph_integration_providers.available.providers : provider.name]
}
