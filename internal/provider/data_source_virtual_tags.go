package provider

import (
	"context"
	costgraph "github.com/baselinehq/costgraph-go"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"slices"
	"strings"
)

type virtualTagsDataSource struct{ client *costgraph.Client }
type virtualTagsModel struct {
	ID       types.String `tfsdk:"id"`
	TenantID types.String `tfsdk:"tenant_id"`
	Keys     types.List   `tfsdk:"keys"`
}
type virtualTagMetadataModel struct {
	ID            types.String `tfsdk:"id"`
	Key           types.String `tfsdk:"key"`
	Description   types.String `tfsdk:"description"`
	Revision      types.String `tfsdk:"revision"`
	RulesRevision types.Int64  `tfsdk:"rules_revision"`
	RuleCount     types.Int64  `tfsdk:"rule_count"`
}

func NewVirtualTagsDataSource() datasource.DataSource { return &virtualTagsDataSource{} }
func (d *virtualTagsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_virtual_tags"
}
func (d *virtualTagsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "List virtual tag key metadata in one authorized tenant. Requires virtual-tags:read. Reads all pages and sorts keys by key then UUID. This does not retrieve or modify rules. Use costgraph_virtual_tag for one complete configuration.", Attributes: map[string]schema.Attribute{
		"id":        schema.StringAttribute{Computed: true, MarkdownDescription: "Stable tenant-bound collection identifier."},
		"tenant_id": schema.StringAttribute{Required: true, MarkdownDescription: "Tenant UUID authorized by the API key.", Validators: []validator.String{virtualTagTenantValidator}},
		"keys": schema.ListNestedAttribute{Computed: true, MarkdownDescription: "Key metadata sorted by key then UUID.", NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Computed: true, MarkdownDescription: "Key UUID."},
			"key":            schema.StringAttribute{Computed: true, MarkdownDescription: "Virtual tag key."},
			"description":    schema.StringAttribute{Computed: true, MarkdownDescription: "Key description."},
			"revision":       schema.StringAttribute{Computed: true, MarkdownDescription: "Aggregate metadata and rules revision."},
			"rules_revision": schema.Int64Attribute{Computed: true, MarkdownDescription: "Server rules revision."},
			"rule_count":     schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of rules, including paused rules."},
		}}},
	}}
}
func (d *virtualTagsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *virtualTagsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config virtualTagsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	items, err := d.client.ListVirtualTags(ctx, config.TenantID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not list virtual tags", err.Error())
		return
	}
	slices.SortFunc(items, func(a, b costgraph.VirtualTagMetadata) int {
		if v := strings.Compare(a.Key, b.Key); v != 0 {
			return v
		}
		return strings.Compare(a.ID, b.ID)
	})
	models := make([]virtualTagMetadataModel, len(items))
	for i, item := range items {
		models[i] = virtualTagMetadataModel{ID: types.StringValue(item.ID), Key: types.StringValue(item.Key), Description: types.StringValue(item.Description), Revision: types.StringValue(item.Revision), RulesRevision: types.Int64Value(item.RulesRevision), RuleCount: types.Int64Value(int64(item.RuleCount))}
	}
	object := types.ObjectType{AttrTypes: map[string]attr.Type{"id": types.StringType, "key": types.StringType, "description": types.StringType, "revision": types.StringType, "rules_revision": types.Int64Type, "rule_count": types.Int64Type}}
	list, diags := types.ListValueFrom(ctx, object, models)
	resp.Diagnostics.Append(diags...)
	config.Keys = list
	config.ID = types.StringValue(config.TenantID.ValueString() + "/virtual-tags")
	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}
