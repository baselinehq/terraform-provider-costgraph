package provider

import (
	"context"
	"regexp"

	costgraph "github.com/baselinehq/costgraph-go"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var _ resource.ResourceWithConfigure = &focusPushIntegrationResource{}
var _ resource.ResourceWithImportState = &focusPushIntegrationResource{}

type focusPushIntegrationResource struct {
	client *costgraph.Client
}

func NewFocusPushIntegrationResource() resource.Resource {
	return &focusPushIntegrationResource{}
}

func (r *focusPushIntegrationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_focus_push_integration"
}

func (r *focusPushIntegrationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Register a provider account for FOCUS billing records sent to CostGraph. The resource returns the connection ID used for ingestion. Creating it does not send records or create an API key. Requires an API key belonging to an organization owner, admin, or billing admin with access to the tenant. Destroying the resource disconnects the account and removes its ingested data from cost views.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, MarkdownDescription: "Connection UUID used for management and import.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"connection_id": schema.StringAttribute{
				Computed: true, MarkdownDescription: "Public connection ID beginning with fpc_ used when sending FOCUS records. Use id for import.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"provider_name": schema.StringAttribute{
				Required: true, MarkdownDescription: "Canonical provider name, such as aws, gcp, or azure, using lowercase letters and digits. Changing it replaces the connection.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(regexp.MustCompile(`^[a-z0-9]+$`), "Use a canonical provider name with lowercase letters and digits."),
					stringvalidator.NoneOf("googlecloud", "googlecloudplatform", "amazonwebservices", "microsoft", "microsoftazure"),
				},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"provider_account_id": schema.StringAttribute{
				Required: true, MarkdownDescription: "Billing account identifier present in the FOCUS records. Changing it replaces the connection.",
				Validators:    []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`\S`), "The billing account identifier must not be blank.")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"tenant_id": schema.StringAttribute{
				Optional: true, Computed: true, MarkdownDescription: "Lowercase tenant UUID. Defaults to the tenant of the API key. The key must have access to this tenant. Changing it replaces the connection.",
				Validators:    []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`), "Use a lowercase tenant UUID.")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"display_name": schema.StringAttribute{
				Optional: true, Computed: true, MarkdownDescription: "Name shown in CostGraph. Defaults to the billing account identifier when created. Updates rename the connection in place. Omitting this after creation keeps the existing name.",
				Validators: []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`\S`), "The display name must not be blank.")},
			},
		},
	}
}

func (r *focusPushIntegrationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *focusPushIntegrationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan connectionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	connection, err := r.client.CreatePushConnection(ctx, costgraph.CreatePushConnectionRequest{
		Provider: plan.Provider.ValueString(), ProviderAccountID: plan.ProviderAccountID.ValueString(),
		TenantID: plan.TenantID.ValueString(), DisplayName: plan.DisplayName.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Could not create FOCUS connection", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, connectionState(connection))...)
}

func (r *focusPushIntegrationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state connectionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	connection, err := r.client.GetConnection(ctx, state.ID.ValueString())
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read FOCUS connection", err.Error())
		return
	}
	if connection.IngestMode != "push" {
		resp.Diagnostics.AddError("Connection does not accept FOCUS pushes", "This resource can manage only a connection whose ingest_mode is push.")
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, connectionState(connection))...)
}

func (r *focusPushIntegrationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan connectionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !plan.DisplayName.IsUnknown() && !plan.DisplayName.IsNull() {
		if err := r.client.RenameConnection(ctx, plan.ID.ValueString(), plan.DisplayName.ValueString()); err != nil {
			resp.Diagnostics.AddError("Could not rename FOCUS connection", err.Error())
			return
		}
	}
	connection, err := r.client.GetConnection(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not read updated FOCUS connection", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, connectionState(connection))...)
}

func (r *focusPushIntegrationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state connectionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteConnection(ctx, state.ID.ValueString()); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Could not disconnect FOCUS connection", err.Error())
	}
}

func (r *focusPushIntegrationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
