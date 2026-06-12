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

# Additional service secrets (FCM key, CONTPAQi creds, etc.) follow the same
# pattern: one google_secret_manager_secret container here, value written out of
# band, referenced by services via the module's secret_env.
