package provider

import (
	"context"
	"os"

	costgraph "github.com/baselinehq/costgraph-go"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = &CostGraphProvider{}

type CostGraphProvider struct {
	version string
}

type providerModel struct {
	APIKey types.String `tfsdk:"api_key"`
	APIURL types.String `tfsdk:"api_url"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &CostGraphProvider{version: version}
	}
}

func (p *CostGraphProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "costgraph"
	resp.Version = p.version
}

func (p *CostGraphProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage CostGraph configuration in hosted or self-hosted deployments using an API key. Use the API URL of the deployment you want to manage.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Optional: true, Sensitive: true,
				MarkdownDescription: "CostGraph API key for the target deployment. Defaults to the `COSTGRAPH_API_KEY` environment variable. Explicit configuration takes precedence. Keep the key in an environment variable or a sensitive input variable.",
			},
			"api_url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Base URL of the CostGraph API, without `/api/v1`. Defaults to `COSTGRAPH_API_URL`, then `https://api.costgraph.ai`. Set this to your self-hosted API URL, including any reverse-proxy prefix. Use the API URL rather than the dashboard or pricing API URL. Explicit configuration takes precedence.",
			},
		},
	}
}

func (p *CostGraphProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if config.APIKey.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("api_key"), "Unknown API key", "The API key must be known before CostGraph can be configured.")
	}
	if config.APIURL.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("api_url"), "Unknown API URL", "The API URL must be known before CostGraph can be configured.")
	}
	if resp.Diagnostics.HasError() {
		return
	}
	apiKey := os.Getenv("COSTGRAPH_API_KEY")
	if !config.APIKey.IsNull() {
		apiKey = config.APIKey.ValueString()
	}
	apiURL := os.Getenv("COSTGRAPH_API_URL")
	if apiURL == "" {
		apiURL = "https://api.costgraph.ai"
	}
	if !config.APIURL.IsNull() {
		apiURL = config.APIURL.ValueString()
	}
	client, err := costgraph.New(apiURL, apiKey)
	if err != nil {
		resp.Diagnostics.AddError("Invalid CostGraph configuration", err.Error())
		return
	}
	resp.ResourceData = client
	resp.DataSourceData = client
}

func (p *CostGraphProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewFocusPushIntegrationResource, NewTenantResource, NewAIGatewayIntegrationResource,
		NewAWSIntegrationResource, NewGoogleCloudIntegrationResource,
	}
}

func (p *CostGraphProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewIntegrationDataSource, NewCurrentOrganizationDataSource, NewTenantDataSource,
		NewIntegrationsDataSource, NewIntegrationProvidersDataSource,
	}
}
