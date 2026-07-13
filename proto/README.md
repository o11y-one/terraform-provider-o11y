# Protobuf contract

`o11y_one/alerts/v1/alerts.proto` is copied verbatim from the current `o11y-api` alert contract. Regenerate checked-in Go bindings with `./scripts/generate.sh`.

Provider calls attach these API authentication headers as gRPC metadata:

- `authorization: Bearer <token>`
- `x-o11y-tenant-id: <tenant_id>`
- `x-o11y-org-id: <org_id>` when configured

The generator and runtime modules are version-pinned in `scripts/generate.sh` and `go.mod`.
