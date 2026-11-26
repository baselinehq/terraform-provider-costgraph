variable "tenant_id" {
  type = string
}

data "costgraph_virtual_tag" "team" {
  tenant_id = var.tenant_id
  key       = "team"
}

output "ordered_rules" {
  value = data.costgraph_virtual_tag.team.rules
}
