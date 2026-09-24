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
          key = "signal"
          fields = {
            title_template = "Signal"
            fields = [
              { label_template = "Severity", value_path = "incident.severity", format = "NOTIFICATION_TEMPLATE_VALUE_FORMAT_V1_TEXT" },
              { label_template = "Confidence", value_path = "impact.confidence", format = "NOTIFICATION_TEMPLATE_VALUE_FORMAT_V1_PERCENT" },
            ]
          }
        },
        {
          key = "actions"
          actions = {
            actions = [
              {
                label_template = "Open alert"
                link           = "NOTIFICATION_TEMPLATE_ACTION_LINK_V1_OPEN_ALERT"
                style          = "NOTIFICATION_TEMPLATE_ACTION_STYLE_V1_PRIMARY"
              },
              {
                label_template = "View traces"
                link           = "NOTIFICATION_TEMPLATE_ACTION_LINK_V1_VIEW_TRACES"
                style          = "NOTIFICATION_TEMPLATE_ACTION_STYLE_V1_DEFAULT"
              },
            ]
          }
        },
      ]
    }
    resolved = {
      title_template   = "Resolved: {{ incident.title }}"
      summary_template = "Customer impact has recovered."
      blocks = [{
        key      = "recovery"
        markdown = { text_template = "*Resolved*\nCustomer impact has recovered." }
      }]
    }
    reminder = {
      title_template   = "Reminder: {{ incident.title }}"
      summary_template = "Customer impact is still ongoing."
      blocks = [{
        key      = "ongoing-impact"
        markdown = { text_template = "*Ongoing customer impact*\n{{ incident.customer_impact }}" }
      }]
    }
  })
}
