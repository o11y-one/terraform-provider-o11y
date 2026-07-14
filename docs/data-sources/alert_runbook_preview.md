# o11y_alert_runbook_preview

Validates and safely renders Markdown without persisting a runbook.

```hcl
data "o11y_alert_runbook_preview" "candidate" {
  markdown = file("${path.module}/checkout-runbook.md")
}
```

Unsafe Markdown fails the plan/read. Successful reads return `normalized_markdown` and server-sanitized `rendered_html`.
