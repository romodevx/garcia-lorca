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
    # Read by data.google_project (pubsub.tf). NOTE: enabling it HERE is not
    # enough — a data source with fully-known config is read during plan/refresh,
    # before this resource can create anything, so a virgin project fails the
    # very first `terraform plan`. It must also be enabled out of band once:
    #   gcloud services enable cloudresourcemanager.googleapis.com --project=...
    # Kept in this list so the state stays declarative and re-applies are inert.
    "cloudresourcemanager.googleapis.com",
    # Budget alert notification channels.
    "monitoring.googleapis.com",
    # Reading the billing account the budget attaches to.
    "cloudbilling.googleapis.com",
    # Minting OIDC tokens for service-to-service calls (pkg/gcpauth).
    "iamcredentials.googleapis.com",
    # Pupils' photos: the private bucket in storage.tf.
    "storage.googleapis.com",
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

# ---- Cloud Build identity --------------------------------------------------
# Since the mid-2024 default change, a project running its first build uses the
# COMPUTE ENGINE default service account — not the legacy
# @cloudbuild.gserviceaccount.com, which is no longer created. compute.googleapis.com
# is not enabled here (deliberately: no VMs), so that account does not exist
# either, and `gcloud builds submit` would fail before running a single step.
#
# So: a dedicated build identity, passed explicitly with --service-account. That
# requires the build to use CLOUD_LOGGING_ONLY, which cloudbuild.yaml already sets.
resource "google_service_account" "build" {
  project      = var.project_id
  account_id   = "garcia-lorca-build-${var.env}"
  display_name = "García Lorca Cloud Build (${var.env})"

  depends_on = [google_project_service.apis]
}

resource "google_project_iam_member" "build_roles" {
  for_each = toset([
    "roles/artifactregistry.writer", # push the service images
    "roles/logging.logWriter",       # required when a build names its own SA
    # `gcloud builds submit` uploads the source tarball to gs://PROJECT_cloudbuild
    # and the build then READS it back. A user-specified SA does not inherit the
    # legacy Cloud Build agent's access to that bucket.
    #
    # objectViewer alone covers only the READ half, and the comment above used to
    # describe both halves while the role granted one. The submit UPLOADS first,
    # so it needs to create objects too — every run since died at step 6 with
    # "The user is forbidden from accessing the bucket [PROJECT_cloudbuild]".
    # serviceusage.services.use — the permission gcloud's error message names by
    # hand. Necessary, but on its own it fixed nothing (verified: the run after
    # granting it failed identically).
    #
    # The OBJECT grants that used to be described here moved to
    # build_staging_bucket, scoped to the one bucket. What stays at project level
    # is only what cannot be bucket-scoped.
    "roles/serviceusage.serviceUsageConsumer",
    # Without this the SA could not create a build at all. It stayed hidden
    # because the source-staging step fails first, so the bucket error was the
    # only one anybody ever saw.
    "roles/cloudbuild.builds.editor",
    # Bucket METADATA at project scope, and nothing else: bucketViewer is
    # exactly two permissions, storage.buckets.get and storage.buckets.list.
    # `gcloud builds submit` resolves the staging bucket before it uploads, and
    # a binding on the bucket alone did not satisfy that lookup (verified: the
    # run after granting it failed identically). Google's own guidance here is
    # roles/storage.admin, which is 105 permissions and would hand a CI build
    # object access to every bucket in the project — including the terraform
    # state. Two permissions buy the same lookup without that.
    "roles/storage.bucketViewer",
    # NOTE: the storage OBJECT grants are deliberately NOT here. See
    # build_staging_bucket below — project-wide object access reached the
    # terraform state bucket, which is not something a CI build should hold.
    # Still required, by a different caller than the comment used to name: the
    # `_DEPLOY=true` build step is gone, but deploy-dev.yml impersonates THIS
    # SA and runs `gcloud run services update` from the runner. Do not drop it.
    "roles/run.admin",
    "roles/iam.serviceAccountUser", # deploy a revision that RUNS AS the runtime SA
    # Connect to Cloud SQL through the Auth Proxy, so the deploy can apply the
    # migrations before it rolls out the images that need them. Project-scoped
    # because that is the only scope cloudsql.client has; it grants the ability
    # to OPEN a connection, not to read anything — the database still asks for a
    # role and a password, which is the grant below and nothing wider.
    "roles/cloudsql.client",
  ])
  project = var.project_id
  role    = each.value
  member  = "serviceAccount:${google_service_account.build.email}"
}

# ---- The migrator password, for the deploy's migration step -----------------
#
# SCOPED TO THIS ONE SECRET, deliberately, and for the same reason the storage
# grants below are scoped to one bucket: roles/secretmanager.secretAccessor at
# project level would hand a CI build the IAM signing key, the database
# application password and the owner's temporary password. A build that can read
# every secret is a build whose compromise is total.
#
# What this buys: the deploy can run scripts/migrate-cloud.sh as the migrator
# role before rolling out images. Applying the schema was manual, so dev drifted
# ELEVEN migrations behind the deployed code and nobody found out until a
# colegiatura failed to save in front of the school (2026-09-07).
resource "google_secret_manager_secret_iam_member" "build_reads_migrator_password" {
  count = var.use_cloud_sql ? 1 : 0

  project   = var.project_id
  secret_id = google_secret_manager_secret.db_migrator_password[0].secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.build.email}"
}

# ---- Staging bucket for `gcloud builds submit` ------------------------------
# Cloud Build stages the source tarball in gs://PROJECT_cloudbuild, which it
# creates on first use and which terraform therefore does not own. Only the IAM
# on it is managed here.
#
# SCOPED TO THIS BUCKET ON PURPOSE. These two roles lived in build_roles at the
# PROJECT level, which reached every bucket the project has — including
# garcia-lorca-tfstate, the terraform state. A CI build that can rewrite state
# decides what the next apply does, so a compromised build became a compromised
# environment. Nothing needed that: the build touches exactly one bucket.
#
# legacyBucketReader carries storage.buckets.get, the permission whose absence
# produced "The user is forbidden from accessing the bucket" — and it is
# bucket-only by design: granting it on a project is rejected outright with
# "Role roles/storage.legacyBucketReader is not supported for this resource".
# The error was telling us where the grant belonged.
resource "google_storage_bucket_iam_member" "build_staging_bucket" {
  for_each = toset([
    "roles/storage.legacyBucketReader", # see the bucket (get)
    "roles/storage.objectAdmin",        # upload the source tarball, read it back
  ])
  bucket = "${var.project_id}_cloudbuild"
  role   = each.value
  member = "serviceAccount:${google_service_account.build.email}"
}

# run.admin lets the build update a service; actually setting that service's
# identity to the runtime SA additionally needs actAs on THAT account.
resource "google_service_account_iam_member" "build_acts_as_runtime" {
  service_account_id = google_service_account.runtime.name
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${google_service_account.build.email}"
}
