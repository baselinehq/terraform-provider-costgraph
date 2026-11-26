package provider

import (
	"context"
	"regexp"
	"slices"
	"strings"

	costgraph "github.com/baselinehq/costgraph-go"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.ResourceWithConfigure = &tenantResource{}
var _ resource.ResourceWithImportState = &tenantResource{}

type tenantResource struct {
	client *costgraph.Client
}

type tenantResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	OrganizationID types.String `tfsdk:"organization_id"`
	Active         types.Bool   `tfsdk:"active"`
}

func NewTenantResource() resource.Resource {
	return &tenantResource{}
}

func (r *tenantResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant"
}

func (r *tenantResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Create a tenant to separate infrastructure, integrations, and costs within your CostGraph organization.\n\nRequires an API key whose account is an active organization owner and whose explicitly granted scopes include organization:manage. Existing tenant keys do not gain this permission automatically. The accompanying organization management API must be available on the deployment.\n\nDestroying the resource deactivates the tenant and revokes its access. Historical records are retained. Reference this tenant from child integration resources so Terraform disconnects them before deactivating the tenant. CostGraph refuses deletion of the default tenant and the tenant that owns the active API key.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Tenant UUID assigned by CostGraph.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Tenant name. It must not be blank or begin or end with whitespace. Updates rename the tenant in place.",
				Validators:          []validator.String{tenantNameValidator{}},
			},
			"organization_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Organization UUID. Defaults to the organization associated with the API key. The key must have organization-management permission in this organization. Changing it replaces the tenant.",
				Validators: []validator.String{stringvalidator.RegexMatches(
					regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`), "Use a lowercase organization UUID.",
				)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"active": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the tenant is active. Active state is reported by CostGraph and is not a configurable setting.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *tenantResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *tenantResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan tenantResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	identity, err := r.client.CurrentIdentity(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Could not read organization identity", err.Error())
		return
	}
	if !slices.Contains(identity.PermissionScopes, costgraph.ScopeOrganizationManage) {
		resp.Diagnostics.AddError("Organization management scope is required", "Use an API key explicitly granted organization:manage by an active organization owner. A tenant-scoped key without this capability cannot create tenants.")
		return
	}
	organizationID := plan.OrganizationID.ValueString()
	if organizationID == "" {
		if identity.OrganizationID == nil || *identity.OrganizationID == "" {
			resp.Diagnostics.AddError("Organization is missing", "The configured credential did not return an organization identity.")
			return
		}
		organizationID = *identity.OrganizationID
	}
	tenant, err := r.client.CreateTenant(ctx, organizationID, costgraph.CreateTenantRequest{Name: plan.Name.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("Could not create tenant", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, tenantState(tenant))...)
}

func (r *tenantResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state tenantResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tenant, err := r.client.GetTenant(ctx, state.OrganizationID.ValueString(), state.ID.ValueString())
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read tenant", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, tenantState(tenant))...)
}

func (r *tenantResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan tenantResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tenant, err := r.client.UpdateTenant(ctx, plan.OrganizationID.ValueString(), plan.ID.ValueString(), costgraph.UpdateTenantRequest{Name: plan.Name.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("Could not update tenant", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, tenantState(tenant))...)
}

func (r *tenantResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state tenantResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteTenant(ctx, state.OrganizationID.ValueString(), state.ID.ValueString()); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Could not delete tenant", err.Error())
	}
}

func (r *tenantResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid tenant import ID", "Use organization_uuid/tenant_uuid to import a tenant.")
		return
	}
	organizationID, organizationErr := uuid.Parse(parts[0])
	tenantID, tenantErr := uuid.Parse(parts[1])
	if organizationErr != nil || tenantErr != nil {
		resp.Diagnostics.AddError("Invalid tenant import ID", "Both organization_uuid and tenant_uuid must be UUIDs.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("organization_id"), organizationID.String())...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), tenantID.String())...)
}

func tenantState(tenant *costgraph.ManagedTenant) tenantResourceModel {
	return tenantResourceModel{
		ID: types.StringValue(tenant.ID), Name: types.StringValue(tenant.Name),
		OrganizationID: types.StringValue(tenant.OrganizationID), Active: types.BoolValue(tenant.Active),
	}
}

type tenantNameValidator struct{}

func (tenantNameValidator) Description(_ context.Context) string {
	return "Tenant names must not be blank or have surrounding whitespace."
}

func (v tenantNameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v tenantNameValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	value := req.ConfigValue.ValueString()
	if value == "" || value != strings.TrimSpace(value) {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid tenant name", v.Description(ctx))
	}
}
