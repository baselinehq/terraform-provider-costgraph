variable "tenant_id" {
  type        = string
  description = "UUID of the tenant authorized by the CostGraph API key."
}

resource "costgraph_virtual_tag" "team" {
  tenant_id   = var.tenant_id
  key         = "team"
  description = "Teams responsible for billing records"

  rules = [
    {
      rule_key   = "payments"
      scope      = "billing"
      expression = "bill.resource_name.startsWith(\"payments-\")"
      value      = "payments"
    },
    {
      rule_key       = "provider-owner"
      scope          = "billing"
      expression     = "\"owner\" in bill.tags"
      inherited_from = "bill.tags[\"owner\"]"
    },
    {
      rule_key   = "aws-fallback"
      scope      = "billing"
      expression = "bill.provider == \"aws\""
      value      = "platform"
      active     = false
    }
  ]
}
