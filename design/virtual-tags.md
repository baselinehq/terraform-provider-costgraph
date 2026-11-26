# Manage virtual tags with Terraform

Status is proposed. Jubril agreed to start with literal and inherited values,
ordered rules, pause controls, import, and read-only discovery. No virtual tag
resource or data source is implemented in v0.1.0.

## Assign ownership without changing cloud resources

One `costgraph_virtual_tag` resource owns one key and its complete ordered rule
list within a tenant. This lets a team version its cost attribution without
changing the underlying infrastructure. The first matching active rule wins
within its scope. Costs without a matching rule remain untagged for that key.

Do not introduce independently managed rule resources in the first version.
They would compete over precedence and could delete each other's rules through
the backend's full-list save operation. Shared-cost allocation is a separate
feature and is outside this resource's initial scope.

## Reference implementation

Vantage's [virtual tag resource][vantage-resource] manages a key with an ordered
list of values. Its [plural data source][vantage-data] lists configurations.
Import uses the configuration token, and reads retrieve the configuration
through its API. Verified against commit
`25dc27f1d3bd937cd7beba3c2d5982a394e4ee0c` on 2026-11-26.

Carry forward aggregate ownership, explicit precedence, import, and readback.
CostGraph should also provide an exact singular lookup so a consumer need not
download every rule to reference one key. Do not copy Vantage's backfill,
overridable, preferred, percentage-allocation, or business-metric options without
a corresponding CostGraph contract. CostGraph evaluates saved virtual tag rules
against cost data at query time, including historical data.

## Proposed resource schema

The [HCL proposal](../examples/proposals/virtual-tags/) shows the complete shape.

| Attribute | Behavior |
| --- | --- |
| `tenant_id` | Required. Changing tenants requires replacement. |
| `key` | Required, unique within the tenant. Changing it requires replacement in the first version. |
| `description` | Optional explanation of the attribution rule set. No AI generation during apply. |
| `rules` | Required ordered list. An explicit empty list clears all rules while retaining the key. |
| `id` | Computed stable key UUID. |
| `rules_revision` | Computed concurrency token returned by the backend, never a user-supplied setting. |

Each rule has a required `rule_key`, `scope`, and `expression`. `rule_key` is a
proposed stable identifier, unique within the key and persisted by the management
API. Exactly one of `value` and `inherited_from` sets a literal or inherited
value. Optional `active` defaults to true. Support the backend's `billing` and
`k8s` scopes. Include a computed server rule UUID in readback.

Changing list position must change precedence without recreating the rule.
Changing an expression or value must retain the ID identified by `rule_key`.
Never associate IDs by list position. Existing rules imported from the dashboard
need a stable server-assigned reference without a write from import or refresh.

Initially accept CostQL expressions as the single predicate representation.
The API also accepts a clause or expression tree. Import must expose the same
compiled expression used by the evaluator, without depending on an unrecoverable
creation-only representation. Rule date windows can follow after their boundary
and timezone behavior is documented and tested.

## Read existing tags

`data.costgraph_virtual_tag` requires `tenant_id` and an exact `key`. Return its
UUID, description, ordered rules, and revision. A missing key is an error. If ID
lookup is added later, require exactly one of key or ID.

`data.costgraph_virtual_tags` requires `tenant_id` and returns key metadata and
rule counts, sorted by key then UUID. Handle API pagination without omission.
Keep the full rules in the singular lookup so a directory query stays small.
Neither data source generates, saves, repairs, or backfills anything.

## Close the management API gaps

The existing backend already provides key metadata, individual rule edits, and
an atomic full-list save. That save requires `expected_rules_revision`, rejects
stale writes with HTTP 409, validates every rule before writing, deletes omitted
rules, and returns the resulting key and ordered rules from the transaction.

The full rule and write endpoints currently sit behind member-or-higher user
authentication. A separate shared key-list route permits `focus:read`; that does
not provide a complete API-key management lifecycle.

Before extending the SDK, add a reviewed management contract covering these
requirements.

1. Define explicit opt-in tenant-scoped `virtual-tags:read` and
   `virtual-tags:write` permissions. Writes imply the reads needed for refresh.
   These names and the grant UI are proposed. Installation tokens cannot use
   them. Ordinary keys stay tenant-bound. `organization:manage` must not silently
   acquire cost-attribution powers. If cross-tenant management is needed, require
   an explicit permission combination and verify same-organization ownership.
2. Create metadata and rules in one transaction. An existing key returns a
   machine-readable conflict. Do not use metadata upsert to adopt a key silently.
3. Read one complete configuration by tenant and stable key UUID in a consistent
   snapshot. Also support exact key lookup and paginated metadata listing. Return
   all managed fields, rule references, ordered IDs, and concurrency tokens.
4. Update metadata and ordered rules atomically. Reuse rule compilation and
   revision checks. Persist `rule_key` and define a stable fallback for existing
   rules. Metadata needs its own concurrency protection or an aggregate version
   because `rules_revision` deliberately excludes description changes.
5. Delete with a concurrency precondition so a write after the latest refresh
   cannot be discarded silently. Removing the key removes all its rules and
   changes attribution, including historical query results. Retain cost records.
6. Preserve cost-cache invalidation, tenant checks, role restrictions, license
   gates, and audit attribution for machine writes. Reuse domain services rather
   than bypassing them through Terraform-specific database calls.
7. Publish stable operation IDs and typed success and error responses in the
   reviewed management OpenAPI contract, then regenerate and extend the Go SDK.

## Import and dashboard edits

Propose import identity `<tenant_uuid>/<key_uuid>`. Import must perform a full
read, recover every rule and its stable reference, and allow a subsequent empty
plan when the configuration matches. The existing UUID remains the server's
object identity even though Terraform initially treats key renames as replacement.

Terraform owns the whole key once imported. Dashboard or accepted AI edits appear
as drift on the next refresh. Plans must show additions, removals, and ordering
changes. Do not hide them with broad diff suppression. A revision conflict stops
apply and asks for refresh and a new plan. Never retry by fetching a newer
revision and silently replacing someone else's work.

The API's AI regeneration produces proposals for human review. It does not belong
in a data source or automatic plan/apply step. Exported tag rules and downstream
consumers need a defined deletion policy before a destroy is released.

## Ship small changes

1. Backend management contract and scoped authentication, including stable rule
   references and atomic read/create/update/delete semantics.
2. Pinned SDK contract and typed methods, with structured not-found, permission,
   validation, duplicate, and revision-conflict errors.
3. Resource, exact data source, and plural data source with examples and generated
   Registry documentation.

Use real Terraform and OpenTofu acceptance against an isolated backend to prove
create, readback, empty plan, edit, reorder, pause/resume, remove a rule, explicitly
clear all rules, import, and destroy. Exercise both literal and inherited values.
The same lifecycle should read both data sources and confirm actual attribution
and first-match precedence through the public cost API using controlled records.
Check duplicate refusal, tenant boundaries, read-only keys, stale update/delete,
and dashboard drift without silently overwriting another writer. Verify configured
self-hosted API prefixes as well as hosted-style URLs. Reuse existing scenarios;
do not add mock CRUD tests as a substitute for this proof.

[vantage-resource]: https://github.com/vantage-sh/terraform-provider-vantage/blob/25dc27f1d3bd937cd7beba3c2d5982a394e4ee0c/docs/resources/virtual_tag_config.md
[vantage-data]: https://github.com/vantage-sh/terraform-provider-vantage/blob/25dc27f1d3bd937cd7beba3c2d5982a394e4ee0c/docs/data-sources/virtual_tag_configs.md
