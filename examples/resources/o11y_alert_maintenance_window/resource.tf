# Suppresses notifications for alerts whose scope matches, between starts_at and ends_at.
resource "o11y_alert_maintenance_window" "checkout_deploy" {
  window_key = "checkout-deploy-2030-01-15"
  name       = "Checkout deploy"

  scope_json = jsonencode({
    service_names = ["checkout"]
    environments  = ["production"]
  })

  starts_at = "2030-01-15T02:00:00Z"
  ends_at   = "2030-01-15T04:00:00Z"
}
