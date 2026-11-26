package provider

import (
	"context"

	costgraph "github.com/baselinehq/costgraph-go"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSourceWithConfigure = &currentOrganizationDataSource{}

type currentOrganizationDataSource struct {
	client *costgraph.Client
}

type organizationModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	Slug types.String `tfsdk:"slug"`
}

func NewCurrentOrganizationDataSource() datasource.DataSource {
	return &currentOrganizationDataSource{}
}

func (d *currentOrganizationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_current_organization"
}

func (d *currentOrganizationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read the organization identity associated with the configured API key. This lookup does not grant organization-wide management permissions.",
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{Computed: true, MarkdownDescription: "Organization UUID."},
			"name": schema.StringAttribute{Computed: true, MarkdownDescription: "Organization name."},
			"slug": schema.StringAttribute{Computed: true, MarkdownDescription: "Organization slug."},
		},
	}
}

func (d *currentOrganizationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *currentOrganizationDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	identity, err := d.client.CurrentIdentity(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Could not read organization", err.Error())
		return
	}
	if identity.OrganizationID == nil || *identity.OrganizationID == "" {
		resp.Diagnostics.AddError("Organization is missing", "The configured credential did not return an organization identity.")
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, organizationModel{
		ID: types.StringPointerValue(identity.OrganizationID), Name: types.StringPointerValue(identity.OrganizationName), Slug: types.StringPointerValue(identity.OrganizationSlug),
	})...)
}
