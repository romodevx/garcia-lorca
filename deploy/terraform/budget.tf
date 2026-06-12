# Budget alerting from day one. Alerts notify by email at threshold percentages
# of the monthly budget. This does NOT cap spend; it warns early.

resource "google_billing_budget" "monthly" {
  billing_account = var.billing_account_id
  display_name    = "garcia-lorca-${var.env}-monthly"

  budget_filter {
    projects               = ["projects/${data.google_project.current.number}"]
    calendar_period        = "MONTH"
    credit_types_treatment = "INCLUDE_ALL_CREDITS"
  }

  amount {
    specified_amount {
      currency_code = var.budget_currency
      units         = tostring(var.monthly_budget_amount)
    }
  }

  # Warn at 50/80/100% of actual spend, and at 100% of forecasted spend.
  threshold_rules {
    threshold_percent = 0.5
    spend_basis       = "CURRENT_SPEND"
  }
  threshold_rules {
    threshold_percent = 0.8
    spend_basis       = "CURRENT_SPEND"
  }
  threshold_rules {
    threshold_percent = 1.0
    spend_basis       = "CURRENT_SPEND"
  }
  threshold_rules {
    threshold_percent = 1.0
    spend_basis       = "FORECASTED_SPEND"
  }

  dynamic "all_updates_rule" {
    for_each = length(var.budget_alert_emails) > 0 ? [1] : []
    content {
      monitoring_notification_channels = [
        for c in google_monitoring_notification_channel.budget : c.id
      ]
      disable_default_iam_recipients = false
    }
  }

  depends_on = [google_project_service.apis]
}

# Email notification channels for budget alerts.
resource "google_monitoring_notification_channel" "budget" {
  for_each     = toset(var.budget_alert_emails)
  project      = var.project_id
  display_name = "budget-alert-${each.value}"
  type         = "email"

  labels = {
    email_address = each.value
  }
}
