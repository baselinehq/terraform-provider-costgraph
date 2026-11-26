# Verify cloud connections

Run these scenarios against isolated AWS and Google Cloud billing exports.
They exercise the Terraform CLI and the real CostGraph API. They are prepared
and have not been run because dedicated cloud accounts are not available yet.
Compilation and core acceptance do not establish cloud connection correctness.

## Prepare CostGraph

Deploy the backend management APIs and configure its normal AWS trust identity
and Google Cloud federation or impersonation setup. Cloud verification runs in
that backend. A developer's local cloud profile does not supply the backend's
credentials. Keep those credentials out of this provider's HCL and environment.

Set the shared test configuration.

| Variable | Required value |
| --- | --- |
| `COSTGRAPH_TEST_API_URL` | API URL of the isolated CostGraph deployment. |
| `COSTGRAPH_TEST_MANAGEMENT_API_KEY` | User API key owned by an active organization owner and explicitly granted `organization:manage`. |
| `COSTGRAPH_TEST_TENANT_ID` | Dedicated active tenant in that organization. |

Use a billing account without an existing CostGraph connection in that tenant.
The tests disconnect their connections, including any data collected through
them. Use no customer data or production account connections. Required variables
are checked before Terraform makes cloud changes. Missing configuration fails
rather than skipping the scenario.

## Prepare the AWS export

Create a dedicated FOCUS billing export in S3 and an IAM role that the CostGraph
backend can assume. Grant the role access to list the bucket and read its export
objects. Configure the trust principal and organization external ID from the
CostGraph setup flow before starting the test. The resource's `external_id`
output becomes available after creation and cannot supply this prerequisite.

Set these values.

| Variable | Required value |
| --- | --- |
| `COSTGRAPH_TEST_AWS_BILLING_ACCOUNT_ID` | Billing account associated with the export. |
| `COSTGRAPH_TEST_AWS_REGION` | Region of the export bucket. |
| `COSTGRAPH_TEST_AWS_BUCKET` | Dedicated export bucket name. |
| `COSTGRAPH_TEST_AWS_PREFIX` | Nonempty export prefix used for the first apply. |
| `COSTGRAPH_TEST_AWS_ROLE_ARN` | IAM role ARN with the required trust and export access. |

The scenario clears the prefix to verify that update behavior. Its dedicated
bucket must also permit listing and reading from the root. The test explicitly
disables cost allocation tag activation and recommendation collection.

Run the AWS lifecycle.

```sh
make testacc-aws
```

The test covers create, direct API readback, an empty follow-up plan, rename
without changing the UUID, prefix clearing, import, external deletion and
recreation, and API-confirmed destruction.

## Prepare the Google Cloud export

Configure a dedicated BigQuery billing export and a service account that the
CostGraph backend can impersonate. Grant the target the required query and
billing-export read permissions. Configure the backend's impersonation or
federation trust before running the test. The provider accepts no service-account
key JSON.

Set these values.

| Variable | Required value |
| --- | --- |
| `COSTGRAPH_TEST_GCP_BILLING_ACCOUNT_ID` | Billing account associated with the export. |
| `COSTGRAPH_TEST_GCP_QUERY_PROJECT` | Project where CostGraph queries the export. |
| `COSTGRAPH_TEST_GCP_TARGET_PRINCIPAL` | Service account principal that CostGraph impersonates. |

Run the Google Cloud lifecycle.

```sh
make testacc-gcp
```

The test covers create, direct API readback, an empty follow-up plan, rename
without changing the UUID, import, external deletion and recreation, and
API-confirmed destruction. It disables recommendation collection.

## Run through GitHub Actions

Configure the protected `acceptance` environment with the matching variables and
a secret `COSTGRAPH_TEST_MANAGEMENT_API_KEY`. Select `aws` or `gcp` in the manual
API lifecycle acceptance workflow. Run it only against reviewed, trusted code.
Select `all` only when both cloud fixtures and the core test configuration are
available. A green credentialless PR check does not replace these scenarios.
