package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	costgraph "github.com/baselinehq/costgraph-go"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccVirtualTag(t *testing.T) {
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("Set TF_ACC=1 and configure an isolated CostGraph deployment.")
	}
	apiURL, key, tenant := os.Getenv("COSTGRAPH_TEST_API_URL"), os.Getenv("COSTGRAPH_TEST_API_KEY"), os.Getenv("COSTGRAPH_TEST_TENANT_ID")
	if apiURL == "" || key == "" || tenant == "" {
		t.Fatal("Set COSTGRAPH_TEST_API_URL, COSTGRAPH_TEST_API_KEY, and COSTGRAPH_TEST_TENANT_ID for an isolated deployment. The key needs virtual-tags:write and focus:read.")
	}
	fixture := os.Getenv("COSTGRAPH_TEST_VIRTUAL_TAG_RESOURCE_ID")
	if fixture == "" {
		t.Fatal("Set COSTGRAPH_TEST_VIRTUAL_TAG_RESOURCE_ID to an AWS billing record named payments-virtual-tag-test with owner=platform tags in the current month.")
	}
	t.Setenv("COSTGRAPH_API_URL", apiURL)
	t.Setenv("COSTGRAPH_API_KEY", key)
	client, err := costgraph.New(apiURL, key)
	if err != nil {
		t.Fatal(err)
	}
	tag := fmt.Sprintf("terraform-team-%d", time.Now().UnixNano())
	ids := map[string]string{}
	tracked := map[string]bool{}
	t.Cleanup(func() {
		for id := range tracked {
			value, err := client.GetVirtualTag(context.Background(), tenant, id)
			if isNotFound(err) {
				continue
			}
			if err == nil {
				err = client.DeleteVirtualTag(context.Background(), tenant, id, value.Revision)
			}
			if err != nil {
				t.Errorf("Could not clean up virtual tag: %v", err)
			}
		}
	})
	check := func(stage int, expected string) resource.TestCheckFunc {
		return func(state *terraform.State) error {
			rec := state.RootModule().Resources["costgraph_virtual_tag.test"]
			if rec == nil {
				return fmt.Errorf("virtual tag missing from Terraform state")
			}
			tracked[rec.Primary.ID] = true
			value, err := client.GetVirtualTag(context.Background(), tenant, rec.Primary.ID)
			if err != nil {
				return err
			}
			if value.Key != tag || value.TenantID != tenant {
				return fmt.Errorf("persisted key or tenant does not match")
			}
			if stage == 3 {
				if len(value.Rules) != 0 || value.Description != "" {
					return fmt.Errorf("explicit empty rules and description did not clear")
				}
				return nil
			}
			if len(value.Rules) != 2 {
				return fmt.Errorf("expected two persisted rules")
			}
			for _, rule := range value.Rules {
				if id, ok := ids[rule.RuleKey]; ok && id != rule.ID {
					return fmt.Errorf("editing or reordering changed rule UUID for %s", rule.RuleKey)
				}
				ids[rule.RuleKey] = rule.ID
			}
			if stage == 1 && (value.Rules[0].RuleKey != "broad" || !value.Rules[0].Active) {
				return fmt.Errorf("reorder or resume was not persisted")
			}
			if stage == 2 && (value.Rules[0].RuleKey != "specific" || value.Rules[0].Active) {
				return fmt.Errorf("pause or reorder was not persisted")
			}
			return assertVirtualTagCost(apiURL, key, tenant, fixture, tag, expected)
		}
	}
	var removed string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"costgraph": providerserver.NewProtocol6WithError(New("test")())},
		CheckDestroy: func(state *terraform.State) error {
			for _, rec := range state.RootModule().Resources {
				if rec.Type == "costgraph_virtual_tag" {
					_, err := client.GetVirtualTag(context.Background(), tenant, rec.Primary.ID)
					if !isNotFound(err) {
						return fmt.Errorf("key still exists after destroy")
					}
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: virtualTagConfig(tenant, tag, 0) + "\n", Check: resource.ComposeAggregateTestCheckFunc(check(0, "payments"), resource.TestCheckResourceAttrPair("data.costgraph_virtual_tag.test", "id", "costgraph_virtual_tag.test", "id"), resource.TestCheckResourceAttr("data.costgraph_virtual_tag.test", "rules.#", "2"))},
			{Config: virtualTagConfig(tenant, tag, 0), PlanOnly: true},
			{ResourceName: "costgraph_virtual_tag.test", ImportState: true, ImportStateVerify: true, ImportStateIdFunc: func(s *terraform.State) (string, error) {
				return tenant + "/" + s.RootModule().Resources["costgraph_virtual_tag.test"].Primary.ID, nil
			}},
			{Config: virtualTagConfig(tenant, tag, 1), Check: check(1, "aws-team")},
			{Config: virtualTagConfig(tenant, tag, 1), PlanOnly: true},
			{Config: virtualTagConfig(tenant, tag, 2), Check: check(2, "platform")},
			{Config: virtualTagConfig(tenant, tag, 2), PlanOnly: true},
			{Config: virtualTagConfig(tenant, tag, 3), Check: check(3, "")},
			{Config: virtualTagConfig(tenant, tag, 3), PlanOnly: true},
			{ResourceName: "costgraph_virtual_tag.test", ImportState: true, ImportStateVerify: true, ImportStateIdFunc: func(s *terraform.State) (string, error) {
				return tenant + "/" + s.RootModule().Resources["costgraph_virtual_tag.test"].Primary.ID, nil
			}},
			{Config: virtualTagConfig(tenant, tag, 3), PreConfig: func() {
				value, err := client.LookupVirtualTag(context.Background(), tenant, tag)
				if err != nil {
					t.Fatal(err)
				}
				removed = value.ID
				if err := client.DeleteVirtualTag(context.Background(), tenant, value.ID, value.Revision); err != nil {
					t.Fatal(err)
				}
			}, Check: func(s *terraform.State) error {
				if s.RootModule().Resources["costgraph_virtual_tag.test"].Primary.ID == removed {
					return fmt.Errorf("externally deleted key was not recreated")
				}
				return check(3, "")(s)
			}},
		},
	})
	testVirtualTagSafeguards(t, client, apiURL, tenant, tag+"-safeguards")
}

func virtualTagConfig(tenant, key string, stage int) string {
	specific := `{ rule_key = "specific", scope = "billing", expression = "bill.resource_name.startsWith(\"payments-\")", value = "payments" }`
	broad := `{ rule_key = "broad", scope = "billing", expression = "bill.provider == \"aws\"", value = "aws-team" }`
	rules := "[" + specific + "," + broad + "]"
	description := "Owned by Terraform"
	switch stage {
	case 1:
		rules = "[" + broad + "," + specific + "]"
	case 2:
		rules = `[{rule_key="specific",scope="billing",expression="bill.resource_name.startsWith(\"payments-\")",value="payments-updated",active=false},{rule_key="broad",scope="billing",expression="\"owner\" in bill.tags",inherited_from="bill.tags[\"owner\"]"}]`
	case 3:
		rules = "[]"
		description = ""
	}
	return fmt.Sprintf(`provider "costgraph" {}
resource "costgraph_virtual_tag" "test" {
 tenant_id = %q
 key = %q
 description = %q
 rules = %s
}
data "costgraph_virtual_tag" "test" {
 tenant_id = costgraph_virtual_tag.test.tenant_id
 key = costgraph_virtual_tag.test.key
 depends_on = [costgraph_virtual_tag.test]
}
data "costgraph_virtual_tags" "test" {
 tenant_id = costgraph_virtual_tag.test.tenant_id
 depends_on = [costgraph_virtual_tag.test]
}
`, tenant, key, description, rules)
}

func assertVirtualTagCost(apiURL, key, tenant, fixture, tag, expected string) error {
	params := url.Values{"group_by": {"tag:" + tag}, "layer": {"billed"}, "filter": {"resource:" + fixture}, "period": {"month_to_date"}}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, strings.TrimRight(apiURL, "/")+"/api/v1/tenant/cost/query?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", key)
	req.Header.Set("X-CostGraph-Tenant-ID", tenant)
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("public cost query failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("public cost query returned HTTP %d", response.StatusCode)
	}
	var result struct {
		Data struct {
			Groups []struct {
				Key     string  `json:"key"`
				Current float64 `json:"current"`
			} `json:"groups"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&result) != nil {
		return fmt.Errorf("could not decode cost query")
	}
	if len(result.Data.Groups) != 1 || result.Data.Groups[0].Key != expected || result.Data.Groups[0].Current != 42 {
		return fmt.Errorf("public cost query did not assign the $42 fixture to %q", expected)
	}
	return nil
}

func testVirtualTagSafeguards(t *testing.T, client *costgraph.Client, apiURL, tenant, key string) {
	t.Helper()
	ctx := context.Background()
	body := costgraph.VirtualTagRequest{Key: key, Description: "original", Rules: []costgraph.VirtualTagRuleInput{}}
	created, err := client.CreateVirtualTag(ctx, tenant, body)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		value, err := client.GetVirtualTag(ctx, tenant, created.ID)
		if err == nil {
			if err = client.DeleteVirtualTag(ctx, tenant, value.ID, value.Revision); err != nil {
				t.Errorf("cleanup safeguard key: %v", err)
			}
		}
	})
	requireStatus := func(err error, status int) {
		t.Helper()
		var apiErr *costgraph.APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != status {
			t.Fatalf("expected HTTP %d, got %v", status, err)
		}
	}
	_, err = client.CreateVirtualTag(ctx, tenant, body)
	requireStatus(err, http.StatusConflict)
	body.ExpectedRevision = &created.Revision
	body.Description = "changed"
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := client.UpdateVirtualTag(ctx, tenant, created.ID, body)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else {
			requireStatus(err, http.StatusConflict)
			conflict++
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("concurrent writes did not produce one success and one conflict")
	}
	requireStatus(client.DeleteVirtualTag(ctx, tenant, created.ID, created.Revision), http.StatusConflict)
	if other := os.Getenv("COSTGRAPH_TEST_OTHER_TENANT_ID"); other != "" {
		_, err := client.GetVirtualTag(ctx, other, created.ID)
		requireStatus(err, http.StatusForbidden)
	}
	if ordinary := os.Getenv("COSTGRAPH_TEST_MANAGEMENT_API_KEY"); ordinary != "" {
		limited, err := costgraph.New(apiURL, ordinary)
		if err != nil {
			t.Fatal(err)
		}
		_, err = limited.GetVirtualTag(ctx, tenant, created.ID)
		requireStatus(err, http.StatusForbidden)
	}
	_, err = client.LookupVirtualTag(ctx, tenant, key+"-missing")
	requireStatus(err, http.StatusNotFound)
}
