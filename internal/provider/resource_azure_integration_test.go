package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	costgraph "github.com/baselinehq/costgraph-go"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccAzureIntegration(t *testing.T) {
	client := cloudAcceptanceClient(t)
	tenantID := requireCloudTestEnv(t, "COSTGRAPH_TEST_TENANT_ID")
	azureTenantID := requireCloudTestEnv(t, "COSTGRAPH_TEST_AZURE_TENANT_ID")
	clientID := requireCloudTestEnv(t, "COSTGRAPH_TEST_AZURE_CLIENT_ID")
	initialSecret := requireCloudTestEnv(t, "COSTGRAPH_TEST_AZURE_CLIENT_SECRET")
	rotatedSecret := requireCloudTestEnv(t, "COSTGRAPH_TEST_AZURE_ROTATED_CLIENT_SECRET")
	storageAccount := requireCloudTestEnv(t, "COSTGRAPH_TEST_AZURE_STORAGE_ACCOUNT")
	container := requireCloudTestEnv(t, "COSTGRAPH_TEST_AZURE_CONTAINER")
	exportName := requireCloudTestEnv(t, "COSTGRAPH_TEST_AZURE_EXPORT_NAME")
	if initialSecret == rotatedSecret {
		t.Fatal("Provide two different, simultaneously valid secrets for the isolated Azure service principal")
	}
	t.Setenv("TF_VAR_azure_client_secret", initialSecret)
	name := fmt.Sprintf("Terraform Azure acceptance %d", time.Now().UnixNano())
	config := func(displayName *string, folder string, version int64, includeSecret bool) string {
		display, rotation, secret := "", "", ""
		if displayName != nil {
			display = fmt.Sprintf("display_name = %q", *displayName)
		}
		if version >= 0 {
			rotation = fmt.Sprintf("secret_version = %d", version)
		}
		if includeSecret {
			secret = "client_secret_wo = var.azure_client_secret"
		}
		return fmt.Sprintf(`
provider "costgraph" {}

variable "azure_client_secret" {
  type = string
  sensitive = true
  ephemeral = true
  default = null
}

resource "costgraph_azure_integration" "test" {
  tenant_id = %q
  azure_tenant_id = %q
  client_id = %q
  storage_account = %q
  container = %q
  export_name = %q
  enable_recommendations = false
  %s
  %s
  %s
}
`, tenantID, azureTenantID, clientID, storageAccount, container, folder, display, rotation, secret)
	}
	const resourceName = "costgraph_azure_integration.test"
	var createdID string
	checkPersisted := func(expectedName, expectedFolder string, version int64, remember bool) resource.TestCheckFunc {
		return func(state *terraform.State) error {
			record, ok := state.RootModule().Resources[resourceName]
			if !ok {
				return fmt.Errorf("Azure connection is missing from Terraform state")
			}
			if record.Primary.Attributes["client_secret_wo"] != "" {
				return fmt.Errorf("the write-only Azure secret appeared in Terraform state")
			}
			settings, err := client.GetAzureIntegrationSettings(context.Background(), record.Primary.ID)
			if err != nil {
				return err
			}
			if settings.TenantID != tenantID || settings.AzureTenantID != azureTenantID || settings.ClientID != clientID || settings.StorageAccount != storageAccount || settings.Container != container || settings.ExportName != expectedFolder || settings.DisplayName != expectedName || settings.SecretVersion != version || settings.EnableRecommendations {
				return fmt.Errorf("Azure connection readback differs from the Terraform configuration")
			}
			if remember {
				createdID = settings.ID
				_, err := client.ConnectAzureIntegration(context.Background(), costgraph.AzureConnectRequest{
					TenantID: tenantID, AzureTenantID: azureTenantID, ClientID: clientID, ClientSecret: &initialSecret,
					StorageAccount: storageAccount, Container: container, ExportName: exportName,
				})
				var apiErr *costgraph.APIError
				if !errors.As(err, &apiErr) || apiErr.StatusCode != 409 {
					return fmt.Errorf("creating the same Azure export connection did not return a conflict")
				}
			} else if settings.ID != createdID {
				return fmt.Errorf("updating the Azure connection changed its UUID")
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
				if record.Type != "costgraph_azure_integration" {
					continue
				}
				if _, err := client.GetConnection(context.Background(), record.Primary.ID); !isNotFound(err) {
					return fmt.Errorf("Azure connection was not confirmed absent after destroy")
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: config(nil, exportName, -1, true), Check: checkPersisted(storageAccount, exportName, 1, true)},
			{Config: config(nil, exportName, -1, true), PlanOnly: true},
			{ResourceName: resourceName, ImportState: true, ImportStateVerify: true},
			{Config: config(&name, exportName, -1, false), Check: checkPersisted(name, exportName, 1, false)},
			{Config: config(nil, exportName, -1, false), PlanOnly: true},
			{
				Config: config(nil, exportName, 2, true),
				PreConfig: func() {
					t.Setenv("TF_VAR_azure_client_secret", rotatedSecret)
				},
				Check: checkPersisted(name, exportName, 2, false),
			},
			{Config: config(nil, exportName, -1, false), PlanOnly: true},
			{Config: config(nil, "", -1, false), Check: checkPersisted(name, "", 2, false)},
			{Config: config(nil, "", -1, false), PlanOnly: true},
			{ResourceName: resourceName, ImportState: true, ImportStateVerify: true},
			{Config: config(nil, "", 1, false), PlanOnly: true, ExpectError: regexp.MustCompile("Secret version cannot decrease")},
		},
	})
}
