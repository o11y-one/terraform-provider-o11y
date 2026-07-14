resource "o11y_alert_runbook" "checkout" {
  runbook_key    = "checkout-agent-recovery"
  title          = "Checkout agent recovery"
  owner_team_id  = var.checkout_team_id
  change_reason  = "Manage the production response procedure"
  provenance_ref = "terraform/checkout-alerts"

  markdown = <<-EOT
    # Checkout agent recovery

    1. Inspect the linked failing traces and eval evidence.
    2. Check provider fallback and tool timeout rates.
    3. Roll back the current prompt release when the regression is release-bound.
  EOT
}
