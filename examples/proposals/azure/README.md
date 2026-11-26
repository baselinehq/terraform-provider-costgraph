# Connect Azure billing exports

Proposed interface only. `costgraph_azure_integration` is not available in v0.1.0.

The example creates a CostGraph tenant and connects an existing Azure FOCUS
billing export. AzureRM or Entra configuration should create the export, storage
container, service principal, and read permissions separately.

Set `COSTGRAPH_API_KEY` to an organization owner's management key. For a
self-hosted deployment, also set `COSTGRAPH_API_URL` to its API base URL. Azure
credentials authenticate the export reader, not the CostGraph provider.

`azure_tenant_id` identifies the Microsoft Entra directory. `tenant_id` identifies
the CostGraph tenant receiving the data. They are different inputs.

`client_secret_wo` and `secret_version` propose a write-only credential with an
explicit rotation counter. Increment the counter when replacing the credential.
This requires a compatible CLI and provider implementation. Terraform 1.11 or
later supports the proposed language features. OpenTofu compatibility and
absence of the secret from saved plans and state must be verified before release.

The backend currently has a dashboard connection flow. Implementation needs
scoped machine authentication, a typed create/read/update/disconnect contract,
redacted readback, duplicate detection, and shared Go SDK coverage first.
