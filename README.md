# Terraform provider for CostGraph

Create tenants and connect provider accounts to hosted or self-hosted CostGraph
with Terraform. The provider uses Plugin Framework and the shared
[CostGraph Go SDK](https://github.com/baselinehq/costgraph-go).

This development version contains five resources and five data sources. The first release is v0.1.0. Registry registration is separate from the GitHub
release. Deploy [the backend management APIs](https://github.com/baselinehq/backend/pull/1446) before using these resources.

| Resource | Outcome |
| --- | --- |
| `costgraph_tenant` | Create, rename, or deactivate a tenant within your organization. |
| `costgraph_aws_integration` | Connect an existing AWS FOCUS export through an IAM role. |
| `costgraph_google_cloud_integration` | Connect a Google Cloud billing export through service account impersonation. |
| `costgraph_ai_gateway_integration` | Connect a gateway and manage model mappings, sub-account names, and collection settings. |
| `costgraph_focus_push_integration` | Register a billing account that receives pushed FOCUS records. |

The five data sources read your current organization, a tenant, an individual
integration, your API key's tenant integration list, and the billing-provider
catalog. GitHub onboarding, member management, and API-key management are outside
this release.

## Configure the provider

Set `COSTGRAPH_API_KEY` to a key from the deployment you want to manage. The
hosted API URL defaults to `https://api.costgraph.ai`. For a self-hosted deployment,
set `COSTGRAPH_API_URL` or the provider's `api_url` attribute. Omit `/api/v1` and
keep any reverse-proxy prefix. Explicit attributes take precedence over the
environment. Certificate verification remains enabled.

Tenant administration and AWS or Google Cloud connections require an organization
owner's user API key explicitly granted `organization:manage`. Existing keys do
not gain this permission automatically. The permission covers supported
management operations within that organization. Ordinary cost and telemetry
queries remain bound to the key's tenant. Installation tokens cannot manage
tenants or provider accounts.

Reference `costgraph_tenant.production.id` from child integrations so Terraform
disconnects them before deactivating their tenant. Keep the management key in a
separate bootstrap tenant. CostGraph refuses to delete that key's tenant or the
default tenant. If someone deactivates a tenant outside Terraform before its
connections are removed, their refresh can fail with a permission error. Resolve
that access issue before removing the remaining managed connections.

Read the [provider configuration guide](docs/index.md) and the resource guides in
[docs/resources](docs/resources) for examples, import, update behavior, and
requirements. Cloud IAM roles, impersonation permissions, and billing exports
must exist before connecting their accounts. This provider configures CostGraph
connections rather than creating those cloud resources.

## Develop locally

Use Go 1.26 or later. The verified CLIs are Terraform 1.15.3 and OpenTofu
1.13.10. OpenTofu uses the same provider binary and resource schemas.
Read [AGENTS.md](AGENTS.md) before contributing.

Run the build and checks.

```sh
make build lint check-generated
```

The binary is written to `bin/terraform-provider-costgraph`. Use a project-local
Terraform CLI development override for `baselinehq/costgraph` when testing it.

## Run acceptance tests

Use an isolated CostGraph deployment. The tests create, update, import, and delete
real resources through Terraform and confirm the results through the public API.
Missing configuration fails clearly. Tests do not bypass cloud access checks.

Configure these values for the core tenant, gateway, and FOCUS lifecycles.

| Variable | Required value |
| --- | --- |
| `COSTGRAPH_TEST_API_URL` | API URL of the isolated deployment. |
| `COSTGRAPH_TEST_API_KEY` | Ordinary tenant-bound key for the FOCUS scenario. |
| `COSTGRAPH_TEST_TENANT_ID` | Tenant associated with that ordinary key. |
| `COSTGRAPH_TEST_MANAGEMENT_API_KEY` | Owner key explicitly granted `organization:manage`. |
| `COSTGRAPH_TEST_MODEL_KEY` | Canonical model key present in the deployment's model catalog. |

Run the core scenarios.

```sh
make testacc-core
```

These cover persisted values, empty follow-up plans, import, all five data
sources, in-place updates, removed gateway overrides and mappings, tenant and
connection recreation after external deletion, replacement, and confirmed
destruction. They passed against an isolated local backend. The earlier FOCUS
scenario also passed through a reverse-proxy URL prefix.

The AWS and Google Cloud lifecycle tests require separate isolated cloud billing
exports and correctly configured access. Their prerequisites are in
[the cloud acceptance guide](CLOUD_ACCEPTANCE.md). Those positive cloud scenarios
are prepared and have not been run. Use their individual Makefile targets when
the accounts are available. `make testacc` runs every acceptance scenario.

PR checks run without credentials. The manual acceptance workflow separates core,
AWS, Google Cloud, and all scenarios. Configure its protected `acceptance`
environment before running it against trusted code.

## Use OpenTofu

Run `make check-opentofu` to load the provider schema with OpenTofu. Use
`make testacc-tofu` with the core acceptance environment to run the same real
resource lifecycles. Set `TOFU` to an installed OpenTofu binary when needed.

Both registries can use the same signed GitHub release. Their initial
registrations remain pending. Read [RELEASING.md](RELEASING.md) for the separate
Registry submissions, signing requirements, and compatibility checks.

## Prepare a release

GoReleaser 2.20.3 packages protocol 6 binaries, platform ZIPs, the Registry
manifest, checksums, and a detached GPG signature. Validate the configuration.

```sh
make release-check
```

The tag workflow creates a draft GitHub release. Configure `GPG_PRIVATE_KEY` and
`GPG_PASSPHRASE` in the `release` environment before using it. Registry namespace
registration, signing-key ownership, cloud acceptance, and
publishing the first version remain release prerequisites. The first provider release uses the v0.1.0 tag.

## License

This project uses the [MIT License](LICENSE).
