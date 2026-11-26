package provider

import (
	"context"
	"errors"
	"regexp"

	costgraph "github.com/baselinehq/costgraph-go"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.ResourceWithConfigure = &googleCloudIntegrationResource{}
var _ resource.ResourceWithImportState = &googleCloudIntegrationResource{}

type googleCloudIntegrationResource struct {
	client *costgraph.Client
}

type googleCloudIntegrationModel struct {
	ID                    types.String `tfsdk:"id"`
	ConnectionID          types.String `tfsdk:"connection_id"`
	TenantID              types.String `tfsdk:"tenant_id"`
	BillingAccountID      types.String `tfsdk:"billing_account_id"`
	DisplayName           types.String `tfsdk:"display_name"`
	QueryProject          types.String `tfsdk:"query_project"`
	TargetPrincipal       types.String `tfsdk:"target_principal"`
	EnableRecommendations types.Bool   `tfsdk:"enable_recommendations"`
}

func NewGoogleCloudIntegrationResource() resource.Resource {
	return &googleCloudIntegrationResource{}
}

func (r *googleCloudIntegrationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_google_cloud_integration"
}

func (r *googleCloudIntegrationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := cloudIntegrationAttributes()
	attributes["query_project"] = schema.StringAttribute{
		Required: true, MarkdownDescription: "Existing Google Cloud project where CostGraph queries the BigQuery billing export.",
		Validators: []validator.String{trimmedStringValidator{}, stringvalidator.RegexMatches(regexp.MustCompile(`^\S+$`), "Use a project identifier without whitespace.")},
	}
	attributes["target_principal"] = schema.StringAttribute{
		Required: true, MarkdownDescription: "Service account principal CostGraph impersonates to read the billing export. Configure impersonation and BigQuery access before creating the connection. Service-account JSON keys are not supported by this resource.",
		Validators: []validator.String{trimmedStringValidator{}, stringvalidator.RegexMatches(regexp.MustCompile(`^\S+$`), "Use a service account principal without whitespace.")},
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Connect an existing Google Cloud billing export through service account impersonation. Requires an organization owner's user API key with the organization:manage scope. CostGraph verifies real export access before saving settings. Service account, federation, and BigQuery export setup remain outside this resource. Updates preserve the connection UUID. Import uses its UUID and supports impersonation connections only. Destroying the resource disconnects collection and removes the account's billing data from cost views.",
		Attributes:          attributes,
	}
}

func (r *googleCloudIntegrationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *googleCloudIntegrationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan googleCloudIntegrationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	connected, err := r.client.ConnectGoogleCloudIntegration(ctx, costgraph.GCPConnectRequest{
		TenantID: plan.TenantID.ValueString(), BillingAccountID: plan.BillingAccountID.ValueString(),
		DisplayName:           plan.DisplayName.ValueString(),
		Impersonation:         costgraph.GCPImpersonation{QueryProject: plan.QueryProject.ValueString(), TargetPrincipal: plan.TargetPrincipal.ValueString()},
		EnableRecommendations: plan.EnableRecommendations.ValueBoolPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Could not connect Google Cloud", err.Error())
		return
	}
	plan.ID = types.StringValue(connected.ID)
	plan.ConnectionID = types.StringValue(connected.ConnectionID)
	if plan.DisplayName.IsUnknown() {
		plan.DisplayName = types.StringValue(plan.BillingAccountID.ValueString())
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	state, err := r.read(ctx, connected.ID)
	if err != nil {
		resp.Diagnostics.AddError("Could not read the created Google Cloud connection", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *googleCloudIntegrationResource) read(ctx context.Context, id string) (googleCloudIntegrationModel, error) {
	settings, err := r.client.GetGoogleCloudIntegrationSettings(ctx, id)
	if err != nil {
		return googleCloudIntegrationModel{}, err
	}
	if settings.Kind != "gcp_impersonation" {
		return googleCloudIntegrationModel{}, errors.New("this resource supports Google Cloud impersonation connections only")
	}
	return googleCloudIntegrationModel{
		ID: types.StringValue(settings.ID), ConnectionID: types.StringValue(settings.ConnectionID),
		TenantID: types.StringValue(settings.TenantID), BillingAccountID: types.StringValue(settings.BillingAccountID),
		DisplayName: types.StringValue(settings.DisplayName), QueryProject: types.StringValue(settings.Impersonation.QueryProject),
		TargetPrincipal:       types.StringValue(settings.Impersonation.TargetPrincipal),
		EnableRecommendations: types.BoolValue(settings.EnableRecommendations),
	}, nil
}

func (r *googleCloudIntegrationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var current googleCloudIntegrationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &current)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state, err := r.read(ctx, current.ID.ValueString())
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read Google Cloud connection", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *googleCloudIntegrationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan googleCloudIntegrationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.UpdateGoogleCloudIntegrationSettings(ctx, plan.ID.ValueString(), costgraph.GCPConnectionUpdateRequest{
		DisplayName:           plan.DisplayName.ValueStringPointer(),
		Impersonation:         &costgraph.GCPImpersonationUpdate{QueryProject: plan.QueryProject.ValueStringPointer(), TargetPrincipal: plan.TargetPrincipal.ValueStringPointer()},
		EnableRecommendations: plan.EnableRecommendations.ValueBoolPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Could not update Google Cloud connection", err.Error())
		return
	}
	state, err := r.read(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not read updated Google Cloud connection", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *googleCloudIntegrationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state googleCloudIntegrationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteConnection(ctx, state.ID.ValueString()); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Could not disconnect Google Cloud", err.Error())
	}
}

func (r *googleCloudIntegrationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !cloudTenantUUIDPattern.MatchString(req.ID) {
		resp.Diagnostics.AddError("Invalid connection UUID", "Use a lowercase connection UUID for import.")
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
