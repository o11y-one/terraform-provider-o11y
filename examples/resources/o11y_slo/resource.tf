resource "o11y_slo" "checkout" {
  slo_key         = "checkout-availability"
  name            = "Checkout availability"
  description     = "Monthly availability budget for checkout"
  sli_id          = "019f7aa2-6c7f-7000-8000-000000000001"
  sli_revision_id = "019f7aa2-6c7f-7000-8000-000000000002"
  target_ratio    = 0.999

  window_mode       = "calendar"
  calendar_period   = "month"
  calendar_timezone = "America/New_York"

  labels_json = jsonencode({ service = "checkout" })
}
