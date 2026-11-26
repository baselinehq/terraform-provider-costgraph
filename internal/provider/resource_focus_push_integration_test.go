package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	costgraph "github.com/baselinehq/costgraph-go"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccFocusPushIntegration(t *testing.T) {
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("Set TF_ACC=1 and configure an isolated CostGraph test deployment to run this lifecycle.")
	}
	apiURL, apiKey := os.Getenv("COSTGRAPH_TEST_API_URL"), os.Getenv("COSTGRAPH_TEST_API_KEY")
	tenantID := os.Getenv("COSTGRAPH_TEST_TENANT_ID")
	if apiURL == "" || apiKey == "" || tenantID == "" {
		t.Fatal("COSTGRAPH_TEST_API_URL, COSTGRAPH_TEST_API_KEY, and COSTGRAPH_TEST_TENANT_ID must identify an isolated test deployment")
	}
	t.Setenv("COSTGRAPH_API_URL", apiURL)
	t.Setenv("COSTGRAPH_API_KEY", apiKey)
	client, err := costgraph.New(apiURL, apiKey)
	if err != nil {
		t.Fatal(err)
	}
	accountID := fmt.Sprintf("terraform-acceptance-%d", time.Now().UnixNano())
	initial := integrationConfig(accountID, "")
	renamed := integrationConfig(accountID, `display_name = "Managed by Terraform"`)
	checkPersisted := func(expectedAccount, name string) resource.TestCheckFunc {
		return func(state *terraform.State) error {
			record, ok := state.RootModule().Resources["costgraph_focus_push_integration.test"]
			if !ok {
				return fmt.Errorf("FOCUS connection missing from Terraform state")
			}
			connection, err := client.GetConnection(context.Background(), record.Primary.ID)
			if err != nil {
				return err
			}
			if connection.DisplayName != name || connection.ProviderAccountID != expectedAccount || connection.TenantID != tenantID || connection.IngestMode != "push" {
				return fmt.Errorf("CostGraph persisted connection does not match the planned configuration")
			}
			return nil
		}
	}
	var replacedID string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"costgraph": providerserver.NewProtocol6WithError(New("test")()),
		},
		CheckDestroy: func(state *terraform.State) error {
			for _, record := range state.RootModule().Resources {
				if record.Type != "costgraph_focus_push_integration" {
					continue
				}
				_, err := client.GetConnection(context.Background(), record.Primary.ID)
				if !isNotFound(err) {
					return fmt.Errorf("connection was not confirmed absent after destroy")
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config:      strings.Replace(initial, `provider_name = "customcloud"`, fmt.Sprintf("%s\n  tenant_id = %q", `provider_name = "customcloud"`, "A0000000-0000-0000-0000-000000000001"), 1),
				ExpectError: regexp.MustCompile("Use a lowercase tenant UUID"),
			},
			{
				Config: initial,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkPersisted(accountID, accountID),
					resource.TestCheckResourceAttr("costgraph_focus_push_integration.test", "display_name", accountID),
					resource.TestCheckResourceAttr("costgraph_focus_push_integration.test", "tenant_id", tenantID),
					resource.TestCheckResourceAttrPair("data.costgraph_integration.test", "id", "costgraph_focus_push_integration.test", "id"),
					resource.TestCheckResourceAttrPair("data.costgraph_integration.test", "connection_id", "costgraph_focus_push_integration.test", "connection_id"),
					resource.TestCheckResourceAttrSet("data.costgraph_current_organization.test", "id"),
					resource.TestCheckResourceAttr("data.costgraph_tenant.test", "id", tenantID),
					resource.TestCheckResourceAttrSet("data.costgraph_tenant.test", "name"),
					resource.TestCheckResourceAttr("data.costgraph_integrations.test", "id", tenantID),
					resource.TestCheckResourceAttrSet("data.costgraph_integrations.test", "integrations.0.id"),
					resource.TestCheckResourceAttrSet("data.costgraph_integration_providers.test", "providers.0.slug"),
				),
			},
			{
				Config:      strings.Replace(initial, "id = costgraph_focus_push_integration.test.id", `id = "A0000000-0000-0000-0000-000000000001"`, 1),
				ExpectError: regexp.MustCompile("Use a lowercase connection UUID"),
			},
			{Config: initial, PlanOnly: true},
			{ResourceName: "costgraph_focus_push_integration.test", ImportState: true, ImportStateVerify: true},
			{Config: renamed, Check: checkPersisted(accountID, "Managed by Terraform")},
			{Config: renamed, PlanOnly: true},
			{Config: initial, Check: checkPersisted(accountID, "Managed by Terraform")},
			{
				Config: initial,
				PreConfig: func() {
					connections, err := client.ListConnections(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					for _, connection := range connections {
						if connection.ProviderAccountID == accountID {
							if err := client.DeleteConnection(context.Background(), connection.ID); err != nil {
								t.Fatal(err)
							}
							return
						}
					}
					t.Fatal("connection missing before external-deletion check")
				},
				Check: checkPersisted(accountID, accountID),
			},
			{
				Config: integrationConfig(accountID+"-replacement", ""),
				PreConfig: func() {
					connections, err := client.ListConnections(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					for _, connection := range connections {
						if connection.ProviderAccountID == accountID {
							replacedID = connection.ID
							return
						}
					}
					t.Fatal("connection missing before replacement check")
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkPersisted(accountID+"-replacement", accountID+"-replacement"),
					func(state *terraform.State) error {
						if state.RootModule().Resources["costgraph_focus_push_integration.test"].Primary.ID == replacedID {
							return fmt.Errorf("changing the billing account did not replace the connection")
						}
						if _, err := client.GetConnection(context.Background(), replacedID); !isNotFound(err) {
							return fmt.Errorf("the replaced connection was not removed")
						}
						return nil
					},
				),
			},
		},
	})
}

func integrationConfig(accountID, name string) string {
	return fmt.Sprintf(`
provider "costgraph" {}

resource "costgraph_focus_push_integration" "test" {
  provider_name = "customcloud"
  provider_account_id = %q
  %s
}

data "costgraph_integration" "test" {
  id = costgraph_focus_push_integration.test.id
}

data "costgraph_current_organization" "test" {}

data "costgraph_tenant" "test" {
  id = costgraph_focus_push_integration.test.tenant_id
}

data "costgraph_integrations" "test" {
  depends_on = [costgraph_focus_push_integration.test]
}

data "costgraph_integration_providers" "test" {}
`, accountID, name)
}
