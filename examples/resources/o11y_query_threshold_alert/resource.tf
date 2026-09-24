variable "owner_team_id" {
  description = "UUID of the O11y.one team that owns and responds to this alert."
  type        = string
}

# 64-bit integers are quoted because that is how the provider writes them back on import.
resource "o11y_query_threshold_alert" "checkout_errors" {
  slug        = "checkout-server-errors"
  name        = "Checkout server errors"
  description = "Observe sustained checkout server-span errors before promoting to notify mode."
  alert_class = "symptom"
  severity    = "warning"

  # The server adds scope values to the query as filters.
  scope_json = jsonencode({
    service_names = ["checkout"]
    span_kinds    = ["SLI_SPAN_KIND_V1_SERVER"]
  })
  owner_json = jsonencode({ team_id = var.owner_team_id })
  action_json = jsonencode({
    summary              = "Checkout server spans are failing."
    external_runbook_url = "https://runbooks.example.com/checkout-errors"
  })
  evaluation_settings_json = jsonencode({
    pending_for_seconds    = "300"
    recovering_for_seconds = "300"
    no_data_behavior       = "ALERT_NO_DATA_BEHAVIOR_V1_HOLD_STATE"
  })
  evaluation_interval_seconds = 60
  sample_guard_json           = jsonencode({ minimum_events = "25" })

  # The server applies no defaults to this recipe: every field below except
  # aggregation_field (empty for COUNT), filters, and group_by is required.
  recipe_config_json = jsonencode({
    dataset                  = "ALERT_QUERY_DATASET_V1_TRACES"
    aggregation              = "ALERT_QUERY_AGGREGATION_V1_COUNT"
    comparison               = "ALERT_QUERY_COMPARISON_V1_GREATER_THAN_OR_EQUAL"
    threshold                = 10
    window_seconds           = "300"
    evaluation_delay_seconds = "120"
    minimum_event_count      = "25"
    max_groups               = 20
    evidence_limit           = 10
    timeout_ms               = 2000
    group_by                 = ["service.name"]
    filters = [{
      field     = "span_attributes.error.type"
      operator  = "ALERT_QUERY_FILTER_OPERATOR_V1_EXISTS"
      data_type = "string"
      values    = []
    }]
  })

  paused = false
  notify = false
}
