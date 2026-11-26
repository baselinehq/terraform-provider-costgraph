variable "tenant_id" {
  type = string
}

data "costgraph_virtual_tags" "available" {
  tenant_id = var.tenant_id
}

output "virtual_tag_keys" {
  value = data.costgraph_virtual_tags.available.keys
}
