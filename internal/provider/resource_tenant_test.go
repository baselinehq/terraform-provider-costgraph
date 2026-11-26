package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"slices"
	"testing"
	"time"

	costgraph "github.com/baselinehq/costgraph-go"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccTenant(t *testing.T) {
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("Set TF_ACC=1 and configure an isolated CostGraph test deployment to run this lifecycle.")
	}
	apiURL := os.Getenv("COSTGRAPH_TEST_API_URL")
	apiKey := os.Getenv("COSTGRAPH_TEST_MANAGEMENT_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("COSTGRAPH_TEST_API_KEY")
	}
	if apiURL == "" || apiKey == "" {
		t.Fatal("COSTGRAPH_TEST_API_URL and COSTGRAPH_TEST_MANAGEMENT_API_KEY must identify an isolated deployment and organization-management API key")
	}
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
	if identity.OrganizationID == nil || *identity.OrganizationID == "" || !slices.Contains(identity.PermissionScopes, costgraph.ScopeOrganizationManage) {
		t.Fatal("The management API key must expose its organization and include the explicit organization:manage scope")
	}
	organizationID := *identity.OrganizationID
	name := fmt.Sprintf("terraform-tenant-%d", time.Now().UnixNano())
	renamedName := name + "-renamed"
	trackedIDs := map[string]struct{}{}
	t.Cleanup(func() {
		for id := range trackedIDs {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := client.DeleteTenant(ctx, organizationID, id); err != nil && !isNotFound(err) {
				t.Errorf("Could not clean up acceptance tenant: %s", err)
			}
			cancel()
		}
	})
	checkPersisted := func(expectedName string) resource.TestCheckFunc {
		return func(state *terraform.State) error {
			record, ok := state.RootModule().Resources["costgraph_tenant.test"]
			if !ok {
				return fmt.Errorf("tenant missing from Terraform state")
			}
			trackedIDs[record.Primary.ID] = struct{}{}
			tenant, err := client.GetTenant(context.Background(), organizationID, record.Primary.ID)
			if err != nil {
				return err
			}
			if tenant.Name != expectedName || tenant.OrganizationID != organizationID || !tenant.Active {
				return fmt.Errorf("CostGraph persisted tenant does not match the planned configuration")
			}
			tenants, err := client.ListTenants(context.Background(), organizationID)
			if err != nil {
				return err
			}
			if !slices.ContainsFunc(tenants, func(candidate costgraph.ManagedTenant) bool {
				return candidate.ID == tenant.ID && candidate.Name == expectedName
			}) {
				return fmt.Errorf("organization tenant list does not contain the persisted tenant")
			}
			return nil
		}
	}
	var originalID, externallyDeletedID string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"costgraph": providerserver.NewProtocol6WithError(New("test")()),
		},
		CheckDestroy: func(state *terraform.State) error {
			for _, record := range state.RootModule().Resources {
				if record.Type != "costgraph_tenant" {
					continue
				}
				_, err := client.GetTenant(context.Background(), organizationID, record.Primary.ID)
				if !isNotFound(err) {
					return fmt.Errorf("tenant was not confirmed absent after destroy")
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: tenantConfig(" \t "), ExpectError: regexp.MustCompile("Tenant names must not be blank")},
			{Config: tenantConfig("\u00a0Tenant\u00a0"), ExpectError: regexp.MustCompile("surrounding whitespace")},
			{
				Config: tenantConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkPersisted(name),
					resource.TestCheckResourceAttr("costgraph_tenant.test", "organization_id", organizationID),
					resource.TestCheckResourceAttr("costgraph_tenant.test", "active", "true"),
					resource.TestCheckResourceAttrPair("data.costgraph_tenant.test", "id", "costgraph_tenant.test", "id"),
					resource.TestCheckResourceAttr("data.costgraph_tenant.test", "name", name),
					func(state *terraform.State) error {
						originalID = state.RootModule().Resources["costgraph_tenant.test"].Primary.ID
						return nil
					},
				),
			},
			{Config: tenantConfig(name), PlanOnly: true},
			{
				ResourceName: "costgraph_tenant.test", ImportState: true, ImportStateVerify: true,
				ImportStateIdFunc: func(state *terraform.State) (string, error) {
					record := state.RootModule().Resources["costgraph_tenant.test"]
					return record.Primary.Attributes["organization_id"] + "/" + record.Primary.ID, nil
				},
			},
			{
				Config: tenantConfig(renamedName),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkPersisted(renamedName),
					func(state *terraform.State) error {
						if state.RootModule().Resources["costgraph_tenant.test"].Primary.ID != originalID {
							return fmt.Errorf("renaming the tenant changed its UUID")
						}
						return nil
					},
					resource.TestCheckResourceAttr("data.costgraph_tenant.test", "name", renamedName),
				),
			},
			{Config: tenantConfig(renamedName), PlanOnly: true},
			{
				Config: tenantConfig(renamedName),
				PreConfig: func() {
					externallyDeletedID = originalID
					if err := client.DeleteTenant(context.Background(), organizationID, externallyDeletedID); err != nil {
						t.Fatal(err)
					}
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkPersisted(renamedName),
					func(state *terraform.State) error {
						if state.RootModule().Resources["costgraph_tenant.test"].Primary.ID == externallyDeletedID {
							return fmt.Errorf("externally deleted tenant was not recreated")
						}
						return nil
					},
				),
			},
			{Config: tenantConfig(renamedName), PlanOnly: true},
		},
	})
}

func tenantConfig(name string) string {
	return fmt.Sprintf(`
provider "costgraph" {}

resource "costgraph_tenant" "test" {
  name = %q
}

data "costgraph_tenant" "test" {
  id = costgraph_tenant.test.id
}
`, name)
}
