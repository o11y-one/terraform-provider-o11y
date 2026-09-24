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

# A rolling 28-day objective.
resource "o11y_slo" "checkout_rolling" {
  slo_key                = "checkout-availability-28d"
  name                   = "Checkout availability, 28 days"
  sli_id                 = o11y_sli.checkout_availability.id
  sli_revision_id        = o11y_sli.checkout_availability.current_revision_id
  target_ratio           = 0.999
  window_mode            = "rolling"
  rolling_window_seconds = 2419200

  labels_json = jsonencode({ service = "checkout", tier = "1" })
}

# A calendar-month objective that resets at midnight New York time.
resource "o11y_slo" "checkout_monthly" {
  slo_key           = "checkout-availability-monthly"
  name              = "Checkout availability, monthly"
  description       = "Monthly availability budget for checkout."
  sli_id            = o11y_sli.checkout_availability.id
  sli_revision_id   = o11y_sli.checkout_availability.current_revision_id
  target_ratio      = 0.995
  window_mode       = "calendar"
  calendar_period   = "month"
  calendar_timezone = "America/New_York"
}
