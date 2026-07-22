# Acceptance test environment

The default test suite is fully in-process and does not require a live account. It uses Terraform protocol v6 with a controlled gRPC server and verifies plan/apply/update/refresh/import/destroy, exact-read drift semantics, deterministic idempotency, SLIs, SLOs, notification templates, maintenance windows, silences, Observe alerts, preview behavior, and fail-closed notify validation.

Live acceptance tests must use a disposable tenant and organization:

```shell
export O11Y_ACC=1
export O11Y_ENDPOINT='https://grpc.o11y.one'
export O11Y_TOKEN='...'
export O11Y_TENANT_ID='...'
export O11Y_ORG_ID='...'
export O11Y_ACC_WEBHOOK_URL='https://your-controlled-webhook.example/terraform-acceptance'
go test ./... -count=1
```

`O11Y_TOKEN` must have alert definition, runtime, preview, destination, notification-policy, maintenance-window, and silence permissions scoped to the supplied tenant and organization. Tests must create unique keys and clean up all resources. Never run live acceptance tests against a production tenant.

The live suite creates, updates, refreshes, imports, previews, and destroys a uniquely keyed destination, policy, maintenance window, silence, and Observe agent-quality alert. The webhook is stored but not probed or used for notify delivery. The suite remains disabled unless `O11Y_ACC=1`; use only a disposable backend scope.
