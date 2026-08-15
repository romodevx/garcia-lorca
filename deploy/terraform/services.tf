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

  # Spoke-to-spoke wiring. Non-gateway services also dial their siblings as the
  # runtime SA; without an injected *_BASE_URL each falls back to
  # envOr("X_BASE_URL", "http://localhost:PORT") and self-dials localhost on
  # Cloud Run, so every cross-service RPC fails. Each local mirrors gateway_env:
  # common_env + only the sibling URL(s) that service's cmd/main.go reads. config
  # and iam are leaves (no sibling deps) and keep common_env directly.
  students_env = merge(local.common_env, {
    CONFIG_BASE_URL = module.config.uri
    IAM_BASE_URL    = module.iam.uri
  })

  finance_env = merge(local.common_env, {
    STUDENTS_BASE_URL = module.students.uri
    CONFIG_BASE_URL   = module.config.uri
  })

  grades_env = merge(local.common_env, {
    CONFIG_BASE_URL   = module.config.uri
    STUDENTS_BASE_URL = module.students.uri
  })

  attendance_env = merge(local.common_env, {
    STUDENTS_BASE_URL = module.students.uri
    CONFIG_BASE_URL   = module.config.uri
  })

  # comms reads STUDENTS_BASE_URL (guardian lookup) and pins COMMS_TENANT_ID to
  # the same default tenant UUID every service falls back to (services/*/cmd/main.go
  # defaultTenant), so the consumed-event tenant can never silently diverge.
  comms_env = merge(local.common_env, {
    STUDENTS_BASE_URL = module.students.uri
    COMMS_TENANT_ID   = "00000000-0000-0000-0000-000000000001"
    # Authenticated PUSH mode: comms skips the Pub/Sub pull loop and serves the
    # POST /pubsub receiver instead. SAFE with min=0/cpu_idle=true because
    # Pub/Sub wakes comms on each delivery (see pubsub.tf push_config). Without
    # this flag comms would pull, which a scaled-to-zero instance cannot do.
    PUBSUB_PUSH_MODE = "true"
    # Defense-in-depth over run.invoker: the /pubsub handler enforces that the
    # caller's OIDC token email equals this push SA (main.tf), so no other
    # run.invoker holder can forge events. Enforced only when non-empty;
    # local docker-compose leaves it unset (check skipped).
    PUBSUB_PUSH_SA_EMAIL = google_service_account.pubsub_push.email
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
  env                = local.students_env
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
  env                = local.finance_env
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
  env                = local.grades_env
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
  env                = local.attendance_env
  secret_env         = local.common_secret_env

  depends_on = [google_project_service.apis]
}

# ---- comms: notifications + Pub/Sub consumer (M08), SCALE-TO-ZERO ---------
# comms is BOTH a Connect server and a Pub/Sub PUSH receiver (see
# services/comms/cmd/main.go). Pub/Sub POSTs each event to /pubsub, waking comms
# from scale-to-zero, so min=0/cpu_idle=true stays SAFE (no pull loop to keep
# alive — see comms_env PUBSUB_PUSH_MODE and the push_config subscriptions in
# pubsub.tf).
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

  # ALL (not INTERNAL_ONLY) because Pub/Sub push originates from Google's public
  # range, not the VPC. This is NOT public exposure: allow_unauthenticated stays
  # false, so every caller must present a valid OIDC token carrying run.invoker —
  # the push SA for /pubsub (main.tf) and the runtime SA for gateway→comms RPCs
  # (internal_invoker below). Unauthenticated public traffic is still rejected.
  ingress = "INGRESS_TRAFFIC_ALL"

  cloudsql_instances = [local.db_connection_name]
  env                = local.comms_env
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
