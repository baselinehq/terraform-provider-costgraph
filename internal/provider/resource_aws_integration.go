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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.ResourceWithConfigure = &awsIntegrationResource{}
var _ resource.ResourceWithImportState = &awsIntegrationResource{}

var cloudTenantUUIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type awsIntegrationResource struct {
	client *costgraph.Client
}

type awsIntegrationModel struct {
	ID                         types.String `tfsdk:"id"`
	ConnectionID               types.String `tfsdk:"connection_id"`
	TenantID                   types.String `tfsdk:"tenant_id"`
	BillingAccountID           types.String `tfsdk:"billing_account_id"`
	DisplayName                types.String `tfsdk:"display_name"`
	Region                     types.String `tfsdk:"region"`
	Bucket                     types.String `tfsdk:"bucket"`
	Prefix                     types.String `tfsdk:"prefix"`
	RoleARN                    types.String `tfsdk:"role_arn"`
	ExternalID                 types.String `tfsdk:"external_id"`
	ActivateCostAllocationTags types.Bool   `tfsdk:"activate_cost_allocation_tags"`
	EnableRecommendations      types.Bool   `tfsdk:"enable_recommendations"`
}

func NewAWSIntegrationResource() resource.Resource {
	return &awsIntegrationResource{}
}

func (r *awsIntegrationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_integration"
}

func cloudIntegrationAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed: true, MarkdownDescription: "Connection UUID used for management and import.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"connection_id": schema.StringAttribute{
			Computed: true, MarkdownDescription: "Public connection identifier. Use id for import.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"tenant_id": schema.StringAttribute{
			Required: true, MarkdownDescription: "Lowercase UUID of the tenant that receives the billing data. Changing it replaces the connection.",
			Validators:    []validator.String{stringvalidator.RegexMatches(cloudTenantUUIDPattern, "Use a lowercase tenant UUID.")},
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"billing_account_id": schema.StringAttribute{
			Required: true, MarkdownDescription: "Billing account whose export CostGraph reads. Changing it replaces the connection.",
			Validators:    []validator.String{trimmedStringValidator{}, stringvalidator.RegexMatches(regexp.MustCompile(`^\S+$`), "Use a billing account identifier without whitespace.")},
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"display_name": schema.StringAttribute{
			Optional: true, Computed: true, MarkdownDescription: "Name shown in CostGraph. Defaults to the billing account identifier. Omitting it after creation preserves the existing name.",
			Validators: []validator.String{trimmedStringValidator{}},
		},
		"enable_recommendations": schema.BoolAttribute{
			Optional: true, Computed: true, Default: booldefault.StaticBool(true),
			MarkdownDescription: "Allow CostGraph to collect the provider's savings recommendations. Defaults to true. The cloud identity needs the corresponding read permissions.",
		},
	}
}

func (r *awsIntegrationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := cloudIntegrationAttributes()
	attributes["region"] = schema.StringAttribute{
		Required: true, MarkdownDescription: "AWS region containing the billing export bucket.",
		Validators: []validator.String{trimmedStringValidator{}, stringvalidator.RegexMatches(regexp.MustCompile(`^\S+$`), "Use an AWS region without whitespace.")},
	}
	attributes["bucket"] = schema.StringAttribute{
		Required: true, MarkdownDescription: "S3 bucket containing the existing FOCUS billing export.",
		Validators: []validator.String{trimmedStringValidator{}, stringvalidator.RegexMatches(regexp.MustCompile(`^\S+$`), "Use an S3 bucket name without whitespace.")},
	}
	attributes["prefix"] = schema.StringAttribute{
		Optional: true, Computed: true, MarkdownDescription: "S3 prefix for the export. Defaults to an empty string on creation. Omit it during updates to keep the stored value. Set an empty string to clear it.",
	}
	attributes["role_arn"] = schema.StringAttribute{
		Required: true, MarkdownDescription: "ARN of the IAM role CostGraph assumes to read the export. Configure its trust policy before applying. This resource supports IAM role assumption only.",
		Validators: []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`^arn:(?:aws|aws-us-gov|aws-cn):iam::[0-9]{12}:role/[A-Za-z0-9+=,.@_/-]+$`), "Use an AWS IAM role ARN.")},
	}
	attributes["external_id"] = schema.StringAttribute{
		Computed: true, MarkdownDescription: "Your organization's external ID, returned after connection creation. Obtain it from CostGraph setup before configuring the IAM trust policy.",
		PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
	attributes["activate_cost_allocation_tags"] = schema.BoolAttribute{
		Optional: true, Computed: true, Default: booldefault.StaticBool(true),
		MarkdownDescription: "Allow CostGraph to activate discovered AWS cost allocation tags. Defaults to true. Set false when the read role must not change tag activation.",
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Connect an existing AWS FOCUS billing export through an IAM role. CostGraph verifies export access before it saves the connection.",
		Attributes:          attributes,
	}
}

func (r *awsIntegrationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *awsIntegrationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan awsIntegrationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	connected, err := r.client.ConnectAWSIntegration(ctx, costgraph.AWSConnectRequest{
		TenantID: plan.TenantID.ValueString(), BillingAccountID: plan.BillingAccountID.ValueString(),
		DisplayName: plan.DisplayName.ValueString(), Region: plan.Region.ValueString(),
		Bucket: plan.Bucket.ValueString(), Prefix: plan.Prefix.ValueString(),
		AssumeRole:                 costgraph.AWSAssumeRole{RoleARN: plan.RoleARN.ValueString()},
		ActivateCostAllocationTags: plan.ActivateCostAllocationTags.ValueBoolPointer(),
		EnableRecommendations:      plan.EnableRecommendations.ValueBoolPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Could not connect AWS", err.Error())
		return
	}
	plan.ID = types.StringValue(connected.ID)
	plan.ConnectionID = types.StringValue(connected.ConnectionID)
	plan.ExternalID = types.StringNull()
	if plan.DisplayName.IsUnknown() {
		plan.DisplayName = types.StringValue(plan.BillingAccountID.ValueString())
	}
	if plan.Prefix.IsUnknown() {
		plan.Prefix = types.StringValue("")
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	state, err := r.read(ctx, connected.ID)
	if err != nil {
		resp.Diagnostics.AddError("Could not read the created AWS connection", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *awsIntegrationResource) read(ctx context.Context, id string) (awsIntegrationModel, error) {
	settings, err := r.client.GetAWSIntegrationSettings(ctx, id)
	if err != nil {
		return awsIntegrationModel{}, err
	}
	if settings.Kind != "aws_assume_role" {
		return awsIntegrationModel{}, errors.New("this resource supports AWS assume-role connections only")
	}
	return awsIntegrationModel{
		ID: types.StringValue(settings.ID), ConnectionID: types.StringValue(settings.ConnectionID),
		TenantID: types.StringValue(settings.TenantID), BillingAccountID: types.StringValue(settings.BillingAccountID),
		DisplayName: types.StringValue(settings.DisplayName), Region: types.StringValue(settings.Region),
		Bucket: types.StringValue(settings.Bucket), Prefix: types.StringValue(settings.Prefix),
		RoleARN: types.StringValue(settings.AssumeRole.RoleARN), ExternalID: types.StringValue(settings.AssumeRole.ExternalID),
		ActivateCostAllocationTags: types.BoolValue(settings.ActivateCostAllocationTags),
		EnableRecommendations:      types.BoolValue(settings.EnableRecommendations),
	}, nil
}

func (r *awsIntegrationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var current awsIntegrationModel
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
		resp.Diagnostics.AddError("Could not read AWS connection", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *awsIntegrationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan awsIntegrationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.UpdateAWSIntegrationSettings(ctx, plan.ID.ValueString(), costgraph.AWSConnectionUpdateRequest{
		DisplayName: plan.DisplayName.ValueStringPointer(), Region: plan.Region.ValueStringPointer(),
		Bucket: plan.Bucket.ValueStringPointer(), Prefix: plan.Prefix.ValueStringPointer(),
		AssumeRole:                 &costgraph.AWSAssumeRoleUpdate{RoleARN: plan.RoleARN.ValueStringPointer()},
		ActivateCostAllocationTags: plan.ActivateCostAllocationTags.ValueBoolPointer(),
		EnableRecommendations:      plan.EnableRecommendations.ValueBoolPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Could not update AWS connection", err.Error())
		return
	}
	state, err := r.read(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not read updated AWS connection", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *awsIntegrationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state awsIntegrationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteConnection(ctx, state.ID.ValueString()); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Could not disconnect AWS", err.Error())
	}
}

func (r *awsIntegrationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !cloudTenantUUIDPattern.MatchString(req.ID) {
		resp.Diagnostics.AddError("Invalid connection UUID", "Use a lowercase connection UUID for import.")
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
