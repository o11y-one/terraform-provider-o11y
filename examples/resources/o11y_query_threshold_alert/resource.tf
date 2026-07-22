variable "owner_team_id" {
  description = "O11y.one team that owns and responds to this alert."
  type        = string
}

resource "o11y_query_threshold_alert" "checkout_traffic" {
  slug                        = "checkout-server-error-rate"
  name                        = "Checkout server errors"
  description                 = "Observe sustained checkout server-span errors before promoting to notify mode."
  alert_class                 = "symptom"
  severity                    = "warning"
  scope_json                  = jsonencode({ service_names = ["checkout"] })
  owner_json                  = jsonencode({ team_id = var.owner_team_id, display_name = "Checkout on-call", active = true })
  action_json                 = jsonencode({ summary = "Inspect checkout traces and recent deployments." })
  evaluation_settings_json    = jsonencode({ pending_for_seconds = 300, recovering_for_seconds = 300 })
  evaluation_interval_seconds = 60
  sample_guard_json           = jsonencode({ minimum_events = "25" })

  recipe_config_json = jsonencode({
    dataset         = "ALERT_QUERY_DATASET_V1_TRACES"
    aggregation     = "ALERT_QUERY_AGGREGATION_V1_COUNT"
    comparison      = "ALERT_QUERY_COMPARISON_V1_GREATER_THAN_OR_EQUAL"
    threshold       = 10
    window_seconds  = "300"
    evaluation_delay_seconds = "120"
    minimum_event_count       = "25"
    max_groups                = 20
    evidence_limit            = 10
    timeout_ms                = 2000
    group_by                  = ["service.name"]
    filters = [{
      field     = "service.name"
      operator  = "ALERT_QUERY_FILTER_OPERATOR_V1_EQUAL"
      data_type = "string"
      values    = [{ string_value = "checkout" }]
    }]
  })

  paused = false
  notify = false
}
