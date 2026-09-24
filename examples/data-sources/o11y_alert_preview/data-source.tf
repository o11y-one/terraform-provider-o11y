variable "definition_id" {
  description = "id of an Observe alert, for example o11y_agent_quality_alert.checkout.id."
  type        = string
}

variable "revision_id" {
  description = "revision_id of the same alert, for example o11y_agent_quality_alert.checkout.revision_id."
  type        = string
}

# Replays that revision over one day of history without notifying anyone.
data "o11y_alert_preview" "checkout" {
  definition_id  = var.definition_id
  revision_id    = var.revision_id
  range_start    = "2030-01-14T00:00:00Z"
  range_end      = "2030-01-15T00:00:00Z"
  evidence_limit = 5
}

locals {
  preview = jsondecode(data.o11y_alert_preview.checkout.result_json)
}

output "preview_status" {
  value = local.preview.status
}

# 64-bit integers in result_json are JSON strings.
output "projected_notifications_per_day" {
  value = tonumber(try(local.preview.projected_notifications_per_day, "0"))
}

output "activation_blockers" {
  value = jsondecode(data.o11y_alert_preview.checkout.activation_blockers_json)
}
