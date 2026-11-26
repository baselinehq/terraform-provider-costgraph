# Terraform provider for CostGraph

Manage CostGraph configuration with Terraform using an API key and the API URL
of your hosted or self-hosted deployment.

This development version includes one managed resource and five data sources.
It has not been published on the Terraform Registry. Connection readback requires
the backend's `GET /api/v1/integrations/connections/{id}` endpoint.

| Type | Outcome |
| --- | --- |
| `costgraph_focus_push_integration` resource | Register a provider billing account for FOCUS ingestion. Rename, import, or disconnect the connection. |
| `costgraph_current_organization` data source | Read the organization associated with your API key. |
| `costgraph_tenant` data source | Read a tenant available to your API key. |
| `costgraph_integration` data source | Look up an existing connection by UUID. |
| `costgraph_integrations` data source | List your tenant's connections. |
| `costgraph_integration_providers` data source | Read the supported billing-provider catalog. |

Managed tenants, AWS, Google Cloud, and AI gateway connections remain in the
initial release scope. They need additional API contracts before implementation.
GitHub is deferred.

## Configure the provider

Set `COSTGRAPH_API_KEY` to a key from the deployment you want to manage. The
hosted API URL defaults to `https://api.costgraph.ai`. For self-hosted deployments,
set `COSTGRAPH_API_URL` or the provider's `api_url` attribute. Omit `/api/v1` and
keep any reverse-proxy prefix. Explicit attributes take precedence over the
environment. Certificate verification remains enabled.

Read the [provider configuration guide](docs/index.md) for examples and the
[FOCUS push resource guide](docs/resources/focus_push_integration.md) for required
permissions, import, replacement behavior, and disconnection effects.

## Develop locally

Use Go 1.26 or later and Terraform. Acceptance was verified with Terraform 1.15.3.
The provider uses Plugin Framework and the shared [CostGraph Go SDK](https://github.com/baselinehq/costgraph-go).

Run the local build and checks.

```sh
make build lint check-generated
```

The binary is written to `bin/terraform-provider-costgraph`. Use a project-local
Terraform CLI development override for `baselinehq/costgraph` when testing the
binary. Read [AGENTS.md](AGENTS.md) before contributing.

## Run the API lifecycle test

Use an isolated CostGraph deployment and tenant. The test creates, renames,
replaces, and deletes connection records. Configure `COSTGRAPH_TEST_API_URL`,
`COSTGRAPH_TEST_API_KEY`, and `COSTGRAPH_TEST_TENANT_ID` for that test environment.

Run the test against the real API.

```sh
make testacc
```

The scenario verifies persisted values, clean plans, import, all five data
sources, recreation after external deletion, replacement, and API-confirmed
destruction. Missing test configuration fails clearly. PR checks run without
credentials. Configure the manual workflow's `acceptance` environment with the
same variables and a secret API key before running it.

## Prepare a release

GoReleaser 2.20.3 packages protocol 6 binaries, platform ZIPs, the Registry
manifest, checksums, and a detached GPG signature. Validate the configuration.

```sh
make release-check
```

The tag workflow creates a draft GitHub release. Configure `GPG_PRIVATE_KEY` and
`GPG_PASSPHRASE` in the `release` environment before using it. Registry
namespace registration, signing-key ownership, license selection, and publishing
the first version remain release prerequisites. No release tag has been created.
