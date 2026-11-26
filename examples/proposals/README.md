# Proposed resources

These examples describe interfaces under discussion. Azure, Kubernetes cluster
registration, and virtual tags are not implemented in v0.1.0. Their names and
attributes can change before implementation.

Do not run these configurations with the published provider. Formatting checks
parse the HCL, but do not prove that these resources validate or apply. These
directories are separate from the runnable examples used in Registry docs.

- [Connect Azure billing exports](azure/)
- [Register a Kubernetes cluster and install the operator](kubernetes-with-helm/)
- [Manage virtual tags and read existing tags](virtual-tags/)

The [virtual tag design](../../design/virtual-tags.md) records ownership, API
requirements, and the implementation sequence.
