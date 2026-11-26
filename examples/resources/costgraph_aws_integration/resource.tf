resource "costgraph_aws_integration" "production" {
  tenant_id                     = "00000000-0000-0000-0000-000000000001"
  billing_account_id            = "123456789012"
  display_name                  = "Production AWS"
  region                        = "us-east-2"
  bucket                        = "example-focus-export"
  prefix                        = "billing/FOCUS/"
  role_arn                      = "arn:aws:iam::123456789012:role/CostGraphRead"
  activate_cost_allocation_tags = false
}
