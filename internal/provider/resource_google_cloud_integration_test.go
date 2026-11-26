package provider

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccGoogleCloudIntegration(t *testing.T) {
	client := cloudAcceptanceClient(t)
	tenantID := requireCloudTestEnv(t, "COSTGRAPH_TEST_TENANT_ID")
	accountID := requireCloudTestEnv(t, "COSTGRAPH_TEST_GCP_BILLING_ACCOUNT_ID")
	project := requireCloudTestEnv(t, "COSTGRAPH_TEST_GCP_QUERY_PROJECT")
	principal := requireCloudTestEnv(t, "COSTGRAPH_TEST_GCP_TARGET_PRINCIPAL")
	name := fmt.Sprintf("Terraform Google Cloud acceptance %d", time.Now().UnixNano())
	config := func(displayName *string) string {
		display := ""
		if displayName != nil {
			display = fmt.Sprintf("display_name = %q", *displayName)
		}
		return fmt.Sprintf(`
provider "costgraph" {}

resource "costgraph_google_cloud_integration" "test" {
  tenant_id = %q
  billing_account_id = %q
  query_project = %q
  target_principal = %q
  enable_recommendations = false
  %s
}
`, tenantID, accountID, project, principal, display)
	}
	const resourceName = "costgraph_google_cloud_integration.test"
	var createdID, deletedID string
	checkPersisted := func(expectedName string, remember bool) resource.TestCheckFunc {
		return func(state *terraform.State) error {
			record, ok := state.RootModule().Resources[resourceName]
			if !ok {
				return fmt.Errorf("Google Cloud connection is missing from Terraform state")
			}
			settings, err := client.GetGoogleCloudIntegrationSettings(context.Background(), record.Primary.ID)
			if err != nil {
				return err
			}
			if settings.TenantID != tenantID || settings.BillingAccountID != accountID || settings.DisplayName != expectedName || settings.Impersonation.QueryProject != project || settings.Impersonation.TargetPrincipal != principal || settings.EnableRecommendations {
				return fmt.Errorf("Google Cloud connection readback differs from the Terraform configuration")
			}
			if remember {
				createdID = settings.ID
			} else if settings.ID != createdID {
				return fmt.Errorf("updating the Google Cloud connection changed its UUID")
			}
			return nil
		}
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"costgraph": providerserver.NewProtocol6WithError(New("test")()),
		},
		CheckDestroy: func(state *terraform.State) error {
			for _, record := range state.RootModule().Resources {
				if record.Type != "costgraph_google_cloud_integration" {
					continue
				}
				if _, err := client.GetConnection(context.Background(), record.Primary.ID); !isNotFound(err) {
					return fmt.Errorf("Google Cloud connection was not confirmed absent after destroy")
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: config(nil), Check: checkPersisted(accountID, true)},
			{Config: config(nil), PlanOnly: true},
			{ResourceName: resourceName, ImportState: true, ImportStateVerify: true},
			{Config: config(&name), Check: checkPersisted(name, false)},
			{Config: config(&name), PlanOnly: true},
			{Config: config(nil), Check: checkPersisted(name, false)},
			{
				Config: config(nil),
				PreConfig: func() {
					if createdID == "" {
						t.Fatal("Google Cloud connection identity was not recorded before external deletion")
					}
					deletedID = createdID
					if err := client.DeleteConnection(context.Background(), createdID); err != nil {
						t.Fatal(err)
					}
					if _, err := client.GetConnection(context.Background(), createdID); !isNotFound(err) {
						t.Fatal("externally deleted Google Cloud connection remains accessible")
					}
				},
				Check: func(state *terraform.State) error {
					if state.RootModule().Resources[resourceName].Primary.ID == deletedID {
						return fmt.Errorf("externally deleted Google Cloud connection was not recreated")
					}
					return checkPersisted(accountID, true)(state)
				},
			},
		},
	})
}
