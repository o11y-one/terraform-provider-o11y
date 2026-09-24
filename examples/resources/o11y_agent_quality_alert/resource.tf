variable "owner_team_id" {
  description = "UUID of the O11y.one team that owns and responds to this alert."
  type        = string
}

# 64-bit integers are quoted because that is how the provider writes them back on import.
resource "o11y_agent_quality_alert" "checkout" {
  slug        = "checkout-agent-quality"
  name        = "Checkout agent quality"
  description = "Observe sustained bad-outcome and eval-failure regressions for the checkout agent."
  severity    = "warning"

  scope_json = jsonencode({
    agent_names  = ["checkout-agent"]
    environments = ["production"]
  })
  owner_json = jsonencode({ team_id = var.owner_team_id })
  action_json = jsonencode({
    summary              = "Checkout agent answers are regressing."
    first_action         = "Compare the failing runs with the most recent prompt release."
    external_runbook_url = "https://runbooks.example.com/checkout-agent-quality"
  })
  evaluation_settings_json = jsonencode({
    pending_for_seconds    = "300"
    recovering_for_seconds = "600"
    no_data_behavior       = "ALERT_NO_DATA_BEHAVIOR_V1_HOLD_STATE"
  })
  evaluation_interval_seconds = 300
  sample_guard_json           = jsonencode({ minimum_events = "50" })

  # Every field is listed: the server fills an omitted one with its default
  # and returns it, and the apply fails with an inconsistent result.
  recipe_config_json = jsonencode({
    short_window_seconds      = "900"
    long_window_seconds       = "3600"
    baseline_window_seconds   = "86400"
    min_run_count             = "50"
    max_bad_outcome_rate      = 0.05
    max_eval_fail_rate        = 0.05
    min_eval_pass_rate        = 0.95
    baseline_bad_outcome_rate = 0.02
    regression_multiplier     = 2
    evidence_limit            = 10
    use_run_quality_facts     = true
  })

  paused = false
  notify = false
}
