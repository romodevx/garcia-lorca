# Service instantiations. Every service is the SAME cloud_run module; the only
# difference is the cost profile passed in.
#
# Cost lever:
#   - gateway, iam  -> min_instances=1, cpu_idle=false, startup_cpu_boost=true
#                      (always warm; CPU always allocated; faster cold path)
#   - everything else -> min_instances=0, cpu_idle=true (scale-to-zero, CPU
#                        billed per request). High concurrency packs requests.
#
# The full service set is: gateway, iam, students, finance, billing, grades,
# scholarships, attendance, docs, comms, uniforms. Only gateway + iam + students
# are instantiated below as canonical examples; the rest follow the same module
# call with the scale-to-zero profile (copy the "students" block, change name).

locals {
  db_connection_name = google_sql_database_instance.main.connection_name

  # Shared runtime env every service receives.
  common_env = {
    APP_ENV          = var.env
    OTEL_SERVICE_ENV = var.env
    DB_NAME          = var.db_name
    DB_USER          = var.db_app_user
    # Unix-socket path used with the Cloud SQL proxy volume mount.
    DB_SOCKET_DIR = "/cloudsql/${local.db_connection_name}"
  }

  # The app DB password is read from Secret Manager at runtime, never inlined.
  common_secret_env = {
    DB_PASSWORD = {
      secret_id = google_secret_manager_secret.db_app_password.secret_id
      version   = "latest"
    }
  }
}

# ---- gateway: public edge, ALWAYS WARM ------------------------------------
module "gateway" {
  source = "./modules/cloud_run"

  name                  = "gateway"
  project_id            = var.project_id
  region                = var.region
  image                 = lookup(local.image, "gateway", "${local.registry_path}/gateway:${var.image_tag}")
  service_account_email = google_service_account.runtime.email

  min_instances     = 1     # always warm
  cpu_idle          = false # CPU always allocated
  startup_cpu_boost = true
  concurrency       = 200
  max_instances     = 20

  ingress               = "INGRESS_TRAFFIC_ALL"
  allow_unauthenticated = true # the only public entry point

  cloudsql_instances = [local.db_connection_name]
  env                = local.common_env
  secret_env         = local.common_secret_env

  depends_on = [google_project_service.apis]
}

# ---- iam: auth/identity, ALWAYS WARM --------------------------------------
module "iam" {
  source = "./modules/cloud_run"

  name                  = "iam"
  project_id            = var.project_id
  region                = var.region
  image                 = lookup(local.image, "iam", "${local.registry_path}/iam:${var.image_tag}")
  service_account_email = google_service_account.runtime.email

  min_instances     = 1     # always warm
  cpu_idle          = false # CPU always allocated
  startup_cpu_boost = true
  concurrency       = 120
  max_instances     = 10

  ingress = "INGRESS_TRAFFIC_INTERNAL_ONLY" # reached only via the gateway

  cloudsql_instances = [local.db_connection_name]
  env                = local.common_env
  secret_env         = local.common_secret_env

  depends_on = [google_project_service.apis]
}

# ---- students: representative SCALE-TO-ZERO service -----------------------
# All remaining services (finance, grades, scholarships, attendance, docs,
# comms, uniforms, billing) are deployed by duplicating this block with a new
# `name` — same module, same scale-to-zero profile.
module "students" {
  source = "./modules/cloud_run"

  name                  = "students"
  project_id            = var.project_id
  region                = var.region
  image                 = lookup(local.image, "students", "${local.registry_path}/students:${var.image_tag}")
  service_account_email = google_service_account.runtime.email

  min_instances     = 0    # scale-to-zero
  cpu_idle          = true # CPU billed only during requests
  startup_cpu_boost = false
  concurrency       = 80
  max_instances     = 10

  ingress = "INGRESS_TRAFFIC_INTERNAL_ONLY"

  cloudsql_instances = [local.db_connection_name]
  env                = local.common_env
  secret_env         = local.common_secret_env

  depends_on = [google_project_service.apis]
}
