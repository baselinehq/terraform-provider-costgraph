# Connect an Azure billing export

This resource connects an existing FOCUS export so CostGraph can collect its billing data. It does not create Azure storage, exports, applications, or permissions.

Use Terraform 1.11 or later or OpenTofu 1.11 or later. Configure CostGraph with an organization owner's user API key that explicitly includes `organization:manage`. The deployment must support the Azure management endpoints.

Before applying, create a FOCUS export in Azure Cost Management and a service principal with Storage Blob Data Reader access to its storage account. The export must contain data that CostGraph can read. `azure_tenant_id` identifies the Microsoft Entra directory. `tenant_id` identifies the CostGraph tenant receiving the costs. `storage_account` identifies this connection and cannot change in place.

Supply the secret through the ephemeral `azure_client_secret` variable, such as your CI secret store's `TF_VAR_azure_client_secret` environment variable. CostGraph verifies access before saving the connection. The provider reads this value from configuration and never stores it in a plan or state. CostGraph stores its copy encrypted so it can continue collecting data.

## Rotate the service principal secret

Create a replacement secret for the same Azure application. Supply it through `TF_VAR_azure_client_secret`, increase `secret_version`, and apply. Changing the secret input without increasing the counter does not rotate the stored credential. Changing `azure_tenant_id` or `client_id` requires a new secret and a higher counter too.

After a successful apply, the secret input can be omitted while its version stays unchanged. Refresh, import, and ordinary settings updates do not need it. They read only the nonsecret configuration and rotation counter. Keep the configured counter aligned with the imported value. Directly owned legacy Azure connections import with version `0` until rotated.

## Update or disconnect the export

Changing the container, export folder, display name, or recommendation setting updates the same connection UUID. Omitting the optional export folder preserves its stored value. Set `export_name = ""` to read the whole container. That change requires access to the container's exports.

Creating another connection for the same CostGraph tenant and storage account returns a conflict. Import the existing connection instead. Connections that reuse another connection's credentials or use a different billing-account identity cannot be managed by this resource.

Destroy disconnects collection and removes the connection's data from CostGraph cost views. It does not delete Azure resources or revoke service principal secrets. Remove an unused secret in Azure after confirming no other application relies on it.
