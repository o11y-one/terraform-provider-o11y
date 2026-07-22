resource "o11y_alert_notification_template" "customer_impact" {
  template_key  = "customer-impact"
  name          = "Customer impact"
  description   = "Rich Email and Slack notification for customer-impacting incidents."
  change_reason = "Manage the customer-impact template in Terraform"
  published     = true

  document_json = jsonencode({
    schema_version   = 1
    subject_template = "[{{ incident.severity }}] {{ incident.title }}"
    firing = {
      title_template   = "{{ incident.title }}"
      summary_template = "{{ incident.customer_impact }}"
      blocks = [
        {
          key      = "summary"
          markdown = { text_template = "*Customer impact*\n{{ incident.customer_impact }}\n\n*Why this fired*\n{{ hypothesis }}" }
        },
        {
          key = "actions"
          actions = {
            actions = [{
              label_template = "Open alert"
              link           = "NOTIFICATION_TEMPLATE_ACTION_LINK_V1_OPEN_ALERT"
              style          = "NOTIFICATION_TEMPLATE_ACTION_STYLE_V1_PRIMARY"
            }]
          }
        }
      ]
    }
    resolved = {
      title_template   = "Resolved: {{ incident.title }}"
      summary_template = "Customer impact has recovered."
    }
  })
}
