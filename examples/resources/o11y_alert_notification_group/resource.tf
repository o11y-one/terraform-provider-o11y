resource "o11y_alert_contact" "primary" {
  contact_key          = "primary-oncall"
  display_name         = "Primary on-call"
  email                = "oncall@example.com"
  request_verification = true
}

resource "o11y_alert_destination" "sre_email" {
  destination_key  = "sre-email"
  name             = "SRE email"
  kind             = "email"
  enabled          = true
  config_json      = jsonencode({ recipients = ["sre@example.com"] })
  secret_refs_json = jsonencode({})
}

# List members in position order and set enabled on each: the server returns
# them that way and any other form shows a change on the next plan.
resource "o11y_alert_notification_group" "platform" {
  group_key = "platform-oncall"
  name      = "Platform on-call"
  enabled   = true

  members_json = jsonencode([
    {
      kind      = "contact"
      member_id = o11y_alert_contact.primary.id
      position  = 0
      enabled   = true
    },
    {
      kind      = "destination"
      member_id = o11y_alert_destination.sre_email.id
      position  = 10
      enabled   = true
    },
  ])
}
