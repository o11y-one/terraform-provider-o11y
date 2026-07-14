# o11y_alert_runbook_usage

Reads durable definition, exact definition-revision, and incident links for a managed runbook.

```hcl
data "o11y_alert_runbook_usage" "checkout" {
  runbook_id = o11y_alert_runbook.checkout.id
}
```

Use this data source before archive or ownership changes to understand the affected alert and incident history.
