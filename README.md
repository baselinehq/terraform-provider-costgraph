# Terraform provider for CostGraph

Manage CostGraph integrations and account configuration with Terraform.

This project is in discovery. No provider binary, resources, data sources, SDK,
CI workflows, or releases exist yet. The first release will target approximately
five managed resources and five data sources.

The initial scope is CostGraph tenants and provider accounts. The working
resource shortlist covers tenants, AWS, Google Cloud, AI gateways, and FOCUS push
connections. GitHub is deferred. Exact schemas and data sources remain open.

Read [AGENTS.md](AGENTS.md) before contributing. Implementation starts after the
selected public API lifecycles and an importable Go management client are ready.

The provider will use Terraform Plugin Framework. Existing SDKv2 providers are
research references, rather than the basis for new code.
