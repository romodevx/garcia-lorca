# Hard billing cutoff. See deploy/functions/billing-killswitch/index.js for what
# firing actually does (everything stops, nothing is deleted, re-attaching the
# billing account restores it).
#
# OFF BY DEFAULT, and not because it is risky to run: the function's identity
# needs roles/billing.projectManager on the BILLING ACCOUNT, which is not a
# project-level role and cannot be granted from this configuration. Enabling this
# without that grant would create the whole pipeline and leave it silently unable
# to do the one thing it exists for. So: grant the role, then flip the flag.
#
#   gcloud billing accounts add-iam-policy-binding BILLING_ACCOUNT_ID \
#     --member="serviceAccount:$(terraform output -raw billing_killswitch_sa)" \
#     --role="roles/billing.projectManager"

variable "enable_billing_killswitch" {
  description = "Create the budget -> Pub/Sub -> Function pipeline that disconnects billing at 100% of budget. Requires roles/billing.projectManager on the billing account for the function's SA (granted out of band); leave false until that exists."
  type        = bool
  default     = false
}

locals {
  killswitch = var.enable_billing_killswitch ? 1 : 0
}

resource "google_project_service" "killswitch_apis" {
  for_each = var.enable_billing_killswitch ? toset([
    "cloudfunctions.googleapis.com",
    "eventarc.googleapis.com",
    "cloudscheduler.googleapis.com",
  ]) : toset([])
  project            = var.project_id
  service            = each.value
  disable_on_destroy = false
}

# The budget publishes here on every threshold crossing; the function decides
# which crossings matter.
resource "google_pubsub_topic" "budget_alerts" {
  count   = local.killswitch
  project = var.project_id
  name    = "budget-alerts"
  labels  = local.labels

  depends_on = [google_project_service.apis]
}

resource "google_service_account" "billing_killswitch" {
  count        = local.killswitch
  project      = var.project_id
  account_id   = "gl-billing-killswitch-${var.env}"
  display_name = "García Lorca billing kill switch (${var.env})"
}

# Source bundle. Kept in its own bucket with versioning off — it is derived
# artefact, reproducible from the repo at any time.
resource "google_storage_bucket" "functions" {
  count                       = local.killswitch
  project                     = var.project_id
  name                        = "${var.project_id}-functions-${var.env}"
  location                    = var.region
  uniform_bucket_level_access = true
  force_destroy               = true
  labels                      = local.labels

  depends_on = [google_project_service.apis]
}

data "archive_file" "billing_killswitch" {
  count       = local.killswitch
  type        = "zip"
  source_dir  = "${path.module}/../functions/billing-killswitch"
  output_path = "${path.module}/.terraform/billing-killswitch.zip"
}

resource "google_storage_bucket_object" "billing_killswitch" {
  count = local.killswitch
  # The hash is in the NAME, not just the metadata: a new bundle is a new object,
  # which is what makes the function actually redeploy when the source changes.
  name   = "billing-killswitch-${data.archive_file.billing_killswitch[0].output_md5}.zip"
  bucket = google_storage_bucket.functions[0].name
  source = data.archive_file.billing_killswitch[0].output_path
}

resource "google_cloudfunctions2_function" "billing_killswitch" {
  count    = local.killswitch
  project  = var.project_id
  name     = "billing-killswitch-${var.env}"
  location = var.region

  build_config {
    runtime     = "nodejs22"
    entry_point = "stopBilling"
    source {
      storage_source {
        bucket = google_storage_bucket.functions[0].name
        object = google_storage_bucket_object.billing_killswitch[0].name
      }
    }
  }

  service_config {
    # One instance is plenty and bounds the blast radius: this must never fan out.
    max_instance_count    = 1
    available_memory      = "256M"
    timeout_seconds       = 60
    service_account_email = google_service_account.billing_killswitch[0].email
    environment_variables = {
      TARGET_PROJECT_ID = var.project_id
    }
  }

  event_trigger {
    trigger_region = var.region
    event_type     = "google.cloud.pubsub.topic.v1.messagePublished"
    pubsub_topic   = google_pubsub_topic.budget_alerts[0].id
    retry_policy   = "RETRY_POLICY_RETRY"
  }

  depends_on = [google_project_service.killswitch_apis]
}
