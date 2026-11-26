package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	costgraph "github.com/baselinehq/costgraph-go"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccAIGatewayIntegration(t *testing.T) {
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("Set TF_ACC=1 to run the real gateway lifecycle.")
	}
	apiURL, key := os.Getenv("COSTGRAPH_TEST_API_URL"), os.Getenv("COSTGRAPH_TEST_MANAGEMENT_API_KEY")
	modelKey := os.Getenv("COSTGRAPH_TEST_MODEL_KEY")
	if apiURL == "" || key == "" || modelKey == "" {
		t.Fatal("Set COSTGRAPH_TEST_API_URL, COSTGRAPH_TEST_MANAGEMENT_API_KEY, and COSTGRAPH_TEST_MODEL_KEY for an isolated deployment, owner management key, and canonical catalog model")
	}
	t.Setenv("COSTGRAPH_API_URL", apiURL)
	t.Setenv("COSTGRAPH_API_KEY", key)
	client, err := costgraph.New(apiURL, key)
	if err != nil {
		t.Fatal(err)
	}
	account := fmt.Sprintf("terraform-gateway-%d", time.Now().UnixNano())
	config := func(settings string) string {
		return fmt.Sprintf(`
provider "costgraph" {}
resource "costgraph_tenant" "gateway" { name = %q }
resource "costgraph_ai_gateway_integration" "test" {
  tenant_id = costgraph_tenant.gateway.id
  provider_name = "agentgateway"
  provider_account_id = %q
  %s
}
data "costgraph_integration" "gateway" {
  id = costgraph_ai_gateway_integration.test.id
}
data "costgraph_tenant" "gateway" { id = costgraph_tenant.gateway.id }
`, account, account, settings)
	}
	initial := config("")
	updated := config(fmt.Sprintf(`display_name = "Inference gateway"
  scrape_namespace = "inference"
  scrape_label_selector = "app=gateway"
  scrape_port = 9090
  tenant_map = { acme = "Acme" }
  model_map = { served-model = %q }`, modelKey))
	check := func(overrides bool, name string) resource.TestCheckFunc {
		return func(state *terraform.State) error {
			item := state.RootModule().Resources["costgraph_ai_gateway_integration.test"]
			settings, err := client.GetAIGatewaySettings(context.Background(), item.Primary.ID)
			if err != nil {
				return err
			}
			connection, err := client.GetConnection(context.Background(), item.Primary.ID)
			if err != nil {
				return err
			}
			if connection.DisplayName != name || connection.ProviderAccountID != account || settings.IngestMode != "push" {
				return fmt.Errorf("gateway API readback differs from Terraform configuration")
			}
			if overrides {
				if settings.ScrapeNamespace == nil || *settings.ScrapeNamespace != "inference" || settings.ScrapePort == nil || *settings.ScrapePort != 9090 || settings.ScrapeLabelSelector == nil || *settings.ScrapeLabelSelector != "app=gateway" || settings.TenantMap["acme"] != "Acme" || settings.ModelMap["served-model"] != modelKey {
					return fmt.Errorf("gateway overrides were not persisted")
				}
			} else if settings.ScrapeNamespace != nil || settings.ScrapePort != nil || settings.ScrapeLabelSelector != nil || len(settings.TenantMap) != 0 || len(settings.ModelMap) != 0 {
				return fmt.Errorf("removed gateway overrides were not cleared")
			}
			return nil
		}
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"costgraph": providerserver.NewProtocol6WithError(New("test")())},
		CheckDestroy: func(state *terraform.State) error {
			for _, item := range state.RootModule().Resources {
				if item.Type == "costgraph_ai_gateway_integration" {
					if _, err := client.GetConnection(context.Background(), item.Primary.ID); !isNotFound(err) {
						return fmt.Errorf("gateway was not confirmed absent after destroy")
					}
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: config(`model_map = { served-model = "nonexistent-acceptance-model" }`), ExpectError: regexp.MustCompile("not a canonical model key")},
			{Config: initial, Check: resource.ComposeAggregateTestCheckFunc(check(false, account), resource.TestCheckResourceAttrPair("data.costgraph_integration.gateway", "id", "costgraph_ai_gateway_integration.test", "id"), resource.TestCheckResourceAttrPair("data.costgraph_tenant.gateway", "id", "costgraph_tenant.gateway", "id"))},
			{Config: initial, PlanOnly: true},
			{ResourceName: "costgraph_ai_gateway_integration.test", ImportState: true, ImportStateVerify: true},
			{Config: updated, Check: check(true, "Inference gateway")},
			{Config: updated, PlanOnly: true},
			{ResourceName: "costgraph_ai_gateway_integration.test", ImportState: true, ImportStateVerify: true},
			{Config: initial, Check: check(false, "Inference gateway")},
			{Config: initial, PlanOnly: true},
		},
	})
}
