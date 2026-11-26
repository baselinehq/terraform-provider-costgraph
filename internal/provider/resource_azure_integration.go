package provider

import (
	"context"
	"regexp"
	"strings"

	costgraph "github.com/baselinehq/costgraph-go"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.ResourceWithConfigure = &azureIntegrationResource{}
var _ resource.ResourceWithImportState = &azureIntegrationResource{}
var _ resource.ResourceWithModifyPlan = &azureIntegrationResource{}

type azureIntegrationResource struct {
	client *costgraph.Client
}

type azureIntegrationModel struct {
	ID                    types.String `tfsdk:"id"`
	ConnectionID          types.String `tfsdk:"connection_id"`
	TenantID              types.String `tfsdk:"tenant_id"`
	AzureTenantID         types.String `tfsdk:"azure_tenant_id"`
	ClientID              types.String `tfsdk:"client_id"`
	ClientSecretWO        types.String `tfsdk:"client_secret_wo"`
	SecretVersion         types.Int64  `tfsdk:"secret_version"`
	StorageAccount        types.String `tfsdk:"storage_account"`
	Container             types.String `tfsdk:"container"`
	ExportName            types.String `tfsdk:"export_name"`
	DisplayName           types.String `tfsdk:"display_name"`
	EnableRecommendations types.Bool   `tfsdk:"enable_recommendations"`
}

func NewAzureIntegrationResource() resource.Resource {
	return &azureIntegrationResource{}
}

func (r *azureIntegrationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_azure_integration"
}

func (r *azureIntegrationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := cloudIntegrationAttributes()
	delete(attributes, "billing_account_id")
	name := attributes["display_name"].(schema.StringAttribute)
	name.MarkdownDescription = "Name shown in CostGraph. Defaults to the storage account name. Omitting it after creation preserves the stored name."
	attributes["display_name"] = name
	attributes["azure_tenant_id"] = schema.StringAttribute{
		Required: true, MarkdownDescription: "Microsoft Entra directory UUID, distinct from the CostGraph tenant_id. Changing it requires a new client_secret_wo and a higher secret_version.",
		Validators: []validator.String{stringvalidator.RegexMatches(cloudTenantUUIDPattern, "Use a lowercase Microsoft Entra directory UUID.")},
	}
	attributes["client_id"] = schema.StringAttribute{
		Required: true, MarkdownDescription: "Application UUID of the service principal reading the export. Changing it requires a new client_secret_wo and a higher secret_version.",
		Validators: []validator.String{stringvalidator.RegexMatches(cloudTenantUUIDPattern, "Use a lowercase service principal application UUID.")},
	}
	attributes["client_secret_wo"] = schema.StringAttribute{
		Optional: true, Sensitive: true, WriteOnly: true,
		MarkdownDescription: "Service principal secret used only on create or explicit rotation. Supply it through an ephemeral input variable. Terraform and OpenTofu do not store it in plan or state. Changing the secret alone does not rotate it. Increase secret_version to use a replacement secret.",
		Validators:          []validator.String{trimmedStringValidator{}},
	}
	attributes["secret_version"] = schema.Int64Attribute{
		Optional: true, Computed: true,
		MarkdownDescription: "Nonsecret rotation counter stored by CostGraph. Defaults to 1 on creation. Increase it and supply client_secret_wo to rotate. Legacy dashboard connections import with 0 until rotated. Omission preserves the current counter.",
		Validators:          []validator.Int64{int64validator.AtLeast(0)},
		PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
	}
	attributes["storage_account"] = schema.StringAttribute{
		Required: true, MarkdownDescription: "Existing Azure storage account containing the export. It identifies the connection. Changing it replaces the connection.",
		Validators:    []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`^[a-z0-9]{3,24}$`), "Use 3 to 24 lowercase letters or digits.")},
		PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
	}
	attributes["container"] = schema.StringAttribute{
		Required: true, MarkdownDescription: "Existing blob container containing the FOCUS export.",
		Validators: []validator.String{trimmedStringValidator{}},
	}
	attributes["export_name"] = schema.StringAttribute{
		Optional: true, Computed: true,
		MarkdownDescription: "Export folder. Defaults to an empty value, which reads the whole container. Omission preserves the stored value. Set an empty string to clear the folder selection.",
		Validators:          []validator.String{azureExportNameValidator{}},
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Connect an existing Azure FOCUS billing export through a service principal. Requires an organization owner's user API key with organization:manage and a backend supporting Azure management. CostGraph verifies export access before saving. Azure storage, the export, principal, and read permissions must already exist. Secrets are write-only and rotate through secret_version. Requires Terraform 1.11 or later or OpenTofu 1.11 or later. Import supports directly owned service-principal connections identified by their storage account. Destroy disconnects collection and removes the connection's data from CostGraph cost views. It does not delete Azure resources or revoke the principal's secret.",
		Attributes:          attributes,
	}
}

func (r *azureIntegrationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *azureIntegrationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan azureIntegrationModel
	var secret types.String
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("client_secret_wo"), &secret)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if secret.IsNull() || secret.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("client_secret_wo"), "Missing Azure client secret", "The service principal secret must be known when creating the connection.")
		return
	}
	if plan.SecretVersion.IsNull() || plan.SecretVersion.IsUnknown() {
		plan.SecretVersion = types.Int64Value(1)
	}
	connected, err := r.client.ConnectAzureIntegration(ctx, costgraph.AzureConnectRequest{
		TenantID: plan.TenantID.ValueString(), AzureTenantID: plan.AzureTenantID.ValueString(), ClientID: plan.ClientID.ValueString(),
		ClientSecret: secret.ValueStringPointer(), SecretVersion: plan.SecretVersion.ValueInt64Pointer(),
		StorageAccount: plan.StorageAccount.ValueString(), Container: plan.Container.ValueString(), ExportName: plan.ExportName.ValueString(),
		DisplayName: plan.DisplayName.ValueString(), EnableRecommendations: plan.EnableRecommendations.ValueBoolPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Could not connect Azure", err.Error())
		return
	}
	plan.ID = types.StringValue(connected.ID)
	plan.ConnectionID = types.StringValue(connected.ConnectionID)
	plan.ClientSecretWO = types.StringNull()
	if plan.DisplayName.IsUnknown() {
		plan.DisplayName = types.StringValue(plan.StorageAccount.ValueString())
	}
	if plan.ExportName.IsUnknown() {
		plan.ExportName = types.StringValue("")
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	state, err := r.read(ctx, connected.ID)
	if err != nil {
		resp.Diagnostics.AddError("Could not read the created Azure connection", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *azureIntegrationResource) read(ctx context.Context, id string) (azureIntegrationModel, error) {
	settings, err := r.client.GetAzureIntegrationSettings(ctx, id)
	if err != nil {
		return azureIntegrationModel{}, err
	}
	return azureIntegrationModel{
		ID: types.StringValue(settings.ID), ConnectionID: types.StringValue(settings.ConnectionID), TenantID: types.StringValue(settings.TenantID),
		AzureTenantID: types.StringValue(settings.AzureTenantID), ClientID: types.StringValue(settings.ClientID), ClientSecretWO: types.StringNull(),
		SecretVersion: types.Int64Value(settings.SecretVersion), StorageAccount: types.StringValue(settings.StorageAccount),
		Container: types.StringValue(settings.Container), ExportName: types.StringValue(settings.ExportName), DisplayName: types.StringValue(settings.DisplayName),
		EnableRecommendations: types.BoolValue(settings.EnableRecommendations),
	}, nil
}

func (r *azureIntegrationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var current azureIntegrationModel
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
		resp.Diagnostics.AddError("Could not read Azure connection", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *azureIntegrationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, previous azureIntegrationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &previous)...)
	if resp.Diagnostics.HasError() {
		return
	}
	update := costgraph.AzureConnectionUpdateRequest{
		AzureTenantID: plan.AzureTenantID.ValueStringPointer(), ClientID: plan.ClientID.ValueStringPointer(),
		Container: plan.Container.ValueStringPointer(), ExportName: plan.ExportName.ValueStringPointer(),
		DisplayName: plan.DisplayName.ValueStringPointer(), EnableRecommendations: plan.EnableRecommendations.ValueBoolPointer(),
	}
	if plan.SecretVersion.ValueInt64() != previous.SecretVersion.ValueInt64() {
		var secret types.String
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("client_secret_wo"), &secret)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if secret.IsNull() || secret.IsUnknown() {
			resp.Diagnostics.AddAttributeError(path.Root("client_secret_wo"), "Missing rotation secret", "The new service principal secret must be known when rotating the credential.")
			return
		}
		update.ClientSecret = secret.ValueStringPointer()
		update.SecretVersion = plan.SecretVersion.ValueInt64Pointer()
	}
	if _, err := r.client.UpdateAzureIntegrationSettings(ctx, plan.ID.ValueString(), update); err != nil {
		resp.Diagnostics.AddError("Could not update Azure connection", err.Error())
		return
	}
	state, err := r.read(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not read updated Azure connection", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *azureIntegrationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state azureIntegrationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteConnection(ctx, state.ID.ValueString()); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Could not disconnect Azure", err.Error())
	}
}

func (r *azureIntegrationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !cloudTenantUUIDPattern.MatchString(req.ID) {
		resp.Diagnostics.AddError("Invalid connection UUID", "Use a lowercase connection UUID for import.")
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *azureIntegrationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var plan, state azureIntegrationModel
	var secret types.String
	var configuredVersion types.Int64
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("client_secret_wo"), &secret)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("secret_version"), &configuredVersion)...)
	if !req.State.Raw.IsNull() {
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	creating := req.State.Raw.IsNull() || !plan.TenantID.IsUnknown() && !plan.TenantID.Equal(state.TenantID) || !plan.StorageAccount.IsUnknown() && !plan.StorageAccount.Equal(state.StorageAccount)
	if creating {
		if configuredVersion.IsNull() {
			plan.SecretVersion = types.Int64Value(1)
			resp.Diagnostics.Append(resp.Plan.Set(ctx, plan)...)
		}
		if !plan.SecretVersion.IsUnknown() && plan.SecretVersion.ValueInt64() < 1 {
			resp.Diagnostics.AddAttributeError(path.Root("secret_version"), "Invalid initial secret version", "New Azure connections require a secret_version of at least 1.")
		}
		if secret.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("client_secret_wo"), "Missing Azure client secret", "Supply the service principal secret to create or replace the connection.")
		}
		return
	}
	if plan.SecretVersion.IsUnknown() {
		return
	}
	if plan.SecretVersion.ValueInt64() < state.SecretVersion.ValueInt64() {
		resp.Diagnostics.AddAttributeError(path.Root("secret_version"), "Secret version cannot decrease", "Increase the counter when rotating the secret, or keep its current value.")
		return
	}
	rotating := plan.SecretVersion.ValueInt64() > state.SecretVersion.ValueInt64()
	identityChanged := !plan.AzureTenantID.IsUnknown() && !plan.AzureTenantID.Equal(state.AzureTenantID) || !plan.ClientID.IsUnknown() && !plan.ClientID.Equal(state.ClientID)
	if identityChanged && !rotating {
		resp.Diagnostics.AddAttributeError(path.Root("secret_version"), "Service principal change requires a new secret", "Supply client_secret_wo and increase secret_version when changing azure_tenant_id or client_id.")
	}
	if rotating && secret.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("client_secret_wo"), "Missing rotation secret", "Supply a new client_secret_wo when increasing secret_version.")
	}
}

type azureExportNameValidator struct{}

func (azureExportNameValidator) Description(context.Context) string {
	return "The export folder cannot start or end with whitespace. An empty value clears the folder selection."
}

func (v azureExportNameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v azureExportNameValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if value := req.ConfigValue.ValueString(); value != strings.TrimSpace(value) {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid export folder", v.Description(ctx))
	}
}
