// Additional Observe-mode detector families.

resource "o11y_cost_per_success_alert" "cost" {
  slug                        = "cost-per-success"
  name                        = "Cost per success"
  description                 = "Detect cost regressions."
  severity                    = "warning"
  scope_json                  = jsonencode({ service_names = ["agent-api"] })
  owner_json                  = jsonencode({ team_id = "019f7aa2-6c7f-7000-8000-000000000010" })
  action_json                 = jsonencode({ summary = "Inspect execution cost, token use, and failed runs." })
  evaluation_settings_json    = jsonencode({ pending_for_seconds = 300 })
  evaluation_interval_seconds = 300
  sample_guard_json           = jsonencode({ minimum_events = "50" })
  recipe_config_json = jsonencode({
    window_seconds                = 3600
    min_success_count             = "50"
    max_cost_per_success_usd      = 0.50
    baseline_cost_per_success_usd = 0.10
    regression_multiplier         = 2.0
    allow_estimated_cost          = false
    imminent_exhaustion_hours     = 24.0
    evidence_limit                = 10
  })
  paused = false
  notify = false
}

resource "o11y_slo_burn_alert" "availability" {
  slug                        = "agent-slo-burn"
  name                        = "Agent SLO burn"
  description                 = "Detect SLO burn."
  severity                    = "critical"
  scope_json                  = jsonencode({ service_names = ["agent-api"] })
  owner_json                  = jsonencode({ team_id = "019f7aa2-6c7f-7000-8000-000000000010" })
  action_json                 = jsonencode({ summary = "Inspect customer-impacting errors and latency." })
  evaluation_settings_json    = jsonencode({ pending_for_seconds = 120 })
  evaluation_interval_seconds = 60
  sample_guard_json           = jsonencode({ minimum_events = "100" })
  recipe_config_json = jsonencode({
    fast_short_window_seconds = 300
    fast_long_window_seconds  = 3600
    slow_short_window_seconds = 1800
    slow_long_window_seconds  = 21600
    slo_window_seconds        = 2592000
    target_percent            = 99.5
    fast_burn_threshold       = 14.4
    slow_burn_threshold       = 6.0
    min_request_count         = "100"
    evidence_limit            = 10
  })
  paused = false
  notify = false
}

resource "o11y_advanced_signal_alert" "latency" {
  slug                        = "agent-latency"
  name                        = "Agent latency"
  description                 = "Detect elevated latency."
  severity                    = "warning"
  scope_json                  = jsonencode({ service_names = ["agent-api"] })
  owner_json                  = jsonencode({ team_id = "019f7aa2-6c7f-7000-8000-000000000010" })
  action_json                 = jsonencode({ summary = "Inspect slow traces and downstream dependencies." })
  evaluation_settings_json    = jsonencode({ pending_for_seconds = 300 })
  evaluation_interval_seconds = 60
  sample_guard_json           = jsonencode({ minimum_events = "100" })
  recipe_config_json          = jsonencode({ min_log_errors = "10", max_log_error_rate = 0.05, evidence_limit = 10 })
  paused                      = false
  notify                      = false
}
