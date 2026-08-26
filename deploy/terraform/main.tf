# Root composition for one environment. Apply with a per-env tfvars file:
#   terraform apply -var-file=environments/dev.tfvars
# Secrets are never placed in tfvars; they live in Secret Manager (see secrets.tf).

locals {
  labels = {
    project = "garcia-lorca"
    env     = var.env
  }

  registry_host = "${var.region}-docker.pkg.dev"
  registry_path = "${local.registry_host}/${var.project_id}/${var.artifact_registry_repo}"

  # image(name) resolves the per-service tag, falling back to the global default.
  image = {
    for name in keys(var.image_tags) :
    name => "${local.registry_path}/${name}:${var.image_tags[name]}"
  }
}

# ---- Enabled APIs ---------------------------------------------------------
resource "google_project_service" "apis" {
  for_each = toset([
    "run.googleapis.com",
    "sqladmin.googleapis.com",
    "pubsub.googleapis.com",
    "secretmanager.googleapis.com",
    "artifactregistry.googleapis.com",
    "cloudbuild.googleapis.com",
    "billingbudgets.googleapis.com",
    "iam.googleapis.com",
  ])
  project            = var.project_id
  service            = each.value
  disable_on_destroy = false
}

# ---- Artifact Registry (container images) ---------------------------------
resource "google_artifact_registry_repository" "images" {
  project       = var.project_id
  location      = var.region
  repository_id = var.artifact_registry_repo
  description   = "García Lorca service container images."
  format        = "DOCKER"
  labels        = local.labels

  # Cost/hygiene: keep the 5 most-recent versions per package (rollback targets)
  # and delete untagged images older than 30 days (churn from every CI build).
  # See deploy/COST-AND-DEPLOY.md §C5.
  cleanup_policies {
    id     = "keep-recent-versions"
    action = "KEEP"
    most_recent_versions {
      keep_count = 5
    }
  }

  cleanup_policies {
    id     = "delete-old-untagged"
    action = "DELETE"
    condition {
      tag_state  = "UNTAGGED"
      older_than = "2592000s" # 30 days
    }
  }

  depends_on = [google_project_service.apis]
}

# ---- Runtime service account (least privilege per environment) ------------
# Each Cloud Run revision runs as this identity. It is granted only what it
# needs: read secrets, publish/subscribe Pub/Sub, connect to Cloud SQL.
resource "google_service_account" "runtime" {
  project      = var.project_id
  account_id   = "garcia-lorca-run-${var.env}"
  display_name = "García Lorca Cloud Run runtime (${var.env})"
}

resource "google_project_iam_member" "runtime_sql_client" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.runtime.email}"
}

resource "google_project_iam_member" "runtime_secret_accessor" {
  project = var.project_id
  role    = "roles/secretmanager.secretAccessor"
  member  = "serviceAccount:${google_service_account.runtime.email}"
}

resource "google_project_iam_member" "runtime_pubsub" {
  for_each = toset(["roles/pubsub.publisher", "roles/pubsub.subscriber"])
  project  = var.project_id
  role     = each.value
  member   = "serviceAccount:${google_service_account.runtime.email}"
}

# ---- Pub/Sub push identity ------------------------------------------------
# Dedicated SA that Pub/Sub impersonates to mint the OIDC token on each push
# request. It is NOT the runtime SA: the runtime identity runs the container,
# while this identity only proves "this call came from Pub/Sub" to the comms
# /pubsub receiver. Keeping them separate means the push path can be granted or
# revoked without touching the service's own permissions. See pubsub.tf, where
# the comms subscriptions reference this SA in their oidc_token block.
resource "google_service_account" "pubsub_push" {
  project      = var.project_id
  account_id   = "garcia-lorca-pubsub-push-${var.env}"
  display_name = "García Lorca Pub/Sub push (${var.env})"
}

# The push SA must be able to invoke the comms Cloud Run service; the OIDC token
# it presents is only accepted because it carries roles/run.invoker on comms.
# Scoped to the single service (not project-wide) — this identity has no reason
# to reach any other service.
resource "google_cloud_run_v2_service_iam_member" "pubsub_push_comms_invoker" {
  project  = var.project_id
  location = var.region
  name     = module.comms.service_name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.pubsub_push.email}"
}

# The same identity, scoped separately, for the second push consumer: finance
# wakes on StudentEnrolled to charge a pupil's inscription (M15). Two bindings
# rather than one project-wide grant, so this identity can reach exactly the two
# services that receive pushes and nothing else.
resource "google_cloud_run_v2_service_iam_member" "pubsub_push_students_invoker" {
  project  = var.project_id
  location = var.region
  name     = module.students.service_name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.pubsub_push.email}"
}

resource "google_cloud_run_v2_service_iam_member" "pubsub_push_finance_invoker" {
  project  = var.project_id
  location = var.region
  name     = module.finance.service_name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.pubsub_push.email}"
}

# REQUIRED for OIDC push: the Pub/Sub service agent (local.pubsub_sa, defined in
# pubsub.tf) must be able to create tokens as the push SA. Without this binding
# Pub/Sub cannot mint the OIDC token and every push delivery fails with a
# permission error. This is the standard authenticated-push prerequisite.
resource "google_service_account_iam_member" "pubsub_agent_token_creator" {
  service_account_id = google_service_account.pubsub_push.name
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = local.pubsub_sa
}
