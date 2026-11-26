resource "costgraph_tenant" "production" {
  name = "Production"
}

resource "costgraph_ai_gateway_integration" "inference" {
  tenant_id           = costgraph_tenant.production.id
  provider_name       = "agentgateway"
  provider_account_id = "production-inference"
  display_name        = "Production inference"

  tenant_map = {
    acme = "Acme"
  }
}
