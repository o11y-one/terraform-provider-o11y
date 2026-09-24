variable "owner_team_id" {
  description = "UUID of the O11y.one team that owns and responds to this alert."
  type        = string
}

resource "o11y_sli" "checkout_availability" {
  sli_key               = "checkout-availability"
  name                  = "Checkout availability"
  indicator_kind        = "availability"
  aggregation           = "event_ratio"
  missing_data_behavior = "unknown"
  eligible_events       = "checkout server spans"
  good_events           = "checkout server spans without an error status"

  scope_json = jsonencode({
    service_names = ["checkout"]
    span_kinds    = ["SLI_SPAN_KIND_V1_SERVER"]
  })
}

resource "o11y_slo" "checkout_availability" {
  slo_key                = "checkout-availability"
  name                   = "Checkout availability"
  sli_id                 = o11y_sli.checkout_availability.id
  sli_revision_id        = o11y_sli.checkout_availability.current_revision_id
  target_ratio           = 0.999
  window_mode            = "rolling"
  rolling_window_seconds = 2592000
}

# 64-bit integers are quoted because that is how the provider writes them back on import.
resource "o11y_slo_burn_alert" "checkout_availability" {
  slug        = "checkout-availability-burn"
  name        = "Checkout availability burn"
  description = "Observe fast and slow error-budget burn for checkout availability."
  severity    = "critical"

  # No scope_json: the server copies it from the SLO's SLI revision.
  owner_json = jsonencode({ team_id = var.owner_team_id })
  action_json = jsonencode({
    summary      = "Checkout is spending its availability error budget too fast."
    first_action = "Open the failing checkout traces and compare with the last deploy."
  })
  evaluation_settings_json = jsonencode({
    pending_for_seconds = "120"
    no_data_behavior    = "ALERT_NO_DATA_BEHAVIOR_V1_HOLD_STATE"
  })
  evaluation_interval_seconds = 60
  sample_guard_json           = jsonencode({ minimum_events = "100" })

  # Inputs only. The target, window, and calendar fields come from the SLO
  # revision and are refused here.
  recipe_config_json = jsonencode({
    slo_id                    = o11y_slo.checkout_availability.id
    slo_revision_id           = o11y_slo.checkout_availability.current_revision_id
    fast_short_window_seconds = "300"
    fast_long_window_seconds  = "3600"
    slow_short_window_seconds = "1800"
    slow_long_window_seconds  = "21600"
    fast_burn_threshold       = 14.4
    slow_burn_threshold       = 6
    min_request_count         = "100"
    evidence_limit            = 10
  })

  paused = false
  notify = false
}
