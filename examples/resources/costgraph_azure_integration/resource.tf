variable "azure_client_secret" {
  type      = string
  sensitive = true
  ephemeral = true
  default   = null
}

resource "costgraph_azure_integration" "production" {
  tenant_id        = "00000000-0000-0000-0000-000000000001"
  display_name     = "Production Azure"
  azure_tenant_id  = "00000000-0000-0000-0000-000000000002"
  client_id        = "00000000-0000-0000-0000-000000000003"
  client_secret_wo = var.azure_client_secret
  secret_version   = 1
  storage_account  = "examplebilling"
  container        = "focus-exports"
  export_name      = "costgraph-focus"
}
