package provider

import (
	"context"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	costgraph "github.com/baselinehq/costgraph-go"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func cloudAcceptanceClient(t *testing.T) *costgraph.Client {
	t.Helper()
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("Set TF_ACC=1 and provide dedicated cloud test accounts to run this real lifecycle.")
	}
	apiURL := requireCloudTestEnv(t, "COSTGRAPH_TEST_API_URL")
	apiKey := requireCloudTestEnv(t, "COSTGRAPH_TEST_MANAGEMENT_API_KEY")
	t.Setenv("COSTGRAPH_API_URL", apiURL)
	t.Setenv("COSTGRAPH_API_KEY", apiKey)
	client, err := costgraph.New(apiURL, apiKey)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := client.CurrentIdentity(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if identity.SubjectType != "api_key" || !slices.Contains(identity.PermissionScopes, "organization:manage") {
		t.Fatal("COSTGRAPH_TEST_MANAGEMENT_API_KEY must be an organization owner's user API key with organization:manage")
	}
	return client
}

func requireCloudTestEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s must identify a dedicated cloud acceptance fixture; cloud tests are not replaced by mocks", name)
	}
	return value
}

func TestAccAWSIntegration(t *testing.T) {
	client := cloudAcceptanceClient(t)
	tenantID := requireCloudTestEnv(t, "COSTGRAPH_TEST_TENANT_ID")
	accountID := requireCloudTestEnv(t, "COSTGRAPH_TEST_AWS_BILLING_ACCOUNT_ID")
	region := requireCloudTestEnv(t, "COSTGRAPH_TEST_AWS_REGION")
	bucket := requireCloudTestEnv(t, "COSTGRAPH_TEST_AWS_BUCKET")
	prefix := requireCloudTestEnv(t, "COSTGRAPH_TEST_AWS_PREFIX")
	roleARN := requireCloudTestEnv(t, "COSTGRAPH_TEST_AWS_ROLE_ARN")
	name := fmt.Sprintf("Terraform AWS acceptance %d", time.Now().UnixNano())
	config := func(displayName *string, exportPrefix string) string {
		display := ""
		if displayName != nil {
			display = fmt.Sprintf("display_name = %q", *displayName)
		}
		return fmt.Sprintf(`
provider "costgraph" {}

resource "costgraph_aws_integration" "test" {
  tenant_id = %q
  billing_account_id = %q
  region = %q
  bucket = %q
  prefix = %q
  role_arn = %q
  activate_cost_allocation_tags = false
  enable_recommendations = false
  %s
}
`, tenantID, accountID, region, bucket, exportPrefix, roleARN, display)
	}
	const resourceName = "costgraph_aws_integration.test"
	var createdID, deletedID string
	checkPersisted := func(expectedName, expectedPrefix string, remember bool) resource.TestCheckFunc {
		return func(state *terraform.State) error {
			record, ok := state.RootModule().Resources[resourceName]
			if !ok {
				return fmt.Errorf("AWS connection is missing from Terraform state")
			}
			settings, err := client.GetAWSIntegrationSettings(context.Background(), record.Primary.ID)
			if err != nil {
				return err
			}
			if settings.TenantID != tenantID || settings.BillingAccountID != accountID || settings.Region != region || settings.Bucket != bucket || settings.Prefix != expectedPrefix || settings.DisplayName != expectedName || settings.AssumeRole.RoleARN != roleARN || settings.AssumeRole.ExternalID == "" || settings.ActivateCostAllocationTags || settings.EnableRecommendations {
				return fmt.Errorf("AWS connection readback differs from the Terraform configuration")
			}
			if remember {
				createdID = settings.ID
			} else if settings.ID != createdID {
				return fmt.Errorf("updating the AWS connection changed its UUID")
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
				if record.Type != "costgraph_aws_integration" {
					continue
				}
				if _, err := client.GetConnection(context.Background(), record.Primary.ID); !isNotFound(err) {
					return fmt.Errorf("AWS connection was not confirmed absent after destroy")
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: config(nil, prefix), Check: checkPersisted(accountID, prefix, true)},
			{Config: config(nil, prefix), PlanOnly: true},
			{ResourceName: resourceName, ImportState: true, ImportStateVerify: true},
			{Config: config(&name, prefix), Check: checkPersisted(name, prefix, false)},
			{Config: config(&name, prefix), PlanOnly: true},
			{Config: config(nil, prefix), Check: checkPersisted(name, prefix, false)},
			{Config: config(nil, ""), Check: checkPersisted(name, "", false)},
			{Config: config(nil, ""), PlanOnly: true},
			{
				Config: config(nil, prefix),
				PreConfig: func() {
					if createdID == "" {
						t.Fatal("AWS connection identity was not recorded before external deletion")
					}
					deletedID = createdID
					if err := client.DeleteConnection(context.Background(), createdID); err != nil {
						t.Fatal(err)
					}
					if _, err := client.GetConnection(context.Background(), createdID); !isNotFound(err) {
						t.Fatal("externally deleted AWS connection remains accessible")
					}
				},
				Check: func(state *terraform.State) error {
					if state.RootModule().Resources[resourceName].Primary.ID == deletedID {
						return fmt.Errorf("externally deleted AWS connection was not recreated")
					}
					return checkPersisted(accountID, prefix, true)(state)
				},
			},
		},
	})
}
