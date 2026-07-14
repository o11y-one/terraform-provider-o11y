# o11y_alert_runbook

Creates a versioned managed alert runbook. O11y.one sanitizes Markdown and rejects raw HTML, executable content, unsafe links, and credential-bearing URLs. Alert definitions and incidents retain the exact runbook revision they linked, so later edits cannot rewrite historical response instructions.

```hcl
resource "o11y_alert_runbook" "checkout" {
  runbook_key    = "checkout-agent-recovery"
  title          = "Checkout agent recovery"
  owner_team_id  = var.checkout_team_id
  change_reason  = "Initial production response procedure"
  provenance_ref = "modules/checkout-alerts"

  markdown = <<-EOT
    # Checkout agent recovery

    1. Inspect the linked failing traces and eval evidence.
    2. Check provider fallback and tool timeout rates.
    3. Roll back the current prompt release when the regression is release-bound.
  EOT
}
```

Exactly one of `owner_user_id` or `owner_team_id` is required. Set `archived = true` to archive and `archived = false` to restore. Content cannot be edited while archived. Deleting the Terraform resource archives it; immutable revisions and historical incident links remain.

Import with the runbook UUID:

```shell
terraform import o11y_alert_runbook.checkout 019f7aa2-6c7f-7000-8000-000000000001
```

## Attributes

- `runbook_key` (required): Stable immutable organization-scoped key.
- `title` (required): Operator-facing title.
- `owner_user_id` / `owner_team_id` (one required): Accountable owner.
- `markdown` (required): Sanitized runbook source, up to the backend limit.
- `change_reason` (required): Audited reason for the desired content or lifecycle change.
- `provenance_ref` (optional): Repository, module, or control-plane reference.
- `archived` (optional/computed): Archive/restore lifecycle state.
- `current_revision_id`, `revision`, `content_hash`, `rendered_html`, `created_at`, `updated_at`, `id` (computed): Authoritative backend state.
