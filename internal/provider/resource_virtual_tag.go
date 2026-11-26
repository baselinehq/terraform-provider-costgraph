package provider

import (
	"context"
	"errors"
	"regexp"
	"strings"

	costgraph "github.com/baselinehq/costgraph-go"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.ResourceWithConfigure = &virtualTagResource{}
var _ resource.ResourceWithImportState = &virtualTagResource{}
var _ resource.ResourceWithModifyPlan = &virtualTagResource{}
var _ resource.ResourceWithValidateConfig = &virtualTagResource{}

type virtualTagResource struct{ client *costgraph.Client }
type virtualTagModel struct {
	ID            types.String `tfsdk:"id"`
	TenantID      types.String `tfsdk:"tenant_id"`
	Key           types.String `tfsdk:"key"`
	Description   types.String `tfsdk:"description"`
	Revision      types.String `tfsdk:"revision"`
	RulesRevision types.Int64  `tfsdk:"rules_revision"`
	Rules         types.List   `tfsdk:"rules"`
}
type virtualTagRuleModel struct {
	ID            types.String `tfsdk:"id"`
	RuleKey       types.String `tfsdk:"rule_key"`
	Scope         types.String `tfsdk:"scope"`
	Expression    types.String `tfsdk:"expression"`
	Value         types.String `tfsdk:"value"`
	InheritedFrom types.String `tfsdk:"inherited_from"`
	Active        types.Bool   `tfsdk:"active"`
}

var virtualTagRuleType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"id": types.StringType, "rule_key": types.StringType, "scope": types.StringType, "expression": types.StringType, "value": types.StringType, "inherited_from": types.StringType, "active": types.BoolType,
}}
var virtualTagNameValidator = stringvalidator.RegexMatches(regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@+=-]{0,127}$`), "Use 1 to 128 key-name characters, beginning with a letter or digit.")
var virtualTagTenantValidator = stringvalidator.RegexMatches(regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`), "Use a lowercase tenant UUID.")

func NewVirtualTagResource() resource.Resource { return &virtualTagResource{} }
func (r *virtualTagResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_virtual_tag"
}
func (r *virtualTagResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Assign a virtual tag with ordered literal or inherited rules. The first active matching rule wins. This resource owns the key and its complete rules list. Requires explicit tenant-bound virtual-tags:write permission. Updates are atomic and stop if metadata or rules changed after refresh. Destroy removes the key and rules, retains source cost records, and refuses keys used by billing exports. Time-windowed and percentage rules are outside this resource's scope.", Attributes: map[string]schema.Attribute{
		"id":             schema.StringAttribute{Computed: true, MarkdownDescription: "Virtual tag key UUID.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"tenant_id":      schema.StringAttribute{Required: true, MarkdownDescription: "Tenant UUID authorized by the API key. Changing it replaces the key. organization:manage does not grant virtual-tag access.", Validators: []validator.String{virtualTagTenantValidator}, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"key":            schema.StringAttribute{Required: true, MarkdownDescription: "Virtual tag key. Changing it replaces the key. Existing keys must be imported rather than adopted during create.", Validators: []validator.String{virtualTagNameValidator}, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"description":    schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), MarkdownDescription: "Key description. An empty value clears it."},
		"revision":       schema.StringAttribute{Computed: true, MarkdownDescription: "Opaque revision of metadata and rules used to prevent overwriting concurrent edits."},
		"rules_revision": schema.Int64Attribute{Computed: true, MarkdownDescription: "Server rules revision."},
		"rules": schema.ListNestedAttribute{Required: true, MarkdownDescription: "Complete ordered rules list, at most 1000 entries. An empty list removes all rules and keeps the key. Reorder entries to change precedence. rule_key preserves each server rule UUID across edits and reorder.", NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Computed: true, MarkdownDescription: "Stable server rule UUID."},
			"rule_key":       schema.StringAttribute{Required: true, MarkdownDescription: "Unique stable reference within this key. Keep it unchanged when editing or moving a rule. Imported legacy rules use their existing UUID as the reference.", Validators: []validator.String{virtualTagNameValidator}},
			"scope":          schema.StringAttribute{Required: true, MarkdownDescription: "CostQL scope, billing or k8s.", Validators: []validator.String{stringvalidator.OneOf("billing", "k8s")}},
			"expression":     schema.StringAttribute{Required: true, MarkdownDescription: "CostQL condition evaluated against a billing record or Kubernetes resource.", Validators: []validator.String{trimmedStringValidator{}}},
			"value":          schema.StringAttribute{Optional: true, MarkdownDescription: "Literal tag value. Set exactly one of value or inherited_from."},
			"inherited_from": schema.StringAttribute{Optional: true, MarkdownDescription: "CostQL expression providing the inherited tag value. Set exactly one of value or inherited_from.", Validators: []validator.String{trimmedStringValidator{}}},
			"active":         schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: "Whether the rule participates in matching. Set false to pause it without changing its identity."},
		}}},
	}}
}
func (r *virtualTagResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *virtualTagResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var rules types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("rules"), &rules)...)
	if rules.IsUnknown() || rules.IsNull() || resp.Diagnostics.HasError() {
		return
	}
	var members []virtualTagRuleModel
	resp.Diagnostics.Append(rules.ElementsAs(ctx, &members, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(members) > 1000 {
		resp.Diagnostics.AddAttributeError(path.Root("rules"), "Too many rules", "A virtual tag supports at most 1000 rules.")
	}
	seen := map[string]bool{}
	for i, m := range members {
		p := path.Root("rules").AtListIndex(i)
		if !m.RuleKey.IsUnknown() {
			key := m.RuleKey.ValueString()
			if seen[key] {
				resp.Diagnostics.AddAttributeError(p.AtName("rule_key"), "Duplicate rule reference", "Each rule_key must be unique within the key.")
			}
			seen[key] = true
		}
		if !m.Value.IsUnknown() && !m.InheritedFrom.IsUnknown() && m.Value.IsNull() == m.InheritedFrom.IsNull() {
			resp.Diagnostics.AddAttributeError(p, "Choose one tag value", "Set exactly one of value or inherited_from.")
		}
	}
}
func (r *virtualTagResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() {
		return
	}
	var plan, state virtualTagModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || plan.Rules.IsUnknown() {
		return
	}
	var planned, previous []virtualTagRuleModel
	resp.Diagnostics.Append(plan.Rules.ElementsAs(ctx, &planned, false)...)
	resp.Diagnostics.Append(state.Rules.ElementsAs(ctx, &previous, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ids := map[string]types.String{}
	for _, m := range previous {
		ids[m.RuleKey.ValueString()] = m.ID
	}
	for i, m := range planned {
		if m.ID.IsUnknown() && !m.RuleKey.IsUnknown() {
			if id, ok := ids[m.RuleKey.ValueString()]; ok {
				planned[i].ID = id
			}
		}
	}
	list, d := types.ListValueFrom(ctx, virtualTagRuleType, planned)
	resp.Diagnostics.Append(d...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("rules"), list)...)
}
func virtualTagState(ctx context.Context, value *costgraph.VirtualTag) (virtualTagModel, diag.Diagnostics) {
	members := make([]virtualTagRuleModel, len(value.Rules))
	for i, m := range value.Rules {
		members[i] = virtualTagRuleModel{ID: types.StringValue(m.ID), RuleKey: types.StringValue(m.RuleKey), Scope: types.StringValue(m.Scope), Expression: types.StringValue(m.Expression), Value: types.StringPointerValue(m.Value), InheritedFrom: types.StringPointerValue(m.InheritedFrom), Active: types.BoolValue(m.Active)}
	}
	list, d := types.ListValueFrom(ctx, virtualTagRuleType, members)
	return virtualTagModel{ID: types.StringValue(value.ID), TenantID: types.StringValue(value.TenantID), Key: types.StringValue(value.Key), Description: types.StringValue(value.Description), Revision: types.StringValue(value.Revision), RulesRevision: types.Int64Value(value.RulesRevision), Rules: list}, d
}
func virtualTagRequest(ctx context.Context, plan virtualTagModel) (costgraph.VirtualTagRequest, diag.Diagnostics) {
	var members []virtualTagRuleModel
	d := plan.Rules.ElementsAs(ctx, &members, false)
	rules := make([]costgraph.VirtualTagRuleInput, len(members))
	for i, m := range members {
		rules[i] = costgraph.VirtualTagRuleInput{RuleKey: m.RuleKey.ValueString(), Scope: m.Scope.ValueString(), Expression: m.Expression.ValueString(), Value: m.Value.ValueStringPointer(), InheritedFrom: m.InheritedFrom.ValueStringPointer(), Active: m.Active.ValueBoolPointer()}
	}
	return costgraph.VirtualTagRequest{Key: plan.Key.ValueString(), Description: plan.Description.ValueString(), Rules: rules}, d
}
func virtualTagNotFound(err error) bool {
	var apiErr *costgraph.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == 404 && apiErr.Code == "not_found"
}
func virtualTagDiagnostic(err error) string {
	var apiErr *costgraph.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case "virtual_tag_exists":
			return err.Error() + ". Import the existing key rather than creating a duplicate."
		case "virtual_tag_revision_conflict":
			return err.Error() + ". Metadata or rules changed after refresh. Refresh and create a new plan."
		case "virtual_tag_exported":
			return err.Error() + ". Remove the key from billing exports before deleting it."
		}
	}
	return err.Error()
}
func (r *virtualTagResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan virtualTagModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body, d := virtualTagRequest(ctx, plan)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.client.CreateVirtualTag(ctx, plan.TenantID.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Could not create virtual tag", virtualTagDiagnostic(err))
		return
	}
	state, d := virtualTagState(ctx, saved)
	resp.Diagnostics.Append(d...)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
func (r *virtualTagResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state virtualTagModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.client.GetVirtualTag(ctx, state.TenantID.ValueString(), state.ID.ValueString())
	if virtualTagNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read virtual tag", err.Error())
		return
	}
	state, d := virtualTagState(ctx, saved)
	resp.Diagnostics.Append(d...)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
func (r *virtualTagResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state virtualTagModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body, d := virtualTagRequest(ctx, plan)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	body.ExpectedRevision = state.Revision.ValueStringPointer()
	saved, err := r.client.UpdateVirtualTag(ctx, state.TenantID.ValueString(), state.ID.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Could not update virtual tag", virtualTagDiagnostic(err))
		return
	}
	state, d = virtualTagState(ctx, saved)
	resp.Diagnostics.Append(d...)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
func (r *virtualTagResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state virtualTagModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteVirtualTag(ctx, state.TenantID.ValueString(), state.ID.ValueString(), state.Revision.ValueString())
	if err != nil && !virtualTagNotFound(err) {
		resp.Diagnostics.AddError("Could not delete virtual tag", virtualTagDiagnostic(err))
	}
}
func (r *virtualTagResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Use <tenant_uuid>/<key_uuid>.")
		return
	}
	for _, part := range parts {
		if _, err := uuid.Parse(part); err != nil {
			resp.Diagnostics.AddError("Invalid import ID", "Use <tenant_uuid>/<key_uuid>, with two UUIDs.")
			return
		}
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("tenant_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
