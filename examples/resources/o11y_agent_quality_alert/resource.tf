resource "o11y_agent_quality_alert" "quality" {
  slug                        = "agent-quality-regression"
  name                        = "Agent quality regression"
  description                 = "Detect sustained quality regressions."
  severity                    = "warning"
  scope_json                  = jsonencode({ service_names = ["agent-api"] })
  owner_json                  = jsonencode({ team_id = "019f7aa2-6c7f-7000-8000-000000000010" })
  action_json                 = jsonencode({ summary = "Inspect failed agent runs and eval evidence." })
  evaluation_settings_json    = jsonencode({ pending_for_seconds = 300 })
  evaluation_interval_seconds = 300
  sample_guard_json           = jsonencode({ minimum_events = "50" })
  recipe_config_json = jsonencode({
    short_window_seconds      = 900
    long_window_seconds       = 3600
    baseline_window_seconds   = 86400
    min_run_count             = "50"
    max_bad_outcome_rate      = 0.05
    max_eval_fail_rate        = 0.05
    min_eval_pass_rate        = 0.95
    baseline_bad_outcome_rate = 0.02
    regression_multiplier     = 2.0
    evidence_limit            = 10
    use_run_quality_facts     = false
  })
  paused = false
  notify = false
}
