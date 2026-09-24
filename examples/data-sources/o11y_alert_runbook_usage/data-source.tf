variable "runbook_id" {
  description = "id of an o11y_alert_runbook."
  type        = string
}

data "o11y_alert_runbook_usage" "checkout" {
  runbook_id = var.runbook_id
}

output "linked_alert_ids" {
  value = data.o11y_alert_runbook_usage.checkout.definition_ids
}
