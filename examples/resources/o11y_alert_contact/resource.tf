resource "o11y_alert_contact" "primary" {
  contact_key          = "primary-oncall"
  display_name         = "Primary on-call"
  email                = "oncall@example.com"
  request_verification = true
}
