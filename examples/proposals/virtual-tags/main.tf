terraform {
  required_providers {
    costgraph = {
      source = "baselinehq/costgraph"
    }
  }
}

provider "costgraph" {}

variable "tenant_id" {
  description = "Tenant containing the cost data and virtual tags."
  type        = string
}

resource "costgraph_virtual_tag" "team" {
  tenant_id   = var.tenant_id
  key         = "team"
  description = "Assign AWS spend to its owning team."

  rules = [
    {
      rule_key   = "payments-service"
      scope      = "billing"
      expression = "bill.resource_name.startsWith(\"payments-\")"
      value      = "payments"
      active     = true
    },
    {
      rule_key       = "existing-owner"
      scope          = "billing"
      expression     = "\"owner\" in bill.tags"
      inherited_from = "bill.tags[\"owner\"]"
      active         = true
    },
    {
      rule_key   = "remaining-aws"
      scope      = "billing"
      expression = "bill.provider == \"aws\""
      value      = "platform"
      active     = true
    }
  ]
}

data "costgraph_virtual_tag" "existing_environment" {
  tenant_id = var.tenant_id
  key       = "environment"
}

data "costgraph_virtual_tags" "available" {
  tenant_id = var.tenant_id
}
