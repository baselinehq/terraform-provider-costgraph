package provider

import (
	"context"
	"errors"
	"regexp"
	"slices"

	costgraph "github.com/baselinehq/costgraph-go"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.ResourceWithConfigure = &kubernetesClusterResource{}
var _ resource.ResourceWithImportState = &kubernetesClusterResource{}

type kubernetesClusterResource struct{ client *costgraph.Client }

type kubernetesClusterModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	TenantID       types.String `tfsdk:"tenant_id"`
	OrganizationID types.String `tfsdk:"organization_id"`
}

func NewKubernetesClusterResource() resource.Resource { return &kubernetesClusterResource{} }

func (r *kubernetesClusterResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_kubernetes_cluster"
}

func (r *kubernetesClusterResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reserve a Kubernetes cluster name in a CostGraph tenant before installing the operator. This resource does not create a Kubernetes cluster, deploy the operator, issue credentials, or report collection health. The operator reports versions, inventory, and heartbeats separately and adopts the reserved registration when its cluster name and tenant match.\n\nRequires an organization owner's user API key explicitly granted organization:manage and a deployment with the Kubernetes registration management API. Keep the operator's tenant-bound collection credential in an existing Kubernetes Secret.\n\nImport an existing active registration by its cluster UUID. An active name in the same tenant cannot be created twice. Changing the name or tenant replaces the registration. Destroying it disconnects collection and retains historical records. Remove the operator first, or reference this resource from its Helm release so Terraform orders removal. A collector cannot recreate a disconnected name until an owner explicitly reserves it again, which creates a new UUID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, MarkdownDescription: "Cluster registration UUID used for management and import.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required: true, MarkdownDescription: "Cluster name, matching global.clusterName in the operator Helm chart. It must not be blank or have surrounding whitespace. Changing it replaces the registration.",
				Validators:    []validator.String{trimmedStringValidator{}},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"tenant_id": schema.StringAttribute{
				Required: true, MarkdownDescription: "Lowercase UUID of the tenant that owns the collector credential. Changing it replaces the registration.",
				Validators:    []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`), "Use a lowercase tenant UUID.")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"organization_id": schema.StringAttribute{
				Computed: true, MarkdownDescription: "Organization UUID associated with the management API key.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *kubernetesClusterResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *kubernetesClusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan kubernetesClusterModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	organizationID, err := r.organizationID(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Could not read organization identity", err.Error())
		return
	}
	cluster, err := r.client.CreateKubernetesCluster(ctx, organizationID, costgraph.CreateKubernetesClusterRequest{Name: plan.Name.ValueString(), TenantID: plan.TenantID.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("Could not reserve Kubernetes cluster", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, kubernetesClusterState(cluster, organizationID))...)
}

func (r *kubernetesClusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state kubernetesClusterModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	organizationID := state.OrganizationID.ValueString()
	if organizationID == "" {
		var err error
		organizationID, err = r.organizationID(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Could not read organization identity", err.Error())
			return
		}
	}
	cluster, err := r.client.GetKubernetesCluster(ctx, organizationID, state.ID.ValueString())
	if isKubernetesClusterNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read Kubernetes cluster", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, kubernetesClusterState(cluster, organizationID))...)
}

func (r *kubernetesClusterResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Cluster registration requires replacement", "Changing a cluster name or tenant requires a replacement registration.")
}

func (r *kubernetesClusterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state kubernetesClusterModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteKubernetesCluster(ctx, state.OrganizationID.ValueString(), state.ID.ValueString()); err != nil && !isKubernetesClusterNotFound(err) {
		resp.Diagnostics.AddError("Could not disconnect Kubernetes cluster", err.Error())
	}
}

func (r *kubernetesClusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := uuid.Parse(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid cluster import ID", "Use the cluster registration UUID to import a Kubernetes cluster.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id.String())...)
}

func (r *kubernetesClusterResource) organizationID(ctx context.Context) (string, error) {
	identity, err := r.client.CurrentIdentity(ctx)
	if err != nil {
		return "", err
	}
	if identity.OrganizationID == nil || *identity.OrganizationID == "" || !slices.Contains(identity.PermissionScopes, costgraph.ScopeOrganizationManage) {
		return "", errors.New("Use an organization owner API key explicitly granted organization:manage")
	}
	return *identity.OrganizationID, nil
}

func kubernetesClusterState(cluster *costgraph.KubernetesCluster, organizationID string) kubernetesClusterModel {
	return kubernetesClusterModel{ID: types.StringValue(cluster.ID), Name: types.StringValue(cluster.Name), TenantID: types.StringValue(cluster.TenantID), OrganizationID: types.StringValue(organizationID)}
}

func isKubernetesClusterNotFound(err error) bool {
	var apiError *costgraph.APIError
	return errors.As(err, &apiError) && apiError.StatusCode == 404 && apiError.Code == "not_found"
}
