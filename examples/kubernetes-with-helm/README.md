# Register a cluster and install the operator

Use this example with an existing Kubernetes cluster, namespace, and collection Secret. Terraform reserves the CostGraph registration and Helm installs the operator with the same name. The reference to the registration also orders removal of the Helm release before disconnection.

Set `COSTGRAPH_API_KEY` to an organization owner's user API key explicitly granted `organization:manage`. Set `COSTGRAPH_API_URL` when managing a self-hosted deployment. The deployment must include the Kubernetes registration management API.

The existing Secret must contain a separate tenant-bound collection credential with `operator:read` and `operator:write` under `apiKey`. Its tenant must match `costgraph_tenant_id`. Keep the organization-management key outside the cluster. This example never reads a Secret or puts credential values in Terraform state.

Choose `kube_context` explicitly. Run `terraform init`, then review the plan before applying to your cluster.

```sh
terraform plan \
  -var='costgraph_tenant_id=00000000-0000-0000-0000-000000000001' \
  -var='kube_context=production'
```

An active cluster with this name in the tenant must be imported rather than created again. Import uses the existing cluster UUID.

```sh
terraform import costgraph_kubernetes_cluster.production 00000000-0000-0000-0000-000000000002
```

Destroying the registration retains historical records and blocks further collection. Explicitly creating a new registration for a disconnected name assigns a fresh UUID. Observed versions, health, and inventory remain owned by the operator.
