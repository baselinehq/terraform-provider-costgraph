# Publish to Terraform and OpenTofu

Both registries can index the same signed GitHub release from
`baselinehq/terraform-provider-costgraph`. Keep one source repository, version,
set of binaries, checksum file, and signature. Registry publication is separate
from CLI compatibility testing.

The provider is listed on the Terraform Registry. The namespace and public
signing key are registered there. OpenTofu Registry registration remains pending.
The release workflow creates a draft, so a successful build does not publish the
next version automatically.

## Verify compatibility

Use the pinned Terraform 1.15.3 and OpenTofu 1.13.10 CLIs for verification. Run
the credentialless OpenTofu plugin check.

```sh
make check-opentofu
```

This builds the normal provider and loads its schema with OpenTofu using a
project-local development override for
`registry.opentofu.org/baselinehq/costgraph`. It also checks example formatting.
It does not contact CostGraph or download a published provider. The CI job pins
OpenTofu and its setup action.

Use the same isolated API configuration described in `README.md` to run the
core lifecycles with OpenTofu.

```sh
make testacc-tofu
```

Set `TOFU=/absolute/path/to/tofu` to select a task-local binary. The target points
the existing acceptance runner at that binary and the OpenTofu Registry address.
It selects the tenant, AI gateway, FOCUS, Kubernetes registration, and virtual-tag
scenarios, including their data sources. The virtual-tag test requires the billed
fixture described in [README.md](README.md#run-acceptance-tests).

The protected manual acceptance workflow also accepts a CLI choice. Select
`opentofu` and the relevant suite when real AWS, Google Cloud, and Azure
fixtures are available. AWS, Google Cloud, and Azure positive cloud acceptance remains pending
for both CLIs until isolated accounts are available. Read
[CLOUD_ACCEPTANCE.md](CLOUD_ACCEPTANCE.md) before running those scenarios.

## Prepare one signed release

Merge and deploy the required backend APIs. Merge the SDK and pin its public
version before merging the provider. Retain the MIT license in both repositories.
Run the real core lifecycles with both CLIs. Record any deferred cloud scenarios
in the release notes without describing them as passed.

The protected `release` environment supplies `GPG_PRIVATE_KEY` and
`GPG_PASSPHRASE`. Keep the private key there and register only its public half with
the registries. Confirm that the signing fingerprint matches the registered key.

Push a reviewed semantic version tag such as `v0.2.0`. The pinned GoReleaser
workflow builds platform archives, a protocol 6 manifest, checksums, and a detached
GPG signature. Verify those assets before publishing the draft GitHub release.
Do not attach a separate public-key file to the provider release. The Terraform
Registry treats unexpected assets as candidate provider packages.

Verify checksums for all platform ZIPs and the manifest, then verify the detached
signature. Load the verified binaries with both CLIs and inspect their schemas.
Publish the verified draft and check a clean Registry installation. Never replace
artifacts under an existing version tag.

No separate OpenTofu build or copy of the repository is required.

## Confirm Terraform Registry publication

The initial provider registration is complete. For later versions, confirm that
the release webhook indexed the published GitHub release and that the expected
version installs with its signing key verified.

If a new namespace or replacement registration is needed, follow the initial
registration steps.

Sign in to the Terraform Registry with a GitHub account authorized for the
BaselineHQ repository. Register the public signing key for the namespace, then
select Publish and Provider to register the repository. The initial registration
creates a GitHub release webhook for subsequent published versions.

Follow [HashiCorp's provider publishing instructions](https://developer.hashicorp.com/terraform/registry/providers/publishing).

## Register with OpenTofu

Submit the provider and signing key through OpenTofu's GitHub issue forms.
The Registry requires the form UI. Do not substitute a pull request, a manually
composed issue, the GitHub API, or `gh issue create`.

Use these values in the provider form.

| Field | Value |
| --- | --- |
| Provider Repository | `baselinehq/terraform-provider-costgraph` |

Use these values in the signing-key form.

| Field | Value |
| --- | --- |
| Provider Namespace | `baselinehq` |
| Provider Name | `costgraph` |
| Provider GPG Key | The ASCII-armored public key used to sign the release. |

The signing-key form requires the submitter's organization membership to be
public. If that is unsuitable, contact the OpenTofu team using the alternative
process described in the form. Do not change membership visibility automatically.

Open the [provider form](https://github.com/opentofu/registry/issues/new?template=provider.yml)
and the [signing-key form](https://github.com/opentofu/registry/issues/new?template=provider_key.yml).
OpenTofu reviews the submissions. After inclusion, its Registry indexes published
versions from the same GitHub repository. Do not overwrite released artifacts.
See the [Registry submission rules](https://github.com/opentofu/registry) and
[inclusion policy](https://github.com/opentofu/registry/blob/main/POLICY.md).

## Configure either CLI

Keep the portable source address in configurations intended for either CLI.

```hcl
terraform {
  required_providers {
    costgraph = {
      source = "baselinehq/costgraph"
    }
  }
}
```

Terraform resolves that shorthand through its default Registry. OpenTofu resolves
it through `registry.opentofu.org`. These are separate full provider addresses.
A working listing on one Registry does not establish publication on the other.

An OpenTofu Registry listing is an index of the GitHub release artifacts.
A filesystem, network, or OCI provider mirror is a separate installation option
for private or disconnected environments. No custom mirror service is needed
for this public distribution plan.
