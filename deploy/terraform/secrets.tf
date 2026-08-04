# Secret Manager. Secret VALUES are never stored in Terraform code or tfvars.
#
# Workflow:
#   1. Terraform creates the empty secret container (google_secret_manager_secret).
#   2. An operator (or CI with a secret-admin identity) writes the value out of
#      band, e.g.:
#        printf '%s' "$(openssl rand -base64 32)" \
#          | gcloud secrets versions add garcia-lorca-db-app-password-dev --data-file=-
#   3. The Cloud SQL user and the Cloud Run revisions reference "latest".
#
# The google_secret_manager_secret_version below is OPTIONAL bootstrap: it is
# created only when var.db_app_password is supplied via a CI-injected variable
# (never committed). If left empty, manage versions entirely out of band.

resource "google_secret_manager_secret" "db_app_password" {
  project   = var.project_id
  secret_id = "garcia-lorca-db-app-password-${var.env}"
  labels    = local.labels

  replication {
    auto {}
  }

  depends_on = [google_project_service.apis]
}

# Bootstrap variable — injected by CI from a secure source, defaults empty.
variable "db_app_password" {
  description = "App DB password. Injected by CI from a secure source; NEVER committed. Leave empty to manage versions out of band."
  type        = string
  default     = ""
  sensitive   = true
}

resource "google_secret_manager_secret_version" "db_app_password" {
  secret      = google_secret_manager_secret.db_app_password.id
  secret_data = var.db_app_password

  lifecycle {
    # Once the secret has a real value (managed out of band), ignore drift so a
    # bootstrap apply never clobbers the live password.
    ignore_changes = [secret_data]
  }
}

# ---- Owner temporary password (SHARED envs only) --------------------------
# STUB — inert until a shared environment opts in via var.seed_owner_secret.
#
# Local dev does NOT use this: it reads OWNER_TEMP_PASSWORD from .env (see the
# repo-root .env.example) which docker/seed-owner.sql consumes to seed the owner
# via pgcrypto. This block lets a SHARED env source the same one-time owner temp
# password from Secret Manager instead of a file. The value is never stored in
# Terraform code or tfvars — same posture as db_app_password above.
#
# Deployment wiring (documented, NOT enabled here — see deploy/README.md,
# "Owner temporary password"):
#   Cloud Run injects this secret's "latest" version as the OWNER_TEMP_PASSWORD
#   env on the migrate/seed step (the step that runs docker/seed-owner.sql),
#   using the same secret_env mechanism services.tf uses for DB_PASSWORD
#   (see local.common_secret_env). The owner rotates it at first login — it is a
#   one-time temporary credential.
resource "google_secret_manager_secret" "owner_temp_password" {
  count     = var.seed_owner_secret ? 1 : 0
  project   = var.project_id
  secret_id = "owner-temp-password-${var.env}"
  labels    = local.labels

  replication {
    auto {}
  }

  depends_on = [google_project_service.apis]
}

# Bootstrap value — supplied at apply time by CI/operator from a secure source;
# NEVER committed and NEVER hardcoded. Only consumed when seed_owner_secret=true.
variable "owner_temp_password" {
  description = "One-time owner temporary password for a SHARED env. Injected at apply time from a secure source; NEVER committed. Consumed only when seed_owner_secret=true; the owner rotates it at first login."
  type        = string
  default     = ""
  sensitive   = true
}

resource "google_secret_manager_secret_version" "owner_temp_password" {
  count       = var.seed_owner_secret ? 1 : 0
  secret      = google_secret_manager_secret.owner_temp_password[0].id
  secret_data = var.owner_temp_password

  lifecycle {
    # Fail fast: never create an empty owner-password secret. Terraform requires
    # a real value at apply time when the shared-env toggle is on.
    precondition {
      condition     = var.owner_temp_password != ""
      error_message = "owner_temp_password must be provided from a secure source when seed_owner_secret = true; never commit the value."
    }
    # After the owner rotates it at first login (value then managed out of band),
    # ignore drift so a later apply never clobbers the live value.
    ignore_changes = [secret_data]
  }
}

# Additional service secrets (FCM key, CONTPAQi creds, etc.) follow the same
# pattern: one google_secret_manager_secret container here, value written out of
# band, referenced by services via the module's secret_env.
