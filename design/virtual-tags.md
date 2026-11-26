# Manage virtual tags with Terraform

This branch adds virtual-tag management for the next provider release. It is not
part of the published v0.1.0 provider. Deploy the accompanying configuration APIs
before using the resource or data sources.

## Own one key and its ordered rules

One `costgraph_virtual_tag` resource manages a key and its complete rule list in
a tenant. The first active matching rule wins within its scope. Reorder the list
to change precedence. An explicit `rules = []` removes every rule while keeping
the key. Unmatched costs stay untagged for that key.

Each rule has a stable `rule_key`. Editing its expression or moving it within the
list preserves its server UUID. Existing dashboard rules expose their UUID as a
stable reference during import. Import and refresh do not write that reference
back to CostGraph.

Set exactly one of `value` for a literal value or `inherited_from` for a CostQL
value expression. Conditions use CostQL `expression` with the `billing` or `k8s`
scope. Set `active = false` to pause a rule without deleting it.

The [resource example](../examples/resources/costgraph_virtual_tag/) shows the
schema. AI generation, time-windowed rules, and percentage allocation are outside
this resource's scope. A key containing time-windowed rules cannot be imported
or read through the exact configuration API. CostGraph evaluates virtual tags
at query time against existing cost records, so no backfill setting is needed.

## Read existing keys

`data.costgraph_virtual_tag` requires a tenant UUID and an exact key. It returns
metadata, the complete ordered rules, server UUIDs, and revision tokens. A
missing key returns an error.

`data.costgraph_virtual_tags` lists metadata and rule counts in stable key and
UUID order. The SDK follows every metadata page. Use the singular lookup when
you need the rules. Neither data source changes CostGraph.

## Use tenant-bound permissions

Lookup requires `virtual-tags:read`. Management requires `virtual-tags:write`,
which includes read access. The API key must belong to the selected tenant.
`organization:manage` does not grant these scopes or cross-tenant attribution
access. Use a separate provider alias with a target-tenant key when the same
configuration also creates tenants or cloud connections.

Installation tokens cannot use these management operations. Machine writes use
the same rule compiler, cost-cache invalidation, and audit context as the public
API. Ordinary data-query permissions remain unchanged.

## Import and review dashboard edits

Import an existing key with `<tenant_uuid>/<key_uuid>`. Creation refuses a
matching existing key instead of adopting it through an upsert. After import,
Terraform owns every rule under the key. Dashboard edits and accepted AI changes
appear as drift at refresh.

The aggregate `revision` covers metadata and rules. Updates and deletes require
the revision read during refresh. A concurrent edit returns a conflict without
writing anything. Refresh and review a new plan instead of retrying with a newer
revision automatically. `rules_revision` remains a separate read-only server
counter for rule changes.

Destroy removes the key and rules but keeps source cost records. Removing a key
changes attribution for historical queries too. Keys used by billing exports
cannot be destroyed until those export references are removed.

## Reference and verification

Vantage's [virtual tag resource][vantage-resource] and
[plural data source][vantage-data] informed aggregate ownership and ordered
precedence. CostGraph also provides an exact lookup. Vantage's backfill,
override, preferred, and business-metric settings are not CostGraph attributes.

The acceptance scenario exercises the real resource and data sources with
Terraform or OpenTofu against an isolated backend. It checks stored rules and
actual cost attribution, including reorder, pause/resume, import, explicit
clearing, conflicts, and tenant boundaries. Cost queries may return a stale
aggregate while their background refresh completes, so attribution assertions
wait for a bounded refresh. A passing format or schema check alone does not
prove this lifecycle.

[vantage-resource]: https://github.com/vantage-sh/terraform-provider-vantage/blob/25dc27f1d3bd937cd7beba3c2d5982a394e4ee0c/docs/resources/virtual_tag_config.md
[vantage-data]: https://github.com/vantage-sh/terraform-provider-vantage/blob/25dc27f1d3bd937cd7beba3c2d5982a394e4ee0c/docs/data-sources/virtual_tag_configs.md
