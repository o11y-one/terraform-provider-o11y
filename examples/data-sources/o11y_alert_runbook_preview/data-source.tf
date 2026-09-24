# Validates and renders Markdown without saving it; a refused document fails the plan.
data "o11y_alert_runbook_preview" "draft" {
  markdown = <<-EOT
    # Checkout agent recovery

    1. Inspect the linked failing traces and eval evidence.
    2. Roll back the current prompt release when the regression is release-bound.
  EOT
}

output "rendered_html" {
  value = data.o11y_alert_runbook_preview.draft.rendered_html
}
