resource "o11y_alert_notification_policy" "default" {
  policy_key = "default-routing"
  name       = "Default routing"
  enabled    = true
  config_json = jsonencode({
    tree = {
      nodes = [
        {
          node_key           = "customer-impact"
          node_kind          = "branch"
          matcher            = { class = ["outcome", "budget"] }
          priority           = 10
          continue_evaluation = false
        },
        {
          node_key            = "primary"
          parent_node_key     = "customer-impact"
          node_kind           = "route"
          destination_id      = o11y_alert_destination.primary.id
          priority            = 10
          continue_evaluation = true
        }
      ]
    }
    grouping = {
      group_by              = ["service.name", "environment"]
      group_wait_seconds    = 30
      group_interval_seconds = 300
    }
    notifications = {
      repeat_interval_seconds = 900
      repeat_limit             = 2
      notify_resolved          = true
    }
    escalation = {
      schedule_key = "critical"
      steps = [{
        delay_seconds = 600
        destination_id = o11y_alert_destination.primary.id
      }]
    }
    noise_budget = {
      max_pages_per_day  = 5
      max_pages_per_week = 20
    }
  })
}
