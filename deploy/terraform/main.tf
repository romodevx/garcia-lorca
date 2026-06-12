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
