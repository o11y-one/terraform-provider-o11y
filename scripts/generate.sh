#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
bin_dir="${GOBIN:-$(go env GOPATH)/bin}"

cd "$repo_root"
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.9
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1

PATH="$bin_dir:$PATH" protoc \
  --go_out=internal/gen --go_opt=paths=source_relative \
  --go_opt=Mproto/o11y_one/alerts/v1/alerts.proto=github.com/o11y-one/terraform-provider-o11y/internal/gen/o11y_one/alerts/v1 \
  --go-grpc_out=internal/gen --go-grpc_opt=paths=source_relative \
  --go-grpc_opt=Mproto/o11y_one/alerts/v1/alerts.proto=github.com/o11y-one/terraform-provider-o11y/internal/gen/o11y_one/alerts/v1 \
  proto/o11y_one/alerts/v1/alerts.proto
