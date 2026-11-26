package provider

import (
	"context"
	"regexp"

	costgraph "github.com/baselinehq/costgraph-go"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var _ datasource.DataSourceWithConfigure = &integrationDataSource{}

type integrationDataSource struct {
	client *costgraph.Client
}

func NewIntegrationDataSource() datasource.DataSource {
	return &integrationDataSource{}
}

func (d *integrationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_integration"
}

func (d *integrationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing CostGraph integration by its connection UUID. Returns common connection metadata without credentials. The API key must have access to the connection's tenant. A missing connection returns an error.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required: true, MarkdownDescription: "Lowercase connection UUID from CostGraph, rather than the fpc_ ingestion ID.",
				Validators: []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`), "Use a lowercase connection UUID.")},
			},
			"connection_id":       schema.StringAttribute{Computed: true, MarkdownDescription: "Public connection identifier."},
			"provider_name":       schema.StringAttribute{Computed: true, MarkdownDescription: "Canonical provider name."},
			"provider_account_id": schema.StringAttribute{Computed: true, MarkdownDescription: "Provider billing account identifier."},
			"tenant_id":           schema.StringAttribute{Computed: true, MarkdownDescription: "Tenant that owns the connection."},
			"display_name":        schema.StringAttribute{Computed: true, MarkdownDescription: "Connection name shown in CostGraph."},
		},
	}
}

func (d *integrationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *integrationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config connectionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	connection, err := d.client.GetConnection(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not read integration", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, connectionState(connection))...)
}
