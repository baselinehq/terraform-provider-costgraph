resource "costgraph_focus_push_integration" "billing" {
  provider_name       = "aws"
  provider_account_id = "123456789012"
  display_name        = "Production billing"
}

output "ingestion_connection_id" {
  value = costgraph_focus_push_integration.billing.connection_id
}
