# o11y_alert_runbook_revisions

Lists immutable runbook revisions newest first. `revisions_json` is canonical JSON suitable for `jsondecode` and includes exact revision IDs, content hashes, sanitized content, owner, actor, reason, and timestamp.

```hcl
data "o11y_alert_runbook_revisions" "checkout" {
  runbook_id = o11y_alert_runbook.checkout.id
  limit      = 25
}

locals {
  checkout_runbook_history = jsondecode(data.o11y_alert_runbook_revisions.checkout.revisions_json)
}
```
