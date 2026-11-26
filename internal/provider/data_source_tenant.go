package provider

import (
	"context"
	"regexp"
	"slices"
	"strings"

	costgraph "github.com/baselinehq/costgraph-go"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSourceWithConfigure = &tenantDataSource{}

type tenantDataSource struct {
	client *costgraph.Client
}

type tenantDataSourceModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	OrganizationID types.String `tfsdk:"organization_id"`
}

func NewTenantDataSource() datasource.DataSource {
	return &tenantDataSource{}
}

func (d *tenantDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant"
}

func (d *tenantDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read a tenant accessible to the configured API key. An active organization owner with organization:manage can look up another tenant in the same organization. Other keys can look up only tenants exposed by their identity. Returns the tenant name and organization ID. This data source does not grant permissions, and an unavailable tenant returns an error.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "UUID of a tenant accessible to the configured API key.",
				Validators: []validator.String{stringvalidator.RegexMatches(
					regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`), "Use a tenant UUID.",
				)},
			},
			"name":            schema.StringAttribute{Computed: true, MarkdownDescription: "Tenant name shown in CostGraph."},
			"organization_id": schema.StringAttribute{Computed: true, MarkdownDescription: "UUID of the organization associated with the API key."},
		},
	}
}

func (d *tenantDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*costgraph.Client)
	if !ok {
		resp.Diagnostics.AddError("Invalid provider client", "The provider did not configure a CostGraph management client.")
		return
	}
	d.client = client
}

func (d *tenantDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config tenantDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	identity, err := d.client.CurrentIdentity(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Could not read tenant", err.Error())
		return
	}
	if slices.Contains(identity.PermissionScopes, costgraph.ScopeOrganizationManage) {
		if identity.OrganizationID == nil || *identity.OrganizationID == "" {
			resp.Diagnostics.AddError("Organization is missing", "The configured credential did not return an organization identity.")
			return
		}
		tenant, err := d.client.GetTenant(ctx, *identity.OrganizationID, config.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Could not read tenant", err.Error())
			return
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, tenantDataSourceModel{
			ID: config.ID, Name: types.StringValue(tenant.Name), OrganizationID: types.StringValue(tenant.OrganizationID),
		})...)
		return
	}
	for _, tenant := range identity.Tenants {
		if !strings.EqualFold(tenant.ID, config.ID.ValueString()) {
			continue
		}
		if identity.OrganizationID == nil || *identity.OrganizationID == "" {
			resp.Diagnostics.AddError("Organization is missing", "The configured credential did not return an organization identity.")
			return
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, tenantDataSourceModel{
			ID: config.ID, Name: types.StringValue(tenant.Name), OrganizationID: types.StringPointerValue(identity.OrganizationID),
		})...)
		return
	}
	resp.Diagnostics.AddError("Tenant is unavailable", "The configured API key does not expose a tenant with this UUID. Check the tenant ID and the key's tenant access.")
}
