# o11y_alert_runbook

Reads the current authoritative revision of a managed runbook.

```hcl
data "o11y_alert_runbook" "checkout" {
  runbook_id = o11y_alert_runbook.checkout.id
}
```

The data source returns identity, owner, current revision ID, revision number, Markdown, sanitized HTML, content hash, provenance, archive state, and the current change reason.
