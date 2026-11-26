package provider

import (
	"context"
	"fmt"
	"regexp"

	costgraph "github.com/baselinehq/costgraph-go"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.ResourceWithConfigure = &aiGatewayIntegrationResource{}
var _ resource.ResourceWithImportState = &aiGatewayIntegrationResource{}

type aiGatewayIntegrationResource struct{ client *costgraph.Client }

type aiGatewayModel struct {
	ID                types.String `tfsdk:"id"`
	ConnectionID      types.String `tfsdk:"connection_id"`
	ProviderName      types.String `tfsdk:"provider_name"`
	ProviderAccountID types.String `tfsdk:"provider_account_id"`
	TenantID          types.String `tfsdk:"tenant_id"`
	DisplayName       types.String `tfsdk:"display_name"`
	ClusterID         types.String `tfsdk:"cluster_id"`
	InstanceID        types.String `tfsdk:"instance_id"`
	ScrapeNamespace   types.String `tfsdk:"scrape_namespace"`
	ScrapeSelector    types.String `tfsdk:"scrape_label_selector"`
	ScrapePort        types.Int64  `tfsdk:"scrape_port"`
	ModelMap          types.Map    `tfsdk:"model_map"`
	TenantMap         types.Map    `tfsdk:"tenant_map"`
	IngestMode        types.String `tfsdk:"ingest_mode"`
}

func NewAIGatewayIntegrationResource() resource.Resource { return &aiGatewayIntegrationResource{} }

func (r *aiGatewayIntegrationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_gateway_integration"
}

func (r *aiGatewayIntegrationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	uuidValidator := stringvalidator.RegexMatches(regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`), "Use a lowercase UUID.")
	textValidator := trimmedStringValidator{}
	emptyMap := types.MapValueMust(types.StringType, map[string]attr.Value{})
	resp.Schema = schema.Schema{
		MarkdownDescription: "Connect an AI gateway to collect token usage and associate reported models and sub-accounts with pricing and display names.\n\nRequires an owner, admin, or billing admin API key with access to the tenant. To configure a newly created tenant, use an organization owner's key explicitly granted organization:manage.\n\nA connection without a cluster or instance accepts pushed usage. Collection through a cluster or instance requires a running CostGraph collector registered in that tenant.\n\nDestroying the resource disconnects the gateway and removes its ingested data from cost views. Advanced metric bindings and attribution rules remain managed outside this resource.",
		Attributes: map[string]schema.Attribute{
			"id":                    schema.StringAttribute{Computed: true, MarkdownDescription: "Connection UUID used for management and import.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"connection_id":         schema.StringAttribute{Computed: true, MarkdownDescription: "Public connection ID used for ingestion. Use id for import.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"provider_name":         schema.StringAttribute{Required: true, MarkdownDescription: "Gateway identifier from the CostGraph integration catalog, such as agentgateway or envoy_ai_gateway. Changing it replaces the connection.", Validators: []validator.String{textValidator}, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"provider_account_id":   schema.StringAttribute{Required: true, MarkdownDescription: "Unique name for this gateway account in the tenant. Changing it replaces the connection.", Validators: []validator.String{textValidator}, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"tenant_id":             schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Lowercase tenant UUID. Defaults to the API key's tenant. Changing it replaces the connection.", Validators: []validator.String{uuidValidator}, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()}},
			"display_name":          schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Name shown in CostGraph. Defaults to provider_account_id. Omitting it after creation keeps the existing name.", Validators: []validator.String{textValidator}},
			"cluster_id":            schema.StringAttribute{Optional: true, MarkdownDescription: "Registered cluster UUID for operator collection. Conflicts with instance_id. Removing both scope fields switches the connection to pushed usage.", Validators: []validator.String{uuidValidator, stringvalidator.ConflictsWith(path.MatchRoot("instance_id"))}},
			"instance_id":           schema.StringAttribute{Optional: true, MarkdownDescription: "Registered instance UUID for host-agent collection. Conflicts with cluster_id.", Validators: []validator.String{uuidValidator, stringvalidator.ConflictsWith(path.MatchRoot("cluster_id"))}},
			"scrape_namespace":      schema.StringAttribute{Optional: true, MarkdownDescription: "Kubernetes namespace to scrape. Omit to use the gateway default. Required with cluster_id when the gateway has no default namespace.", Validators: []validator.String{textValidator}},
			"scrape_label_selector": schema.StringAttribute{Optional: true, MarkdownDescription: "Pod label selector for collection. Omit to use the gateway default. Required with cluster_id when the gateway has no default selector.", Validators: []validator.String{textValidator}},
			"scrape_port":           schema.Int64Attribute{Optional: true, MarkdownDescription: "Metrics port from 1 through 65535. Omit to use the gateway default.", Validators: []validator.Int64{int64validator.Between(1, 65535)}},
			"model_map":             schema.MapAttribute{Optional: true, Computed: true, ElementType: types.StringType, Default: mapdefault.StaticValue(emptyMap), MarkdownDescription: "Map reported model names to canonical model keys from the CostGraph catalog. Aliases are not canonical keys. Defaults to an empty map. Removing an entry removes its mapping and can reprice recent usage.", Validators: []validator.Map{mapvalidator.SizeAtMost(500), mapvalidator.KeysAre(textValidator), mapvalidator.ValueStringsAre(textValidator)}},
			"tenant_map":            schema.MapAttribute{Optional: true, Computed: true, ElementType: types.StringType, Default: mapdefault.StaticValue(emptyMap), MarkdownDescription: "Map reported sub-account identifiers to display names. Defaults to an empty map. Removing an entry removes the display-name override.", Validators: []validator.Map{mapvalidator.KeysAre(textValidator), mapvalidator.ValueStringsAre(textValidator)}},
			"ingest_mode":           schema.StringAttribute{Computed: true, MarkdownDescription: "Collection mode. push accepts usage posted to the connection. telemetry collects through the configured cluster or instance."},
		},
	}
}

func (r *aiGatewayIntegrationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*costgraph.Client)
	if !ok {
		resp.Diagnostics.AddError("Invalid provider client", "The provider did not configure a CostGraph management client.")
		return
	}
	r.client = client
}

func (r *aiGatewayIntegrationResource) read(ctx context.Context, id string) (aiGatewayModel, error) {
	connection, err := r.client.GetConnection(ctx, id)
	if err != nil {
		return aiGatewayModel{}, err
	}
	settings, err := r.client.GetAIGatewaySettings(ctx, id)
	if err != nil {
		return aiGatewayModel{}, err
	}
	models, diags := types.MapValueFrom(ctx, types.StringType, settings.ModelMap)
	if diags.HasError() {
		return aiGatewayModel{}, fmt.Errorf("could not read model mappings")
	}
	tenants, diags := types.MapValueFrom(ctx, types.StringType, settings.TenantMap)
	if diags.HasError() {
		return aiGatewayModel{}, fmt.Errorf("could not read sub-account mappings")
	}
	port := types.Int64Null()
	if settings.ScrapePort != nil {
		port = types.Int64Value(int64(*settings.ScrapePort))
	}
	return aiGatewayModel{ID: types.StringValue(connection.ID), ConnectionID: types.StringValue(connection.ConnectionID), ProviderName: types.StringValue(connection.Provider), ProviderAccountID: types.StringValue(connection.ProviderAccountID), TenantID: types.StringValue(connection.TenantID), DisplayName: types.StringValue(connection.DisplayName), ClusterID: types.StringPointerValue(settings.ClusterID), InstanceID: types.StringPointerValue(settings.InstanceID), ScrapeNamespace: types.StringPointerValue(settings.ScrapeNamespace), ScrapeSelector: types.StringPointerValue(settings.ScrapeLabelSelector), ScrapePort: port, ModelMap: models, TenantMap: tenants, IngestMode: types.StringValue(settings.IngestMode)}, nil
}

func gatewayUpdate(ctx context.Context, plan aiGatewayModel) (costgraph.AIGatewayUpdateRequest, diag.Diagnostics) {
	models, tenants := map[string]string{}, map[string]string{}
	diags := plan.ModelMap.ElementsAs(ctx, &models, false)
	diags.Append(plan.TenantMap.ElementsAs(ctx, &tenants, false)...)
	cluster, instance, namespace, selector := plan.ClusterID.ValueString(), plan.InstanceID.ValueString(), plan.ScrapeNamespace.ValueString(), plan.ScrapeSelector.ValueString()
	port := int(plan.ScrapePort.ValueInt64())
	return costgraph.AIGatewayUpdateRequest{ClusterID: &cluster, InstanceID: &instance, ScrapeNamespace: &namespace, ScrapeLabelSelector: &selector, ScrapePort: &port, ModelMap: &models, TenantMap: &tenants}, diags
}

func (r *aiGatewayIntegrationResource) validateModelMap(ctx context.Context, requested, current map[string]string) error {
	validated := map[string]bool{}
	for name, target := range requested {
		if current[name] == target || validated[target] {
			continue
		}
		models, err := r.client.SearchCatalogModels(ctx, target)
		if err != nil {
			return err
		}
		for _, model := range models {
			if model.Key == target {
				validated[target] = true
				break
			}
		}
		if !validated[target] {
			return fmt.Errorf("model_map target %q is not a canonical model key in this deployment's catalog. Select the model's exact key rather than an alias", target)
		}
	}
	return nil
}

func (r *aiGatewayIntegrationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan aiGatewayModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tenantID := plan.TenantID.ValueString()
	if tenantID == "" {
		identity, err := r.client.CurrentIdentity(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Could not resolve tenant", err.Error())
			return
		}
		if len(identity.Tenants) != 1 {
			resp.Diagnostics.AddError("Tenant is required", "Set tenant_id when the configured credential does not identify exactly one tenant.")
			return
		}
		tenantID = identity.Tenants[0].ID
	}
	update, diags := gatewayUpdate(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.validateModelMap(ctx, *update.ModelMap, nil); err != nil {
		resp.Diagnostics.AddError("Invalid model mapping", err.Error())
		return
	}
	connection, err := r.client.CreateAIGatewayConnection(ctx, costgraph.AIGatewayConnectRequest{TenantID: tenantID, Provider: plan.ProviderName.ValueString(), ProviderAccountID: plan.ProviderAccountID.ValueString(), DisplayName: plan.DisplayName.ValueString(), ClusterID: plan.ClusterID.ValueString(), InstanceID: plan.InstanceID.ValueString(), ScrapeNamespace: plan.ScrapeNamespace.ValueString(), ScrapeLabelSelector: plan.ScrapeSelector.ValueString(), ScrapePort: int(plan.ScrapePort.ValueInt64()), ModelMap: *update.ModelMap})
	if err != nil {
		resp.Diagnostics.AddError("Could not connect AI gateway", err.Error())
		return
	}
	plan.ID = types.StringValue(connection.ID)
	plan.ConnectionID = types.StringValue(connection.ConnectionID)
	plan.TenantID = types.StringValue(connection.TenantID)
	plan.DisplayName = types.StringValue(connection.DisplayName)
	plan.IngestMode = types.StringValue(connection.IngestMode)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if _, err = r.client.UpdateAIGatewayConnection(ctx, connection.ID, update); err != nil {
		resp.Diagnostics.AddError("Could not configure AI gateway", err.Error())
		return
	}
	state, err := r.read(ctx, connection.ID)
	if err != nil {
		resp.Diagnostics.AddError("Could not read AI gateway", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *aiGatewayIntegrationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state aiGatewayModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	next, err := r.read(ctx, state.ID.ValueString())
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read AI gateway", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, next)...)
}

func (r *aiGatewayIntegrationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan aiGatewayModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	update, diags := gatewayUpdate(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	var previous aiGatewayModel
	resp.Diagnostics.Append(req.State.Get(ctx, &previous)...)
	current := map[string]string{}
	resp.Diagnostics.Append(previous.ModelMap.ElementsAs(ctx, &current, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.validateModelMap(ctx, *update.ModelMap, current); err != nil {
		resp.Diagnostics.AddError("Invalid model mapping", err.Error())
		return
	}
	if !plan.DisplayName.IsUnknown() && !plan.DisplayName.IsNull() {
		if err := r.client.RenameConnection(ctx, plan.ID.ValueString(), plan.DisplayName.ValueString()); err != nil {
			resp.Diagnostics.AddError("Could not rename AI gateway", err.Error())
			return
		}
	}
	if _, err := r.client.UpdateAIGatewayConnection(ctx, plan.ID.ValueString(), update); err != nil {
		resp.Diagnostics.AddError("Could not update AI gateway", err.Error())
		return
	}
	state, err := r.read(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not read updated AI gateway", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *aiGatewayIntegrationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state aiGatewayModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteConnection(ctx, state.ID.ValueString()); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Could not disconnect AI gateway", err.Error())
	}
}

func (r *aiGatewayIntegrationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
