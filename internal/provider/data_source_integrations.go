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

var _ datasource.DataSourceWithConfigure = &integrationsDataSource{}

type integrationsDataSource struct {
	client *costgraph.Client
}

type integrationsDataSourceModel struct {
	ID           types.String              `tfsdk:"id"`
	Integrations []integrationSummaryModel `tfsdk:"integrations"`
}

type integrationSummaryModel struct {
	ID                types.String `tfsdk:"id"`
	ConnectionID      types.String `tfsdk:"connection_id"`
	ProviderName      types.String `tfsdk:"provider_name"`
	ProviderAccountID types.String `tfsdk:"provider_account_id"`
	DisplayName       types.String `tfsdk:"display_name"`
	TenantID          types.String `tfsdk:"tenant_id"`
	Status            types.String `tfsdk:"status"`
	IngestMode        types.String `tfsdk:"ingest_mode"`
}

func NewIntegrationsDataSource() datasource.DataSource {
	return &integrationsDataSource{}
}

func (d *integrationsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_integrations"
}

func (d *integrationsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "List integration connections in the tenant bound to the configured API key. Returns common metadata without credentials, sorted by connection UUID. An empty tenant returns an empty list. The API returns the complete tenant list without pagination.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, MarkdownDescription: "UUID of the tenant bound to the configured API key."},
			"integrations": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Tenant integration connections sorted by their internal UUID.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":                  schema.StringAttribute{Computed: true, MarkdownDescription: "Internal connection UUID used for management."},
					"connection_id":       schema.StringAttribute{Computed: true, MarkdownDescription: "Public connection identifier."},
					"provider_name":       schema.StringAttribute{Computed: true, MarkdownDescription: "Canonical provider name."},
					"provider_account_id": schema.StringAttribute{Computed: true, MarkdownDescription: "Provider billing account identifier."},
					"display_name":        schema.StringAttribute{Computed: true, MarkdownDescription: "Connection name shown in CostGraph."},
					"tenant_id":           schema.StringAttribute{Computed: true, MarkdownDescription: "Tenant that owns the connection."},
					"status":              schema.StringAttribute{Computed: true, MarkdownDescription: "Connection status reported by CostGraph."},
					"ingest_mode":         schema.StringAttribute{Computed: true, MarkdownDescription: "How the connection collects or uses data, such as push, pull, or telemetry."},
				}},
			},
		},
	}
}

func (d *integrationsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *integrationsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	identity, err := d.client.CurrentIdentity(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Could not read tenant identity", err.Error())
		return
	}
	if len(identity.Tenants) != 1 || identity.Tenants[0].ID == "" {
		resp.Diagnostics.AddError("Tenant scope is unavailable", "This data source requires an API key bound to exactly one tenant.")
		return
	}
	connections, err := d.client.ListConnections(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Could not list integrations", err.Error())
		return
	}
	slices.SortFunc(connections, func(a, b costgraph.Connection) int { return cmp.Compare(a.ID, b.ID) })
	rows := make([]integrationSummaryModel, len(connections))
	for i, connection := range connections {
		rows[i] = integrationSummaryModel{
			ID: types.StringValue(connection.ID), ConnectionID: types.StringValue(connection.ConnectionID),
			ProviderName: types.StringValue(connection.Provider), ProviderAccountID: types.StringValue(connection.ProviderAccountID),
			DisplayName: types.StringValue(connection.DisplayName), TenantID: types.StringValue(connection.TenantID),
			Status: types.StringValue(connection.Status), IngestMode: types.StringValue(connection.IngestMode),
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, integrationsDataSourceModel{ID: types.StringValue(identity.Tenants[0].ID), Integrations: rows})...)
}
