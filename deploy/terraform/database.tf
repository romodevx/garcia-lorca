# Cloud SQL for PostgreSQL — single instance to start (cost-conscious).
# Scale by raising db_tier and switching availability_type to REGIONAL for prod.

resource "google_sql_database_instance" "main" {
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
      point_in_time_recovery_enabled = true
      start_time                     = "07:00"
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
  project  = var.project_id
  instance = google_sql_database_instance.main.name
  name     = var.db_name
}

# Application DB role.
#
# IMPORTANT — RLS posture (see CLAUDE.md rule 5 and rules/database-rls.md):
#   * This role is NOT the instance owner.
#   * It must NOT have the BYPASSRLS attribute.
#   * Cloud SQL does not grant SUPERUSER/BYPASSRLS to user-created roles, so this
#     role is already constrained; RLS policies apply to it. The backend sets
#     SET LOCAL app.current_* per transaction (pkg/authctx.WithUserContext).
#
# Terraform creates the login role with a password; the privilege grants and the
# explicit "ALTER ROLE app NOBYPASSRLS" intent are applied by the SQL setup
# snippet in db/setup_app_role.sql (run once by an admin/owner connection),
# because Terraform's google_sql_user does not manage GRANT/role attributes.
resource "google_sql_user" "app" {
  project  = var.project_id
  instance = google_sql_database_instance.main.name
  name     = var.db_app_user

  # Password comes from Secret Manager, not from tfvars or code.
  password = google_secret_manager_secret_version.db_app_password.secret_data
}
