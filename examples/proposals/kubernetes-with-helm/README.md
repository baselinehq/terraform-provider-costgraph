# Register a cluster and install the operator

Proposed interface only. `costgraph_kubernetes_cluster` is not available in
v0.1.0. Helm provider 3.3.0 syntax and operator chart 0.8.27 values were checked
against their source. This combined configuration has not been applied.

The proposed CostGraph resource reserves a registration for an existing cluster.
It does not create an AKS, EKS, or GKE cluster. Helm installs the collector into
that cluster and supplies the registered name.

Before applying a future supported version, prepare an existing CostGraph
tenant, the target Kubernetes namespace, and a Secret in that namespace with an
`apiKey` entry containing that tenant's collector credential. Provision the
Secret through your secret manager. Do not give the collector the provider's
organization-management key. Creating a new tenant needs a separate credential
provisioning step before this example can install its collector.

The CostGraph provider reads `COSTGRAPH_API_KEY` and, for self-hosted deployments,
`COSTGRAPH_API_URL`. Also set `collector_api_url` for self-hosted installations.
It is the API address reachable from the cluster and may differ from the address
used by the Terraform runner.

The reference from Helm values to the registration creates a dependency, so
Terraform removes the Helm release before destroying the registration. The
management API must define disconnect behavior, retained history, and protection
against an external collector recreating a deleted registration.

The existing operator registration endpoint reports observed versions and
collection state. Terraform must not call it with invented version information
or claim the collector is healthy. A separate management lifecycle and shared Go
SDK support are prerequisites.
