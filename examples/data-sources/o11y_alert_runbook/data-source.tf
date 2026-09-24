variable "runbook_id" {
  description = "id of an o11y_alert_runbook."
  type        = string
}

data "o11y_alert_runbook" "checkout" {
  runbook_id = var.runbook_id
}

output "checkout_runbook_revision" {
  value = data.o11y_alert_runbook.checkout.current_revision_id
}
