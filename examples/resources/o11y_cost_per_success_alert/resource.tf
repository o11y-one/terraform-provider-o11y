variable "owner_team_id" {
  description = "UUID of the O11y.one team that owns and responds to this alert."
  type        = string
}

# 64-bit integers are quoted because that is how the provider writes them back on import.
resource "o11y_cost_per_success_alert" "checkout" {
  slug        = "checkout-cost-per-success"
  name        = "Checkout cost per success"
  description = "Observe LLM spend per successful checkout-agent run."
  severity    = "warning"

  scope_json = jsonencode({
    agent_names     = ["checkout-agent"]
    model_providers = ["openai"]
  })
  owner_json = jsonencode({ team_id = var.owner_team_id })
  action_json = jsonencode({
    summary      = "Spend per successful checkout run has regressed."
    first_action = "Check token usage, retries, and failed runs for the checkout agent."
  })
  evaluation_settings_json = jsonencode({
    pending_for_seconds = "600"
    no_data_behavior    = "ALERT_NO_DATA_BEHAVIOR_V1_HOLD_STATE"
  })
  evaluation_interval_seconds = 300
  sample_guard_json           = jsonencode({ minimum_events = "20" })

  # Every field with a server default is listed; budget_remaining_usd has none
  # and may be omitted to turn off the budget-exhaustion check.
  recipe_config_json = jsonencode({
    window_seconds                = "3600"
    min_success_count             = "20"
    max_cost_per_success_usd      = 0.5
    baseline_cost_per_success_usd = 0.1
    regression_multiplier         = 2
    allow_estimated_cost          = false
    budget_remaining_usd          = 250
    imminent_exhaustion_hours     = 24
    evidence_limit                = 10
  })

  paused = false
  notify = false
}
