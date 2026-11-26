# Manage a tag and read existing tags

Proposed interface only. The virtual tag resource and data sources do not exist
in v0.1.0. This configuration cannot validate or apply with that release.

The resource owns the `team` key and every rule under it. The list runs from
first to last. A matching payments resource belongs to `payments`, even if a
later rule would inherit another owner. Remaining AWS costs belong to
`platform`. Costs that match no rule stay untagged for this key.

`value` assigns a literal value. `inherited_from` evaluates a value expression
against the matching record. Each rule supplies exactly one. The example uses
existing CostQL field names, but the proposed Terraform schema is not implemented.

`rule_key` is a proposed stable identifier within the key. The management API
must persist it so editing or reordering a rule preserves that rule's identity.
The dashboard API currently uses rule UUIDs and does not expose this field.

The singular data source reads an existing `environment` key and its ordered
rules without changing it. The plural data source lists key metadata in stable
order. The example assumes `environment` already exists.

Import an existing key before managing it. Creating a resource with the same
key must fail rather than overwrite dashboard configuration. Once imported,
Terraform owns the complete rule list. Review plans after dashboard edits.

See the [design](../../../design/virtual-tags.md) for API requirements, proposed
permissions, and acceptance criteria.
