# config_json takes the shape of kind; secret_refs_json holds only secret: references.

resource "o11y_alert_destination" "oncall_email" {
  destination_key  = "oncall-email"
  name             = "On-call email"
  kind             = "email"
  enabled          = true
  config_json      = jsonencode({ recipients = ["oncall@example.com"], reply_to = "sre@example.com" })
  secret_refs_json = jsonencode({})
}

resource "o11y_alert_destination" "incident_webhook" {
  destination_key = "incident-webhook"
  name            = "Incident webhook"
  kind            = "webhook"
  enabled         = true
  # Set method: omitted, the server stores POST and the apply fails with an inconsistent result.
  config_json = jsonencode({
    url     = "https://hooks.example.com/o11y"
    method  = "ALERT_WEBHOOK_METHOD_V1_POST"
    headers = [{ name = "X-Source", value = "o11y" }]
  })
  secret_refs_json = jsonencode({
    authorization  = "secret:incident-webhook-authorization"
    signing_secret = "secret:incident-webhook-signing"
  })
}

resource "o11y_alert_destination" "sre_slack" {
  destination_key  = "sre-slack"
  name             = "SRE Slack"
  kind             = "slack"
  enabled          = true
  config_json      = jsonencode({ channel_id = "C0123456789" })
  secret_refs_json = jsonencode({ token = "secret:slack-bot-token" })
}

resource "o11y_alert_destination" "pagerduty" {
  destination_key  = "pagerduty-checkout"
  name             = "PagerDuty checkout"
  kind             = "pagerduty"
  enabled          = true
  config_json      = jsonencode({})
  secret_refs_json = jsonencode({ routing_key = "secret:pagerduty-checkout-routing-key" })
}
