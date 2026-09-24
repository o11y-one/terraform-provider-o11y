variable "runbook_id" {
  description = "id of an o11y_alert_runbook."
  type        = string
}

data "o11y_alert_runbook_revisions" "checkout" {
  runbook_id = var.runbook_id
  limit      = 10
}

output "revision_history" {
  value = [
    for revision in jsondecode(data.o11y_alert_runbook_revisions.checkout.revisions_json) :
    "${revision.revision_number}: ${revision.change_reason} (${revision.created_at})"
  ]
}
