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
    # Selects the real Cloud Pub/Sub transport (vs the dev log bus) for every
    # service that publishes or consumes events. Harmless for services that use
    # neither — the var is only read by pkg/pubsub/gcp.*FromEnv. See pubsub.tf.
    PUBSUB_PROJECT_ID = var.project_id
  }

  # The app DB password is read from Secret Manager at runtime, never inlined.
  common_secret_env = {
    DB_PASSWORD = {
      secret_id = google_secret_manager_secret.db_app_password.secret_id
      version   = "latest"
    }
  }

  # Gateway → internal service wiring. ENABLED_MODULES is the authoritative
  # module-visibility list the gateway enforces (default-deny); each *_BASE_URL
  # points at the internal service's Cloud Run URL. Mirrors docker-compose.yml
  # and the env the gateway reads (services/gateway/cmd/main.go).
  gateway_env = merge(local.common_env, {
    ENABLED_MODULES     = "iam,gateway,students,config,grades,finance,attendance,comms"
    IAM_BASE_URL        = module.iam.uri
    STUDENTS_BASE_URL   = module.students.uri
    CONFIG_BASE_URL     = module.config.uri
    GRADES_BASE_URL     = module.grades.uri
    FINANCE_BASE_URL    = module.finance.uri
    ATTENDANCE_BASE_URL = module.attendance.uri
    COMMS_BASE_URL      = module.comms.uri
  })

  # Internal-ingress services the gateway (and other services) call as the
  # runtime SA. Each needs roles/run.invoker granted to that SA — cheaper and
  # simpler than a Serverless VPC connector. See internal_invoker below.
  internal_services = {
    iam        = module.iam.service_name
    students   = module.students.service_name
    config     = module.config.service_name
    finance    = module.finance.service_name
    grades     = module.grades.service_name
    attendance = module.attendance.service_name
    comms      = module.comms.service_name
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

  # Warm by default (prod); dev tfvars flips these to scale-to-zero.
  min_instances     = var.gateway_min_instances
  cpu_idle          = var.gateway_cpu_idle
  startup_cpu_boost = var.gateway_startup_cpu_boost
  concurrency       = 200
  max_instances     = var.gateway_max_instances

  ingress               = "INGRESS_TRAFFIC_ALL"
  allow_unauthenticated = true # the only public entry point

  cloudsql_instances = [local.db_connection_name]
  # gateway_env adds ENABLED_MODULES + one *_BASE_URL per internal service.
  env        = local.gateway_env
  secret_env = local.common_secret_env

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

  # Warm by default (prod); dev tfvars flips these to scale-to-zero.
  min_instances     = var.iam_min_instances
  cpu_idle          = var.iam_cpu_idle
  startup_cpu_boost = var.iam_startup_cpu_boost
  concurrency       = 120
  max_instances     = 10

  ingress = "INGRESS_TRAFFIC_INTERNAL_ONLY" # reached only via the gateway

  cloudsql_instances = [local.db_connection_name]
  env                = local.common_env
  secret_env         = local.common_secret_env

  depends_on = [google_project_service.apis]
}

# ---- SCALE-TO-ZERO internal services --------------------------------------
# The 8 REAL services are: gateway, iam (warm above) + students, config,
# finance, grades, attendance, comms (scale-to-zero below). The 4 stub services
# (billing, scholarships, docs, uniforms) are NOT deployed. All internal
# services share the same profile: min=0, cpu_idle=true, internal-only ingress,
# capped max. See deploy/COST-AND-DEPLOY.md §C3.
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
  max_instances     = 3

  ingress = "INGRESS_TRAFFIC_INTERNAL_ONLY"

  cloudsql_instances = [local.db_connection_name]
  env                = local.common_env
  secret_env         = local.common_secret_env

  depends_on = [google_project_service.apis]
}

# ---- config: academic catalog (M04), SCALE-TO-ZERO ------------------------
module "config" {
  source = "./modules/cloud_run"

  name                  = "config"
  project_id            = var.project_id
  region                = var.region
  image                 = lookup(local.image, "config", "${local.registry_path}/config:${var.image_tag}")
  service_account_email = google_service_account.runtime.email

  min_instances     = 0
  cpu_idle          = true
  startup_cpu_boost = false
  concurrency       = 80
  max_instances     = 3

  ingress = "INGRESS_TRAFFIC_INTERNAL_ONLY"

  cloudsql_instances = [local.db_connection_name]
  env                = local.common_env
  secret_env         = local.common_secret_env

  depends_on = [google_project_service.apis]
}

# ---- finance: adeudos (M05), SCALE-TO-ZERO --------------------------------
module "finance" {
  source = "./modules/cloud_run"

  name                  = "finance"
  project_id            = var.project_id
  region                = var.region
  image                 = lookup(local.image, "finance", "${local.registry_path}/finance:${var.image_tag}")
  service_account_email = google_service_account.runtime.email

  min_instances     = 0
  cpu_idle          = true
  startup_cpu_boost = false
  concurrency       = 80
  max_instances     = 3

  ingress = "INGRESS_TRAFFIC_INTERNAL_ONLY"

  cloudsql_instances = [local.db_connection_name]
  env                = local.common_env
  secret_env         = local.common_secret_env

  depends_on = [google_project_service.apis]
}

# ---- grades: grade capture (M07), SCALE-TO-ZERO ---------------------------
module "grades" {
  source = "./modules/cloud_run"

  name                  = "grades"
  project_id            = var.project_id
  region                = var.region
  image                 = lookup(local.image, "grades", "${local.registry_path}/grades:${var.image_tag}")
  service_account_email = google_service_account.runtime.email

  min_instances     = 0
  cpu_idle          = true
  startup_cpu_boost = false
  concurrency       = 80
  max_instances     = 3

  ingress = "INGRESS_TRAFFIC_INTERNAL_ONLY"

  cloudsql_instances = [local.db_connection_name]
  env                = local.common_env
  secret_env         = local.common_secret_env

  depends_on = [google_project_service.apis]
}

# ---- attendance: daily pass list (M06), SCALE-TO-ZERO ---------------------
module "attendance" {
  source = "./modules/cloud_run"

  name                  = "attendance"
  project_id            = var.project_id
  region                = var.region
  image                 = lookup(local.image, "attendance", "${local.registry_path}/attendance:${var.image_tag}")
  service_account_email = google_service_account.runtime.email

  min_instances     = 0
  cpu_idle          = true
  startup_cpu_boost = false
  concurrency       = 80
  max_instances     = 3

  ingress = "INGRESS_TRAFFIC_INTERNAL_ONLY"

  cloudsql_instances = [local.db_connection_name]
  env                = local.common_env
  secret_env         = local.common_secret_env

  depends_on = [google_project_service.apis]
}

# ---- comms: notifications + Pub/Sub consumer (M08), SCALE-TO-ZERO ---------
# comms is BOTH a Connect server and a Pub/Sub subscriber (see
# services/comms/cmd/main.go). The subscriber starts because common_env sets
# PUBSUB_PROJECT_ID; it consumes the comms-* subscriptions pre-created in
# pubsub.tf. min=0 is fine — push is a courtesy on top of a durable feed, and a
# cold start on the first event only defers delivery, it does not drop it.
module "comms" {
  source = "./modules/cloud_run"

  name                  = "comms"
  project_id            = var.project_id
  region                = var.region
  image                 = lookup(local.image, "comms", "${local.registry_path}/comms:${var.image_tag}")
  service_account_email = google_service_account.runtime.email

  min_instances     = 0
  cpu_idle          = true
  startup_cpu_boost = false
  concurrency       = 80
  max_instances     = 3

  ingress = "INGRESS_TRAFFIC_INTERNAL_ONLY"

  cloudsql_instances = [local.db_connection_name]
  env                = local.common_env
  secret_env         = local.common_secret_env

  depends_on = [google_project_service.apis]
}

# ---- Service-to-service auth ----------------------------------------------
# Internal-ingress services accept only authenticated callers. The gateway (and
# services that call each other, e.g. grades→config) run as the runtime SA, so
# granting that SA roles/run.invoker on each internal service lets those calls
# through. This is the cheap alternative to a Serverless VPC connector.
resource "google_cloud_run_v2_service_iam_member" "internal_invoker" {
  for_each = local.internal_services

  project  = var.project_id
  location = var.region
  name     = each.value
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.runtime.email}"
}
