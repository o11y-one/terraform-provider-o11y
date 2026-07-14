# o11y_alert_notification_group

Creates a bounded reusable group of contacts, provider destinations, or nested groups. The backend rejects cycles, missing cross-tenant members, duplicate targets, and expansions over the server limit.

```hcl
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
```

Deleting the resource archives the group. Import with the group UUID. Notification policy routes can target the resulting group UUID with `group_id` in their typed route configuration.

## Attributes

- `group_key` (required): Stable organization-scoped key.
- `name` (required): Operator-facing name.
- `enabled` (required): Whether policy simulation may expand the group.
- `members_json` (required): Ordered array of `kind`, `member_id`, `position`, and optional `enabled`.
- `revision`, `id` (computed): Authoritative backend state.
