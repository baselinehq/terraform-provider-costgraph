package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	costgraph "github.com/baselinehq/costgraph-go"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccKubernetesCluster(t *testing.T) {
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("Set TF_ACC=1 and configure an isolated CostGraph backend to run this lifecycle.")
	}
	apiURL := os.Getenv("COSTGRAPH_TEST_API_URL")
	managementKey := os.Getenv("COSTGRAPH_TEST_MANAGEMENT_API_KEY")
	collectorKey := os.Getenv("COSTGRAPH_TEST_API_KEY")
	tenantID := os.Getenv("COSTGRAPH_TEST_TENANT_ID")
	if apiURL == "" || managementKey == "" || collectorKey == "" || tenantID == "" {
		t.Fatal("Configure COSTGRAPH_TEST_API_URL, COSTGRAPH_TEST_MANAGEMENT_API_KEY, COSTGRAPH_TEST_API_KEY with operator:read and operator:write, and COSTGRAPH_TEST_TENANT_ID for an isolated backend")
	}
	t.Setenv("COSTGRAPH_API_URL", apiURL)
	t.Setenv("COSTGRAPH_API_KEY", managementKey)
	client, err := costgraph.New(apiURL, managementKey)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := client.CurrentIdentity(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if identity.OrganizationID == nil || !slices.Contains(identity.PermissionScopes, costgraph.ScopeOrganizationManage) {
		t.Fatal("The management key must belong to an organization owner with organization:manage")
	}
	organizationID := *identity.OrganizationID
	name := fmt.Sprintf("terraform-kubernetes-%d", time.Now().UnixNano())
	initial := kubernetesClusterConfig(name, tenantID)
	replacement := kubernetesClusterConfig(name+"-replacement", tenantID)
	tracked := map[string]struct{}{}
	t.Cleanup(func() {
		for id := range tracked {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := client.DeleteKubernetesCluster(ctx, organizationID, id); err != nil && !isNotFound(err) {
				t.Errorf("Could not disconnect acceptance cluster: %s", err)
			}
			cancel()
		}
	})
	type observedCluster struct {
		ID                string         `json:"id"`
		OperatorVersion   string         `json:"operator_version"`
		KubernetesVersion string         `json:"kubernetes_version"`
		RawData           map[string]any `json:"raw_data"`
	}
	collectorRequest := func(method, path string, payload any, expectedStatus int) (*observedCluster, error) {
		var body io.Reader
		if payload != nil {
			encoded, err := json.Marshal(payload)
			if err != nil {
				return nil, err
			}
			body = bytes.NewReader(encoded)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(apiURL, "/")+path, body)
		if err != nil {
			return nil, errors.New("could not build collector request")
		}
		request.Header.Set("X-API-Key", collectorKey)
		request.Header.Set("Content-Type", "application/json")
		response, err := (&http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}).Do(request)
		if err != nil {
			return nil, errors.New("collector request failed")
		}
		defer response.Body.Close()
		if response.StatusCode != expectedStatus {
			return nil, fmt.Errorf("collector endpoint returned HTTP %d, expected %d", response.StatusCode, expectedStatus)
		}
		if expectedStatus != http.StatusOK {
			return nil, nil
		}
		var envelope struct {
			Data struct {
				Cluster observedCluster `json:"cluster"`
			} `json:"data"`
		}
		if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&envelope); err != nil {
			return nil, errors.New("could not decode collector registration")
		}
		return &envelope.Data.Cluster, nil
	}
	registerCollector := func(clusterName string, expectedStatus int) (*observedCluster, error) {
		return collectorRequest(http.MethodPost, "/api/v1/tenant/clusters", map[string]any{
			"name": clusterName, "operator_version": "acceptance-operator", "kubernetes_version": "v1.34.0",
			"raw_data": map[string]any{"acceptance_observation": "reported-by-collector"},
		}, expectedStatus)
	}
	checkPersisted := func(expectedName string) resource.TestCheckFunc {
		return func(state *terraform.State) error {
			record := state.RootModule().Resources["costgraph_kubernetes_cluster.test"]
			if record == nil {
				return errors.New("cluster registration missing from Terraform state")
			}
			tracked[record.Primary.ID] = struct{}{}
			cluster, err := client.GetKubernetesCluster(context.Background(), organizationID, record.Primary.ID)
			if err != nil {
				return err
			}
			if cluster.Name != expectedName || cluster.TenantID != tenantID {
				return errors.New("registration readback differs from the planned configuration")
			}
			return nil
		}
	}
	var firstID, externallyDeletedID, replacementID string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"costgraph": providerserver.NewProtocol6WithError(New("test")())},
		CheckDestroy: func(state *terraform.State) error {
			for _, record := range state.RootModule().Resources {
				if record.Type != "costgraph_kubernetes_cluster" {
					continue
				}
				if _, err := client.GetKubernetesCluster(context.Background(), organizationID, record.Primary.ID); !isNotFound(err) {
					return errors.New("destroyed cluster registration is still active")
				}
				if _, err := registerCollector(record.Primary.Attributes["name"], http.StatusGone); err != nil {
					return err
				}
				if _, err := collectorRequest(http.MethodGet, "/api/v1/tenant/clusters/"+record.Primary.ID+"/scrape-config", nil, http.StatusGone); err != nil {
					return err
				}
				fresh, err := client.CreateKubernetesCluster(context.Background(), organizationID, costgraph.CreateKubernetesClusterRequest{Name: record.Primary.Attributes["name"], TenantID: tenantID})
				if err != nil {
					return err
				}
				tracked[fresh.ID] = struct{}{}
				if fresh.ID == record.Primary.ID {
					return errors.New("explicit re-enrollment resurrected the historical registration")
				}
				observed, err := registerCollector(fresh.Name, http.StatusOK)
				if err != nil {
					return err
				}
				if observed.ID != fresh.ID {
					return errors.New("collector did not adopt the explicitly re-enrolled registration")
				}
				if err := client.DeleteKubernetesCluster(context.Background(), organizationID, fresh.ID); err != nil {
					return err
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: kubernetesClusterConfig(name, "A0000000-0000-0000-0000-000000000001"), ExpectError: regexp.MustCompile("Use a lowercase tenant UUID")},
			{
				Config: initial,
				Check: resource.ComposeAggregateTestCheckFunc(checkPersisted(name), func(state *terraform.State) error {
					firstID = state.RootModule().Resources["costgraph_kubernetes_cluster.test"].Primary.ID
					_, err := client.CreateKubernetesCluster(context.Background(), organizationID, costgraph.CreateKubernetesClusterRequest{Name: name, TenantID: tenantID})
					var apiError *costgraph.APIError
					if !errors.As(err, &apiError) || apiError.StatusCode != http.StatusConflict {
						return errors.New("duplicate registration was not rejected with HTTP 409")
					}
					observed, err := registerCollector(name, http.StatusOK)
					if err != nil {
						return err
					}
					if observed.ID != firstID || observed.OperatorVersion != "acceptance-operator" || observed.KubernetesVersion != "v1.34.0" || observed.RawData["acceptance_observation"] != "reported-by-collector" {
						return errors.New("collector did not update the reserved UUID with its own observations")
					}
					return nil
				}),
			},
			{Config: initial, PlanOnly: true},
			{ResourceName: "costgraph_kubernetes_cluster.test", ImportState: true, ImportStateVerify: true},
			{
				Config: initial,
				PreConfig: func() {
					externallyDeletedID = firstID
					if err := client.DeleteKubernetesCluster(context.Background(), organizationID, firstID); err != nil {
						t.Fatal(err)
					}
					if _, err := registerCollector(name, http.StatusGone); err != nil {
						t.Fatal(err)
					}
				},
				Check: resource.ComposeAggregateTestCheckFunc(checkPersisted(name), func(state *terraform.State) error {
					replacementID = state.RootModule().Resources["costgraph_kubernetes_cluster.test"].Primary.ID
					if replacementID == externallyDeletedID {
						return errors.New("external deletion did not create a fresh registration")
					}
					return nil
				}),
			},
			{Config: initial, PlanOnly: true},
			{
				Config: replacement,
				Check: resource.ComposeAggregateTestCheckFunc(checkPersisted(name+"-replacement"), func(state *terraform.State) error {
					if state.RootModule().Resources["costgraph_kubernetes_cluster.test"].Primary.ID == replacementID {
						return errors.New("changing the name did not replace the registration")
					}
					if _, err := client.GetKubernetesCluster(context.Background(), organizationID, replacementID); !isNotFound(err) {
						return errors.New("replaced registration is still active")
					}
					return nil
				}),
			},
			{Config: replacement, PlanOnly: true},
		},
	})
}

func kubernetesClusterConfig(name, tenantID string) string {
	return fmt.Sprintf(`
provider "costgraph" {}
resource "costgraph_kubernetes_cluster" "test" {
  name      = %q
  tenant_id = %q
}
`, name, tenantID)
}
