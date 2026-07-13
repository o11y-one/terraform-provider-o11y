resource "o11y_alert_notification_policy" "default" {
  policy_key = "default-routing"
  name       = "Default routing"
  enabled    = true
  config_json = jsonencode({
    routes = [{
      route_key      = "primary"
      destination_id = o11y_alert_destination.primary.id
      matcher        = { severity = ["warning", "critical"] }
      priority       = 10
    }]
  })
}
