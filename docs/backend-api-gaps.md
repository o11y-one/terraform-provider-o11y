# Backend API gaps

This provider is intentionally bounded by `o11y_one.alerts.v1` and does not infer lifecycle operations that the API does not expose.

## Alert definitions

`AlertDefinitionService` exposes recipe-specific create RPCs, exact get/list, Observe update, candidate-revision workflows, and revision-fenced archive.

- Terraform destroy calls `ArchiveDefinitionV2` with the expected revision.
- `UpdateObserve` cannot update slug, severity, scope, or detector configuration, so these attributes are replacement-triggering.
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

Policy JSON follows the exact typed protobuf JSON contract. Route targets live under `target`, matchers are repeated typed clauses, route behavior is an enum, notification budgets are top-level fields, and `requested_notification_template_revision_id` pins an exact published template revision. `bound_notification_template_revision_id` remains backend output authority and must not be configured.

## Notification templates

Terraform manages user templates only. `published = true` publishes the current revision explicitly; publishing a newer revision never rebinds an existing policy. A policy can select `notification_template_id` alone to bind the then-current published revision, or add `requested_notification_template_revision_id` to pin an exact published revision. System-default templates remain backend-owned.

## Secret references

The destination contract contains separate secret-management RPCs that accept plaintext. This provider does not expose them because Terraform would persist plaintext in configuration/state. It manages only opaque `secret:` references already provisioned through an external secret workflow.
