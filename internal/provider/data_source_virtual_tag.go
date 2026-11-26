package provider

import (
	"context"
	costgraph "github.com/baselinehq/costgraph-go"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

type virtualTagDataSource struct{ client *costgraph.Client }

func NewVirtualTagDataSource() datasource.DataSource { return &virtualTagDataSource{} }
func (d *virtualTagDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_virtual_tag"
}
func virtualTagReadRules() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{Computed: true, MarkdownDescription: "Ordered rules, including stable references and server UUIDs. The first active matching rule wins.", NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
		"id":             schema.StringAttribute{Computed: true, MarkdownDescription: "Server rule UUID."},
		"rule_key":       schema.StringAttribute{Computed: true, MarkdownDescription: "Stable reference within this key. Legacy rules expose their existing UUID."},
		"scope":          schema.StringAttribute{Computed: true, MarkdownDescription: "CostQL scope, billing or k8s."},
		"expression":     schema.StringAttribute{Computed: true, MarkdownDescription: "CostQL matching condition."},
		"value":          schema.StringAttribute{Computed: true, MarkdownDescription: "Literal value, or null for inherited rules."},
		"inherited_from": schema.StringAttribute{Computed: true, MarkdownDescription: "Inherited CostQL value expression, or null for literal rules."},
		"active":         schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the rule participates in matching."},
	}}}
}
func (d *virtualTagDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Look up one virtual tag by exact key in an authorized tenant. Requires virtual-tags:read. Returns the complete ordered configuration without changing CostGraph. Missing keys return an error. Time-windowed rules cannot be represented by this data source.", Attributes: map[string]schema.Attribute{
		"id":             schema.StringAttribute{Computed: true, MarkdownDescription: "Virtual tag key UUID."},
		"tenant_id":      schema.StringAttribute{Required: true, MarkdownDescription: "Tenant UUID authorized by the API key.", Validators: []validator.String{virtualTagTenantValidator}},
		"key":            schema.StringAttribute{Required: true, MarkdownDescription: "Exact virtual tag key.", Validators: []validator.String{virtualTagNameValidator}},
		"description":    schema.StringAttribute{Computed: true, MarkdownDescription: "Key description."},
		"revision":       schema.StringAttribute{Computed: true, MarkdownDescription: "Aggregate metadata and rules revision."},
		"rules_revision": schema.Int64Attribute{Computed: true, MarkdownDescription: "Server rules revision."},
		"rules":          virtualTagReadRules(),
	}}
}
func (d *virtualTagDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *virtualTagDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config virtualTagModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	value, err := d.client.LookupVirtualTag(ctx, config.TenantID.ValueString(), config.Key.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not look up virtual tag", err.Error())
		return
	}
	state, diags := virtualTagState(ctx, value)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
