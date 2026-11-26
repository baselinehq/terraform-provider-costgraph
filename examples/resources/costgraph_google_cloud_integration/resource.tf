resource "costgraph_google_cloud_integration" "production" {
  tenant_id          = "00000000-0000-0000-0000-000000000001"
  billing_account_id = "012345-ABCDEF-6789AB"
  display_name       = "Production Google Cloud"
  query_project      = "billing-export-project"
  target_principal   = "costgraph-reader@billing-export-project.iam.gserviceaccount.com"
}
