# Backend API gaps

This provider is intentionally bounded by `o11y_one.alerts.v1` and does not infer lifecycle operations that the API does not expose.

## Alert definitions

`AlertDefinitionService` exposes create RPCs, exact get/list, shadow update, and soft delete.

- Terraform destroy calls `DeleteDefinition`; replacement-triggering changes can complete without orphaning the prior definition.
- `UpdateShadow` cannot update slug, severity, scope, or recipe/condition configuration, so these attributes are replacement-triggering.
- `GetDefinition` returns detector kind, recipe-specific configuration, and evaluation interval for stable import and drift detection.

## Notify activation

Notify activation is a separate `ActivateNotifyMode` RPC requiring `confirmation` and `audit_reason`. Terraform configuration cannot safely infer either value.

- Every alert resource requires `notify = false` and rejects `true` during configuration validation.
- The provider never calls `ActivateNotifyMode`, including during create, update, resume, or import.
- Notify activation must remain an explicit audited workflow outside this provider until the backend defines a safe declarative activation contract.

## Destinations and notification policies

The API exposes exact get and delete RPCs for destinations and notification policies.

- Read/import/drift use exact ID lookups.
- Missing IDs remove the resource from Terraform state.

## Secret references

The destination contract contains separate secret-management RPCs that accept plaintext. This provider does not expose them because Terraform would persist plaintext in configuration/state. It manages only opaque `secret:` references already provisioned through an external secret workflow.
