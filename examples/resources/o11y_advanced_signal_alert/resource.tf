variable "owner_team_id" {
  description = "UUID of the O11y.one team that owns and responds to this alert."
  type        = string
}

# 64-bit integers are quoted because that is how the provider writes them back on import.
resource "o11y_advanced_signal_alert" "checkout" {
  slug        = "checkout-advanced-signals"
  name        = "Checkout advanced signals"
  description = "Observe log errors, tool failures, provider fallbacks, and cache misses for checkout."
  severity    = "warning"

  scope_json = jsonencode({
    service_names = ["checkout"]
    tool_names    = ["payments.charge"]
  })
  owner_json = jsonencode({ team_id = var.owner_team_id })
  action_json = jsonencode({
    summary      = "Checkout symptoms are rising."
    first_action = "Inspect failing tool calls and provider fallbacks for checkout."
  })
  evaluation_settings_json = jsonencode({
    pending_for_seconds    = "300"
    recovering_for_seconds = "300"
    no_data_behavior       = "ALERT_NO_DATA_BEHAVIOR_V1_RESOLVE"
  })
  evaluation_interval_seconds = 60
  sample_guard_json           = jsonencode({ minimum_events = "100" })

  # Every field is listed: the server fills any omitted one with its default
  # and echoes it, which would plan a replacement on every run.
  recipe_config_json = jsonencode({
    min_log_errors             = "10"
    max_log_error_rate         = 0.05
    min_tool_failures          = "5"
    min_fallback_count         = "5"
    max_cache_miss_rate        = 0.9
    symptom_only_page_override = false
    evidence_limit             = 10
  })

  paused = false
  notify = false
}
