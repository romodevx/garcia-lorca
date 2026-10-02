# Cloud Scheduler: the monthly billing run (M15).
#
# The school can always press "Generar cargos del mes" in the panel. This exists
# so nobody has to remember. Both call the SAME endpoint, and that is safe for
# one reason: RunBilling is idempotent — every charge it writes carries an
# external_ref unique per school — so a button pressed after the scheduler has
# run reports everything as skipped rather than charging a family twice.
#
# IMPORTANT, and easy to get wrong: turning module_tuition off does NOT stop
# this. Feature flags live in the gateway's ENABLED_MODULES env, and the
# scheduler calls finance directly. Stopping the run means setting
# billing_schedule_enabled = false here and applying — see FLAGS.md.

resource "google_project_service" "scheduler" {
  count   = var.billing_schedule_enabled || var.birthday_schedule_enabled ? 1 : 0
  project = var.project_id
  service = "cloudscheduler.googleapis.com"

  disable_on_destroy = false
}

# Its own identity, with invoker on finance and nothing else. Reusing the push SA
# would have blurred two different jobs into one identity, and the whole point of
# per-purpose service accounts is that revoking one does not revoke the other.
resource "google_service_account" "billing_scheduler" {
  count        = var.billing_schedule_enabled ? 1 : 0
  project      = var.project_id
  account_id   = "billing-scheduler"
  display_name = "Cloud Scheduler — monthly billing run"
}

resource "google_cloud_run_v2_service_iam_member" "billing_scheduler_invoker" {
  count    = var.billing_schedule_enabled ? 1 : 0
  project  = var.project_id
  location = var.region
  name     = module.finance.service_name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.billing_scheduler[0].email}"
}

# The job posts the Connect RPC directly. cycle_id is a variable and not derived:
# a scheduler that guessed which school year to bill would, on the day a cycle
# rolls over, bill the wrong one silently. Setting it is a deliberate act each
# August, and leaving it empty disables the schedule entirely.
resource "google_cloud_scheduler_job" "monthly_billing" {
  count       = var.billing_schedule_enabled && var.billing_cycle_id != "" ? 1 : 0
  project     = var.project_id
  region      = var.region
  name        = "monthly-billing-run"
  description = "Generates tuition, inscriptions and late fees for the active cycle"
  schedule    = var.billing_schedule_cron
  time_zone   = var.billing_schedule_timezone

  # One retry, well spaced. More would be pointless: the run is idempotent, so a
  # genuine failure is a failure of the ledger itself, and hammering it does not
  # help. The panel's button is the manual recovery path.
  retry_config {
    retry_count          = 1
    min_backoff_duration = "60s"
  }

  http_target {
    http_method = "POST"
    uri         = "${module.finance.uri}/garcialorca.finance.v1.FinanceService/RunBilling"
    headers = {
      "Content-Type" = "application/json"
    }
    # asOfDate is omitted on purpose: the run reads today's date, which is what a
    # schedule means. Passing a date would freeze the job at the day it was
    # written.
    body = base64encode(jsonencode({ cycleId = var.billing_cycle_id }))

    oidc_token {
      service_account_email = google_service_account.billing_scheduler[0].email
      audience              = module.finance.uri
    }
  }

  depends_on = [google_project_service.scheduler]
}

# Cloud Scheduler: the daily birthday greeting (T11).
#
# Every morning at 07:00 school time comms greets, in the parents' app, each
# enrolled pupil whose birthday it is (February 29 pupils on February 28 of a
# non-leap year). The job posts CommsService/RunBirthdays with NO date, so comms
# reads "today" in America/Mexico_City (SCHOOL_TIMEZONE) — never the UTC day,
# which after 18:00 local is already tomorrow.
#
# Safe to retry and to run twice: each greeting is claimed in comms'
# processed_events as birthday:<student>:<year> in the same transaction as its
# notification, so a retry greets nobody twice.
#
# IMPORTANT, as with billing: turning module_comms off does NOT stop this.
# Flags live in the gateway's ENABLED_MODULES and only gate gateway routes; the
# scheduler calls comms directly with its own OIDC identity. Stopping the
# greetings means birthday_schedule_enabled = false here and an apply (or
# pausing the job in the console).
#
# Repo config only as of T11: NOT applied. Turn it on per environment with
# birthday_schedule_enabled = true.

resource "google_service_account" "birthday_scheduler" {
  count        = var.birthday_schedule_enabled ? 1 : 0
  project      = var.project_id
  account_id   = "birthday-scheduler"
  display_name = "Cloud Scheduler — daily birthday greeting"
}

# Invoker on comms and nothing else; its own identity so revoking it never
# touches the billing job or the Pub/Sub push.
resource "google_cloud_run_v2_service_iam_member" "birthday_scheduler_invoker" {
  count    = var.birthday_schedule_enabled ? 1 : 0
  project  = var.project_id
  location = var.region
  name     = module.comms.service_name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.birthday_scheduler[0].email}"
}

resource "google_cloud_scheduler_job" "daily_birthdays" {
  count       = var.birthday_schedule_enabled ? 1 : 0
  project     = var.project_id
  region      = var.region
  name        = "daily-birthday-greetings"
  description = "Greets the families of the pupils whose birthday is today"
  schedule    = var.birthday_schedule_cron
  time_zone   = var.birthday_schedule_timezone

  # A couple of spaced retries: the run is idempotent, so a retry after a
  # partial failure (students blinked for one pupil) greets only who was missed.
  retry_config {
    retry_count          = 2
    min_backoff_duration = "120s"
  }

  http_target {
    http_method = "POST"
    uri         = "${module.comms.uri}/garcialorca.comms.v1.CommsService/RunBirthdays"
    headers = {
      "Content-Type" = "application/json"
    }
    # No date on purpose: comms reads today in the school's zone.
    body = base64encode(jsonencode({}))

    oidc_token {
      service_account_email = google_service_account.birthday_scheduler[0].email
      audience              = module.comms.uri
    }
  }

  depends_on = [google_project_service.scheduler]
}
