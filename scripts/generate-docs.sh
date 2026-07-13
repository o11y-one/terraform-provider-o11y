#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$repo_root"
go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.25.0 generate \
  --provider-name o11y \
  --rendered-provider-name "O11y.one"
