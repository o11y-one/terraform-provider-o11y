# O11y.one Terraform Provider

Terraform Plugin Framework provider for O11y.one alert destinations, notification policies, shadow alert definitions, and alert previews.

## Provider configuration

```hcl
terraform {
  required_providers {
    o11y = {
      source = "o11y-one/o11y"
    }
  }
}

provider "o11y" {}
```

Configuration may be supplied directly or with environment variables:

| Attribute | Environment | Required | Notes |
| --- | --- | --- | --- |
| `endpoint` | `O11Y_ENDPOINT` | yes | O11y.one gRPC API origin: `https://grpc.o11y.one` |
| `token` | `O11Y_TOKEN` | yes | Sensitive O11y.one platform API token sent as `x-o11y-key`; the provider never logs it |
| `tenant_id` | `O11Y_TENANT_ID` | yes | Tenant UUID sent as `x-o11y-tenant-id` |
| `org_id` | `O11Y_ORG_ID` | yes | Organization UUID sent as `x-o11y-org-id` |
| `insecure_skip_verify` | none | no | Explicitly allows HTTP or disables TLS verification; default `false` |
| `connect_timeout_seconds` | none | no | Default `10` |
| `request_timeout_seconds` | none | no | Default `30` |

Keep the token in the environment or a secret-backed Terraform variable. Although the schema marks it sensitive, Terraform state handling is ultimately controlled by the selected backend.

## Resources and data sources

- `o11y_alert_runbook`: versioned managed runbook CRUD, archive/restore, import, sanitized Markdown, and exact revision state.
- `o11y_alert_destination`: destination CRUD, import, and direct-read drift detection. `secret_refs_json` accepts only opaque values beginning with `secret:`; inline secret-like keys are rejected from `config_json`.
- `o11y_alert_notification_policy`: policy CRUD, import, drift detection, and arbitrary route configuration through `config_json`.
- `o11y_alert_maintenance_window`: maintenance-window CRUD and import.
- `o11y_alert_silence`: silence CRUD and import.
- `o11y_agent_quality_alert`: agent quality regression shadow alert.
- `o11y_cost_per_success_alert`: cost-per-success shadow alert.
- `o11y_slo_burn_alert`: SLO burn shadow alert.
- `o11y_advanced_signal_alert`: advanced signal/symptom shadow alert.
- `o11y_alert_preview`: runs `PreviewAlert` for an existing definition.
- `o11y_alert_runbook`: reads a runbook's current authoritative revision.
- `o11y_alert_runbook_revisions`: lists immutable runbook revision history.
- `o11y_alert_runbook_preview`: validates and renders candidate Markdown without persistence.
- `o11y_alert_runbook_usage`: lists linked definitions, revisions, and incidents.

All mutation idempotency keys are deterministic over tenant, organization, resource type, operation, and stable resource identity. Alert resources require `notify = false`; this provider never calls `ActivateNotifyMode` implicitly.

## Import

Runbooks, destinations, policies, and shadow alerts import by backend ID:

```shell
terraform import o11y_alert_destination.primary 019abc...
terraform import o11y_alert_notification_policy.default 019def...
terraform import o11y_agent_quality_alert.quality 019fed...
terraform import o11y_alert_runbook.checkout 019cab...
```

Destination, policy, and alert imports use exact `Get*` RPCs. Alert readback includes detector kind, recipe configuration, and evaluation interval so imported state is reconstructable.

## Development

```shell
./scripts/generate.sh
./scripts/generate-docs.sh
gofmt -w .
go test ./...
go vet ./...
```

The checked-in protobuf source is copied verbatim from `o11y-api/proto/o11y_one/alerts/v1/alerts.proto`. Generator versions are pinned in `scripts/generate.sh`; Go modules are pinned in `go.mod` and `go.sum`.

See [docs/backend-api-gaps.md](docs/backend-api-gaps.md) for lifecycle limitations and [docs/acceptance-testing.md](docs/acceptance-testing.md) for the live API test contract.

## Releases

Signed, cross-platform GitHub releases are produced from semantic version tags
and indexed by both Terraform Registry and OpenTofu Registry. See
[docs/releasing.md](docs/releasing.md) for one-time registry onboarding, signing
key setup, and the repeatable release procedure.
