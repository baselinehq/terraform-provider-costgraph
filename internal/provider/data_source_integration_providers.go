package provider

import (
	"cmp"
	"context"
	"slices"

	costgraph "github.com/baselinehq/costgraph-go"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSourceWithConfigure = &integrationProvidersDataSource{}

type integrationProvidersDataSource struct {
	client *costgraph.Client
}

type integrationProvidersDataSourceModel struct {
	ID        types.String               `tfsdk:"id"`
	Providers []integrationProviderModel `tfsdk:"providers"`
}

type integrationProviderModel struct {
	Slug     types.String `tfsdk:"slug"`
	Name     types.String `tfsdk:"name"`
	Category types.String `tfsdk:"category"`
	DocsURL  types.String `tfsdk:"docs_url"`
}

func NewIntegrationProvidersDataSource() datasource.DataSource {
	return &integrationProvidersDataSource{}
}

func (d *integrationProvidersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_integration_providers"
}

func (d *integrationProvidersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read the deployment's FOCUS billing-provider catalog, sorted by provider slug. Returns provider names, categories, and documentation links without credential values. Catalog entries do not imply that the Terraform provider can manage those integrations. The API returns the complete catalog without pagination.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, MarkdownDescription: "Stable catalog identifier, focus-billing-providers."},
			"providers": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "FOCUS billing providers sorted by their canonical slug.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"slug":     schema.StringAttribute{Computed: true, MarkdownDescription: "Canonical provider slug."},
					"name":     schema.StringAttribute{Computed: true, MarkdownDescription: "Provider display name."},
					"category": schema.StringAttribute{Computed: true, MarkdownDescription: "Category reported by the provider catalog."},
					"docs_url": schema.StringAttribute{Computed: true, MarkdownDescription: "Provider integration documentation URL."},
				}},
			},
		},
	}
}

func (d *integrationProvidersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *integrationProvidersDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	providers, err := d.client.ListIntegrationProviders(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Could not read integration providers", err.Error())
		return
	}
	slices.SortFunc(providers, func(a, b costgraph.IntegrationProvider) int { return cmp.Compare(a.Slug, b.Slug) })
	rows := make([]integrationProviderModel, len(providers))
	for i, provider := range providers {
		rows[i] = integrationProviderModel{
			Slug: types.StringValue(provider.Slug), Name: types.StringValue(provider.Name),
			Category: types.StringValue(provider.Category), DocsURL: types.StringValue(provider.DocsURL),
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, integrationProvidersDataSourceModel{ID: types.StringValue("focus-billing-providers"), Providers: rows})...)
}
