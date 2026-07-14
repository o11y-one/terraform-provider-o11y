# o11y_alert_contact

Creates a human email contact for alert notification groups. The backend sends a generation-bound verification email when `request_verification` is true. The token is confirmed in the o11y.one UI or API and is never stored in Terraform state.

```hcl
resource "o11y_alert_contact" "primary" {
  contact_key         = "primary-oncall"
  display_name        = "Primary on-call"
  email               = "oncall@example.com"
  request_verification = true
}
```

Deleting the resource archives the contact and invalidates its verification. Import with the contact UUID.

## Attributes

- `contact_key` (required): Stable organization-scoped key.
- `display_name` (required): Operator-facing name.
- `email` (required): Address to verify.
- `request_verification` (optional): Send a verification email for each unverified generation.
- `status`, `generation`, `revision`, `id` (computed): Authoritative backend state.
