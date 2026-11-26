# Terraform provider for CostGraph

Manage CostGraph tenants, provider connections, Kubernetes registrations, and
virtual tags with Terraform. Use the same provider with hosted or self-hosted
CostGraph. API access comes from the shared
[CostGraph Go SDK](https://github.com/baselinehq/costgraph-go).

Provider [v0.1.0 is published on the Terraform Registry](https://registry.terraform.io/providers/baselinehq/costgraph/0.1.0).
This branch prepares the next release with eight resources and seven data sources.
Azure, Kubernetes registration, and virtual-tag management require the
corresponding backend APIs before you can apply their configurations.

## Manage CostGraph configuration

The resources cover these tasks.

| Resource | Outcome |
| --- | --- |
| `costgraph_tenant` | Create, rename, or deactivate a tenant within your organization. |
| `costgraph_aws_integration` | Connect an existing AWS FOCUS export through an IAM role. |
| `costgraph_google_cloud_integration` | Connect a Google Cloud billing export through service account impersonation. |
| `costgraph_azure_integration` | Connect an Azure FOCUS export and rotate its service principal secret without storing it in Terraform state. |
| `costgraph_ai_gateway_integration` | Connect a gateway and manage model mappings, sub-account names, and collection settings. |
| `costgraph_focus_push_integration` | Register a billing account that receives pushed FOCUS records. |
| `costgraph_kubernetes_cluster` | Reserve or import a cluster registration before installing the operator. |
| `costgraph_virtual_tag` | Manage a tag key and its complete ordered literal or inherited rules. |

Data sources read your organization, a tenant, an individual integration, your
key's tenant integration list, and the billing-provider catalog. The two virtual
tag data sources look up a key with its rules or list the tenant's key metadata.
GitHub onboarding, member management, and API-key management remain outside the
provider's scope.

## Configure authentication

Set `COSTGRAPH_API_KEY` to a key from the deployment you want to manage. The
hosted API URL defaults to `https://api.costgraph.ai`. For a self-hosted deployment,
set `COSTGRAPH_API_URL` or the provider's `api_url` attribute. Omit `/api/v1` and
keep any reverse-proxy prefix. Explicit attributes take precedence over the
environment. Certificate verification remains enabled.

Tenant administration, AWS, Google Cloud, and Azure connections, and Kubernetes
registrations require an organization owner's user API key explicitly granted
`organization:manage`. Existing keys do not gain this permission automatically.
Ordinary cost and telemetry queries remain bound to the key's tenant.
Installation tokens cannot manage these resources.

Virtual tags use a key bound to their target tenant. Lookup requires
`virtual-tags:read`. Management requires `virtual-tags:write`, which includes
read access. `organization:manage` does not grant these permissions or access to
another tenant's tags. If one configuration manages organization resources and
tags in another tenant, configure a separate provider alias with that tenant's key.

Reference a managed tenant from its child integrations so Terraform disconnects
them before deactivating the tenant. Keep the organization-management key in a
separate bootstrap tenant. CostGraph refuses to delete the default tenant or the
tenant that owns the active key. If a tenant is deactivated outside Terraform,
resolve its access before changing the remaining connections.

## Use the examples

Read the [provider configuration guide](docs/index.md) and
[resource guides](docs/resources) for configuration, imports, and deletion effects.
The provider connects existing cloud resources. IAM roles, service principals,
permissions, and billing exports must exist first.

Use the [Azure example](examples/resources/costgraph_azure_integration/) for
write-only secret input and rotation. It requires Terraform 1.11 or later or
OpenTofu 1.11 or later.

The [Kubernetes and Helm example](examples/kubernetes-with-helm/) pairs a cluster
registration with the operator chart. Prepare the cluster, namespace, and an
existing Secret containing a separate tenant-bound collector credential first.
The registration does not create the Kubernetes cluster or issue credentials.

The [virtual tag example](examples/resources/costgraph_virtual_tag/) assigns
literal or inherited values in a defined order. Terraform owns the whole key.
The first active matching rule wins. Import an existing key before managing it.
The [ownership design](design/virtual-tags.md) explains revision conflicts and
supported rule types.

## Develop locally

Use Go 1.26 or later. The pinned verification CLIs are Terraform 1.15.3 and
OpenTofu 1.13.10. Read [AGENTS.md](AGENTS.md) before contributing.

Run the build and checks.

```sh
make build lint check-generated
```

The binary is written to `bin/terraform-provider-costgraph`. Use a project-local
CLI development override for `baselinehq/costgraph` when testing it.

## Run acceptance tests

Use an isolated CostGraph deployment. The scenarios create, update, import, and
delete real objects and check them through the public API. Missing configuration
fails clearly. The tests do not bypass cloud access checks.

Configure the core suite with these values.

| Variable | Required value |
| --- | --- |
| `COSTGRAPH_TEST_API_URL` | API URL of the isolated deployment. |
| `COSTGRAPH_TEST_API_KEY` | Tenant-bound key with `focus:read`, `focus:write`, `integrations:read`, `operator:read`, `operator:write`, and `virtual-tags:write`. |
| `COSTGRAPH_TEST_TENANT_ID` | Tenant associated with that key. |
| `COSTGRAPH_TEST_MANAGEMENT_API_KEY` | Organization owner's key explicitly granted `organization:manage`. |
| `COSTGRAPH_TEST_MODEL_KEY` | Canonical model key in the deployment's model catalog. |
| `COSTGRAPH_TEST_VIRTUAL_TAG_RESOURCE_ID` | Resource ID of the controlled AWS billing fixture described next. |

The virtual-tag scenario needs a current-month billed AWS record named
`payments-virtual-tag-test` with an `owner=platform` tag. Its assertions check
actual cost attribution, including rule reordering and pausing. Prepare this
synthetic record only in the isolated test deployment. Set
`COSTGRAPH_TEST_OTHER_TENANT_ID` to exercise the additional tenant-boundary check.

Run the core scenarios.

```sh
make testacc-core
```

This suite selects tenant, AI gateway, FOCUS push, Kubernetes registration, and
virtual-tag lifecycles. It exercises persisted values, empty follow-up plans,
imports, data sources, updates or replacement, and confirmed destruction. Use
`make testacc-kubernetes` or `make testacc-virtual-tags` to select either addition.

AWS, Google Cloud, and Azure positive cloud lifecycles are prepared but have not
run because isolated accounts are not available. Follow
[cloud acceptance](CLOUD_ACCEPTANCE.md) before selecting their individual targets.
`make testacc` selects every scenario and requires every fixture.

PR checks run without credentials. The protected manual acceptance workflow lets
you select the CLI and suite. Run it only against reviewed code and an isolated
deployment.

## Use OpenTofu

Run `make check-opentofu` to load the provider schema with OpenTofu. Use
`make testacc-tofu` with the core acceptance configuration to run the same real
lifecycles. Set `TOFU=/absolute/path/to/tofu` to select an installed binary.

OpenTofu Registry registration remains pending. Its shorthand provider address
resolves through a different registry from Terraform, so a Terraform listing
does not establish OpenTofu installation availability. Both registries can index
the same signed release. Read [RELEASING.md](RELEASING.md) for registration and
publication steps.

## Prepare a release

GoReleaser 2.20.3 packages protocol 6 binaries, platform ZIPs, the Registry
manifest, checksums, and a detached GPG signature. Validate the configuration.

```sh
make release-check
```

The tag workflow creates a draft GitHub release using signing credentials in the
protected `release` environment. Deploy the required backend APIs, pin the public
SDK version, complete the available lifecycle checks, and record deferred cloud
checks before publishing. Verify the signature and artifact checksums, then
confirm the published version in each registry separately.

## License

This project uses the [MIT License](LICENSE).
