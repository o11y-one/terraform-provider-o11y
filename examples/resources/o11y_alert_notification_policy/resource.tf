variable "customer_impact_template_id" {
  description = "id of a published o11y_alert_notification_template."
  type        = string
}

variable "customer_impact_template_revision_id" {
  description = "published_revision_id of the same template, to pin that exact revision."
  type        = string
}

resource "o11y_alert_destination" "oncall_email" {
  destination_key  = "oncall-email"
  name             = "On-call email"
  kind             = "email"
  enabled          = true
  config_json      = jsonencode({ recipients = ["oncall@example.com"] })
  secret_refs_json = jsonencode({})
}

resource "o11y_alert_destination" "incident_webhook" {
  destination_key = "incident-webhook"
  name            = "Incident webhook"
  kind            = "webhook"
  enabled         = true
  config_json = jsonencode({
    url    = "https://hooks.example.com/o11y"
    method = "ALERT_WEBHOOK_METHOD_V1_POST"
  })
  secret_refs_json = jsonencode({ authorization = "secret:incident-webhook-authorization" })
}

resource "o11y_alert_notification_policy" "customer_impact" {
  policy_key = "customer-impact"
  name       = "Customer impact"
  enabled    = true

  # Set enabled = true on every node: an omitted enabled is stored as false.
  config_json = jsonencode({
    tree = {
      nodes = [
        {
          node_key  = "customer-impact"
          node_kind = "ALERT_POLICY_TREE_NODE_KIND_V1_BRANCH"
          enabled   = true
          priority  = 10
          matcher = [{
            field  = "class"
            values = [{ string_value = "outcome" }, { string_value = "budget" }]
          }]
          behavior = "ALERT_NOTIFICATION_ROUTE_BEHAVIOR_V1_STOP"
        },
        {
          node_key        = "oncall-email"
          parent_node_key = "customer-impact"
          node_kind       = "ALERT_POLICY_TREE_NODE_KIND_V1_ROUTE"
          enabled         = true
          priority        = 10
          target          = { destination_id = o11y_alert_destination.oncall_email.id }
          behavior        = "ALERT_NOTIFICATION_ROUTE_BEHAVIOR_V1_CONTINUE"
          # Templates render only on email and Slack targets.
          notification_template_id                    = var.customer_impact_template_id
          requested_notification_template_revision_id = var.customer_impact_template_revision_id
        },
      ]
    }
    grouping = {
      group_by               = ["service.name", "deployment.environment"]
      group_wait_seconds     = 30
      group_interval_seconds = 300
    }
    notifications = {
      repeat_interval_seconds = 900
      repeat_limit            = 2
      notify_resolved         = true
    }
    escalation = {
      schedule_key = "customer-impact"
      steps = [{
        delay_seconds = 600
        target        = { destination_id = o11y_alert_destination.incident_webhook.id }
      }]
    }
    max_pages_per_day  = 5
    max_pages_per_week = 20
  })
}
