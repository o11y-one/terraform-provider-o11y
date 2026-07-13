resource "o11y_alert_destination" "primary" {
  destination_key = "primary-webhook"
  name            = "Primary webhook"
  kind            = "webhook"
  enabled         = true
  config_json     = jsonencode({ url = "https://hooks.example.com/o11y" })
  secret_refs_json = jsonencode({
    authorization = "secret:primary-webhook-authorization"
  })
}
