variable "connection_uuid" {
  type = string
}

data "costgraph_integration" "existing" {
  id = var.connection_uuid
}

output "connection_name" {
  value = data.costgraph_integration.existing.display_name
}
