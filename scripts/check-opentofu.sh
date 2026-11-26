#!/bin/sh
set -eu

provider_dir=$(CDPATH='' cd -- bin && pwd)
check_dir=$(mktemp -d)
trap 'rm -rf "$check_dir"' EXIT HUP INT TERM

cat > "$check_dir/tofurc" <<EOF_CONFIG
provider_installation {
  dev_overrides {
    "registry.opentofu.org/baselinehq/costgraph" = "$provider_dir"
  }
  direct {}
}
EOF_CONFIG

cat > "$check_dir/main.tf" <<'EOF_CONFIG'
terraform {
  required_providers {
    costgraph = {
      source = "baselinehq/costgraph"
    }
  }
}
EOF_CONFIG

"${TOFU:-tofu}" fmt -check -recursive examples
TF_CLI_CONFIG_FILE="$check_dir/tofurc" "${TOFU:-tofu}" -chdir="$check_dir" providers schema -json > "$check_dir/schema.json"
echo 'OpenTofu loaded the CostGraph provider schema using registry.opentofu.org/baselinehq/costgraph.'
