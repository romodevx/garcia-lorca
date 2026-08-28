# Cloud SQL for PostgreSQL — single instance to start (cost-conscious).
# Scale by raising db_tier and switching availability_type to REGIONAL for prod.

resource "google_sql_database_instance" "main" {
  count = var.use_cloud_sql ? 1 : 0

  project          = var.project_id
  name             = "garcia-lorca-${var.env}"
  region           = var.region
  database_version = "POSTGRES_16"

  # deletion_protection guards prod; relax via tfvars only in throwaway envs.
  deletion_protection = var.env == "prod"

  settings {
    tier              = var.db_tier
    availability_type = var.db_availability_type
    disk_type         = "PD_SSD"
    disk_size         = 10
    disk_autoresize   = true # auto storage increase

    backup_configuration {
      enabled                        = true
      point_in_time_recovery_enabled = var.db_point_in_time_recovery_enabled
      start_time                     = "07:00"
      backup_retention_settings {
        retained_backups = var.db_backup_retained_count
      }
    }

    ip_configuration {
      # No public IP. Access is via the Cloud SQL proxy / private path only.
      ipv4_enabled = false
      # private_network = "projects/<project>/global/networks/<vpc>"  # set when VPC exists
    }

    insights_config {
      query_insights_enabled = true
    }

    user_labels = local.labels
  }

  depends_on = [google_project_service.apis]
}

resource "google_sql_database" "app" {
  count = var.use_cloud_sql ? 1 : 0

  project  = var.project_id
  instance = google_sql_database_instance.main[0].name
  name     = var.db_name
}

# Application DB role.
#
# IMPORTANT — RLS posture (see CLAUDE.md rule 5 and rules/database-rls.md):
#   * This role is NOT the instance owner.
#   * It must NOT have the BYPASSRLS attribute.
#   * Cloud SQL does not grant SUPERUSER/BYPASSRLS to user-created roles, so this
#     role is already constrained; RLS policies apply to it. The backend binds
#     app.current_tenant per transaction (set_config(..., true) = SET LOCAL) via
#     pkg/authctx.WithTenantContext.
#
# Terraform creates the login role with a password; the privilege grants and the
# explicit "ALTER ROLE app NOBYPASSRLS" intent are applied by the SQL setup
# snippet in db/setup_app_role.sql (run once by an admin/owner connection),
# because Terraform's google_sql_user does not manage GRANT/role attributes.
resource "google_sql_user" "app" {
  count = var.use_cloud_sql ? 1 : 0

  project  = var.project_id
  instance = google_sql_database_instance.main[0].name
  name     = var.db_app_user

  # The password is GENERATED AT APPLY TIME and flows one way: random_password ->
  # Secret Manager version -> this user. It used to read the secret version whose
  # data came from var.db_app_password (default ""), which meant either an empty
  # password or, worse, an operator writing a value out of band afterwards: Cloud
  # Run then mounted "latest" (the new password) while Postgres kept the old one,
  # every service died on "password authentication failed", and ignore_changes
  # guaranteed no later apply would ever reconcile them.
  # Rotating later is: terraform apply -replace=random_password.db_app
  password = google_secret_manager_secret_version.db_app_password[0].secret_data
}
