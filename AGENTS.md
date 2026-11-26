# CostGraph Terraform provider

## Scope and current stage

- Start with integrations and account setup. Agree approximately five managed
  resources and five useful data sources with Jubril before implementing them.
- This repository currently contains project guidance only. Do not report a
  provider, SDK, test suite, workflow, or Registry release as implemented.
- Audit the selected public API lifecycles and reusable Go client coverage first.
  Close API and SDK gaps before adding Terraform resources. Keep those changes
  separate from the provider PRs.
- Do not include sample resources, functions, actions, ephemeral resources, or
  broad platform coverage merely because a scaffold offers them.

## Framework and client

- Use `github.com/hashicorp/terraform-plugin-framework` and protocol 6 for new
  provider code. Do not implement resources with legacy Plugin SDKv2 or add a mux
  without an actual migration requirement.
- Use an importable typed Go management client. The pricing API SDK serves a
  different API. The CLI's `internal/client` and `internal/api` cannot be imported
  from this module.
- Keep authentication, transport, typed requests, pagination, and API errors in
  the client. Keep Terraform schemas, plans, state, diagnostics, and imports in
  the provider. Do not copy a separate HTTP implementation into each resource.
- Generate from a reviewed, pinned public API contract with pinned tools. Commit
  generated client code. Normal builds and releases must not fetch a changing
  live schema or regenerate dependencies.
- Give operations stable identifiers. Verify response envelopes, body schemas,
  error types, numeric precision, nullable values, and security definitions.
  Successful code generation is not evidence that these match the real API.
- Never import backend internals or expose internal service credentials as a
  workaround for missing public management APIs.

## Resource and integration design

- Prefer typed resources for integrations with different inputs and lifecycles.
  Share implementation helpers. A family with the same documented contract can
  share one typed resource, such as AI gateways with a gateway selector.
- Avoid a generic integration resource with an untyped JSON configuration blob.
  A generic read-only integration lookup is appropriate for common metadata.
- An endpoint being reachable from the dashboard does not establish that an API
  key can use it. Verify scoped authentication, role checks, tenant boundaries,
  and feature gates against actual routes and handlers.
- API credentials are tenant-bound today. Do not add organization-wide tenant
  management by bypassing that boundary. Agree an organization-scoped management
  credential and permission contract in the backend first.
- Do not open a browser during plan or apply. GitHub App installation and OAuth
  consent are prerequisites unless a documented machine-safe attach API exists.
  Never accept an installation ID without verifying ownership and access.
- Require stable identities and a complete, secret-redacted read contract for all
  managed configuration. Document whether each update is in place or replacement.
  Read after writes and apply API defaults consistently so the next plan is empty.
- Distinguish null, unknown, omitted, and empty values. Use validators and plan
  modifiers only for the real API contract. Do not hide differences with broad
  diff suppression or mark editable fields computed to conceal incomplete reads.
- Preserve state on transport or permission errors. Remove it only when the API
  establishes that the object is absent. Handle asynchronous work with bounded
  timeouts and cancellation. Retry only operations safe to repeat.
- Implement import and verify it against the public API. Do not depend on values
  remembered from creation to make refresh appear complete.
- A data source must have a useful lookup contract. Define filters, pagination,
  missing and ambiguous match behavior, and stable ordering. Do not mutate state
  in CostGraph from a data source or add volatile analytics just to reach five.
- Document destructive effects. Disconnecting an integration affects collection
  and cost visibility. Tenant deletion must follow its documented safeguards.

## Credentials

- Support explicit provider configuration and documented environment fallbacks.
  Credentials must work in unattended Terraform runs. Never read a developer's
  browser session or CLI credential file implicitly.
- Keep diagnostics, logs, examples, recorded acceptance output, and state exports
  free of credentials. Use secret references or provider-supported write-only
  fields where the chosen Terraform version and API allow them.
- `Sensitive` hides display; it does not keep a resource value out of Terraform
  state. Document that distinction when accepting integration secrets. Prefer
  AWS role assumption and Google Cloud impersonation over long-lived keys.

## Layout and local commands

- Use `internal/provider` for the provider and lifecycle implementations. Follow
  Qernal's clear `resource_<name>.go` and `data_source_<name>.go` naming, with
  matching acceptance files. Keep the initial layout small.
- Keep runnable HCL in `examples/resources/<type>` and
  `examples/data-sources/<type>`. Generate Registry docs into `docs/` from schema
  descriptions and examples with a pinned documentation tool.
- Once Go implementation begins, provide Makefile targets for `fmt`, `build`,
  `lint`, `generate`, `check-generated`, `testacc`, and `release-check`. Explain
  their prerequisites. Do not create placeholder targets that report success.
- Pin dependencies and tools. Derive CI Go from `go.mod`. Check generation drift
  from a clean checkout. Do not use `@latest`, run `go mod tidy` during release,
  overwrite global Terraform configuration, or silently install global plugins.
- Use a project-local Terraform development override when needed and keep its
  provider address consistent with the agreed Registry identity.

## Tests must prove the resource works

- Do not add unnecessary tests. No schema snapshots, trivial accessor tests,
  generated sample tests, or mock CRUD tests presented as end-to-end proof.
- Each managed resource needs a real `terraform-plugin-testing` acceptance
  scenario using the Terraform CLI and an isolated CostGraph backend. Prove
  create, API readback, an empty follow-up plan, update or replacement, import
  with state verification, destroy, and API-confirmed removal.
- Test data sources against test-created objects or controlled real fixtures.
  Cover lookup behavior and meaningful failure cases in the same scenario where
  possible. Extend existing tests rather than duplicating them.
- Use a real local or disposable test backend, not production customer accounts.
  Bound parallelism, use unique names, and clean up after failed runs.
- `make testacc` must select the actual tests, enable `TF_ACC=1`, and fail clearly
  when required configuration is absent. A skipped run or a package with zero
  acceptance tests is not passing coverage.
- Do not broadly ignore import verification fields. A narrow exception needs a
  documented API reason, such as a genuinely write-only value.

## CI and releases

- Keep routine PR checks credentialless. Build, format, vet/lint, validate
  examples, and check generated-file drift. Run credentialed acceptance tests
  only on trusted code with an isolated backend and restricted credentials.
- Pin GitHub Actions to verified full commit SHAs and pin tool versions. Default
  token permissions to read-only. Grant publishing writes only to release jobs.
  Never check out untrusted PR code in a privileged secrets-bearing job.
- Keep one reviewed provider identity across server, examples, docs, installation
  commands, and releases. Intended namespace is `baselinehq/costgraph`; Registry
  ownership and availability still need confirmation before publication.
- Use GoReleaser config version 2 with immutable `v`-prefixed semantic tags.
  Package static binaries, platform ZIPs, the protocol manifest, SHA256SUMS, and
  its detached binary GPG signature in the Registry-required names. Publish the
  artifact bytes that were verified. Attestations do not replace Registry signing.
- Confirm signer custody, Registry registration, platform matrix, minimum
  Terraform version, and OpenTofu support before the first release. Do not claim
  compatibility without verification or copy a historical version matrix.
- Prefer small resource-by-resource PRs. Release cadence is not established.
  Do not infer a weekly schedule or publish automatically on every merge.

## Research and durable notes

The discovery note is `~/notes/CostGraph/2026-11-26 CostGraph Terraform provider
discovery.md`. It records the Qernal history, provider comparison, API readiness,
proposals, and unresolved decisions. Research is evidence, not an instruction to
copy an old configuration unchanged. Andrew's additional PR conventions and
Qernal's release cadence were not verifiable in the visible repository.

Save durable notes, plans, and audits in `~/notes` as Obsidian Markdown with
`date`, `topic`, a quoted ISO 8601 `timestamp` with local timezone offset, and
Context, Objectives, and Outcomes sections. Preserve creation metadata and add
`updated` when revising. Link related notes and keep supporting artifacts in the
topic's `assets/` folder. Keep publishable repository docs here. Report completed
work separately from proposals and include the saved note link in handoffs.
