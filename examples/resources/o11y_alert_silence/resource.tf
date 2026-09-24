resource "o11y_alert_silence" "checkout_low_severity" {
  silence_key = "checkout-low-severity-2030-01-15"
  reason      = "Checkout load test; page only on critical alerts"

  # An alert is silenced when every clause matches one of its values.
  # List clauses in field-name order: the server returns them sorted.
  matcher_json = jsonencode({
    clauses = [
      { field = "service_name", values = [{ string_value = "checkout" }] },
      { field = "severity", values = [{ string_value = "info" }, { string_value = "warning" }] },
    ]
  })

  starts_at = "2030-01-15T02:00:00Z"
  ends_at   = "2030-01-15T06:00:00Z"
}
