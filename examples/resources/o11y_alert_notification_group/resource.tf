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
    }
  ])
}
