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
  # Empty when running against an external Postgres (use_cloud_sql = false).
  db_connection_name = var.use_cloud_sql ? google_sql_database_instance.main[0].connection_name : ""
  cloudsql_instances = var.use_cloud_sql ? [local.db_connection_name] : []

  # Shared runtime env every service receives. The DB_* half only applies to the
  # Cloud SQL socket path; with an external Postgres the whole DSN arrives as the
  # DATABASE_URL secret, which pkg/dbdsn.Resolve() reads first.
  common_env = merge(
    {
      APP_ENV          = var.env
      OTEL_SERVICE_ENV = var.env
      # Selects the real Cloud Pub/Sub transport (vs the dev log bus) for every
      # service that publishes or consumes events. Harmless for services that use
      # neither — the var is only read by pkg/pubsub/gcp.*FromEnv. See pubsub.tf.
      PUBSUB_PROJECT_ID = var.project_id
      # db-f1-micro allows ~25 connections total. Ten services that could each
      # run several instances would exhaust that, and the failure is an
      # intermittent "remaining connection slots are reserved" rather than
      # anything obvious. pkg/dbpool defaults to 3; trim it here for the small
      # tier. Raise it in an environment that has the headroom.
      DB_MAX_OPEN_CONNS = "2"
      DB_MAX_IDLE_CONNS = "1"
    },
    var.use_cloud_sql ? {
      DB_NAME = var.db_name
      DB_USER = var.db_app_user
      # Unix-socket path used with the Cloud SQL proxy volume mount.
      DB_SOCKET_DIR = "/cloudsql/${local.db_connection_name}"
    } : {}
  )

  # Database credentials are read from Secret Manager at runtime, never inlined.
  common_secret_env = var.use_cloud_sql ? {
    DB_PASSWORD = {
      secret_id = google_secret_manager_secret.db_app_password[0].secret_id
      version   = "latest"
    }
    } : {
    DATABASE_URL = {
      secret_id = google_secret_manager_secret.database_url[0].secret_id
      version   = "latest"
    }
  }

  # iam additionally needs its token signing key. Without it iam refuses to boot
  # outside a local environment — deliberately, because the previous random
  # fallback silently invalidated every session on each scale-to-zero restart.
  iam_secret_env = merge(local.common_secret_env, {
    IAM_JWT_SECRET = {
      secret_id = google_secret_manager_secret.iam_jwt_secret.secret_id
      version   = "latest"
    }
  })

  # Gateway → internal service wiring. ENABLED_MODULES is the authoritative
  # module-visibility list the gateway enforces (default-deny); each *_BASE_URL
  # points at the internal service's Cloud Run URL. Mirrors docker-compose.yml
  # and the env the gateway reads (services/gateway/cmd/main.go).
  # iam is the only service that talks to the identity provider, so the setting
  # lives here and not in common_env. EMPTY unless the environment provisions
  # Identity Platform: with no project named, iam's verifier stays nil, the
  # provider sign-in path refuses, and the password login is untouched. Turning
  # this on is therefore a deliberate, reversible act.
  iam_env = merge(local.common_env, {
    IDENTITY_PLATFORM_PROJECT = var.enable_identity_platform ? var.project_id : ""
    # Sending invitations. Without it the sign-in path is unaffected and only
    # inviting refuses — so an environment that provisions the provider but
    # forgets this key degrades to "cannot add people", never to "cannot log in".
    IDENTITY_PLATFORM_API_KEY = var.enable_identity_platform ? var.identity_platform_api_key : ""

    # Where an invitation link points. iam defaults to localhost when this is
    # unset, which in a deployed environment produces a link that looks correct
    # and goes nowhere — the kind of failure the person receiving it cannot
    # diagnose and the director cannot see.
    PUBLIC_WEB_URL = var.public_web_url

    # The mail relay. EMPTY IS THE EXPECTED STATE until the school hands over a
    # mailbox: with none, invitations still work and fall back to having the
    # identity provider send its own link, and the admin screen says so.
    SMTP_HOST      = var.smtp_host
    SMTP_PORT      = var.smtp_port
    SMTP_USERNAME  = var.smtp_username
    SMTP_PASSWORD  = var.smtp_password
    SMTP_FROM      = var.smtp_from
    SMTP_FROM_NAME = var.smtp_from_name
  })

  gateway_env = merge(local.common_env, {
    # The authoritative module-visibility list (default-deny). `teachers` and
    # `uniforms` were missing: teachers guards /api/v1/teachers +teaching
    # assignments (M14), and uniforms is a real Clean-Architecture service with
    # its own migrations since M12 — not a stub. There is deliberately NO
    # `tuition` entry: M15 rides on module_finance (FLAGS.md).
    ENABLED_MODULES = "iam,gateway,students,config,grades,finance,attendance,comms,teachers,uniforms"
    # CORS allow-list for clients that call the gateway cross-origin. The web app
    # does not: it proxies same-origin. This is a plain variable rather than
    # module.web.uri on purpose — see var.web_origin for the dependency cycle.
    WEB_ORIGIN          = var.web_origin
    UNIFORMS_BASE_URL   = module.uniforms.uri
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
  # students consumes payment-events (a paid inscription is what makes a pupil
  # "inscrito"), so it needs the same push wiring comms and finance have.
  students_env = merge(local.common_env, {
    PUBSUB_PUSH_MODE     = "true"
    PUBSUB_PUSH_SA_EMAIL = google_service_account.pubsub_push.email
    CONFIG_BASE_URL      = module.config.uri
    IAM_BASE_URL         = module.iam.uri
  })

  # finance became a CONSUMER as well as a publisher in M15: StudentEnrolled is
  # what charges a pupil's inscription the moment they take a place in a group.
  # Same push reasoning as comms — a scaled-to-zero instance cannot pull, so
  # Pub/Sub POSTs instead and wakes it (see pubsub.tf student_events_finance).
  finance_env = merge(local.common_env, {
    STUDENTS_BASE_URL    = module.students.uri
    CONFIG_BASE_URL      = module.config.uri
    PUBSUB_PUSH_MODE     = "true"
    PUBSUB_PUSH_SA_EMAIL = google_service_account.pubsub_push.email
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

  # uniforms sells at the counter, which creates a charge in finance by
  # external_ref (ADR 0008), and resolves pupils for the order list.
  uniforms_env = merge(local.common_env, {
    FINANCE_BASE_URL  = module.finance.uri
    STUDENTS_BASE_URL = module.students.uri
    CONFIG_BASE_URL   = module.config.uri
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
    uniforms   = module.uniforms.service_name
  }
}

# ---- gateway: public edge, ALWAYS WARM ------------------------------------
module "gateway" {
  source = "./modules/cloud_run"

  name                  = "gateway"
  project_id            = var.project_id
  environment           = var.env
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

  cloudsql_instances = local.cloudsql_instances
  # gateway_env adds ENABLED_MODULES + one *_BASE_URL per internal service.
  env        = local.gateway_env
  secret_env = local.common_secret_env

  depends_on = [google_project_service.apis]
}

# ---- iam: auth/identity, ALWAYS WARM --------------------------------------
module "iam" {
  source = "./modules/cloud_run"

  name        = "iam"
  project_id  = var.project_id
  environment = var.env
  region      = var.region
  image       = lookup(local.image, "iam", "${local.registry_path}/iam:${var.image_tag}")
  # iam gets its OWN service account, and it is the only service that does.
  #
  # It is the only one that talks to the identity provider, and the permission it
  # needs — writing somebody's password — is the one permission you would least
  # like nine unrelated services to inherit. They all share
  # google_service_account.runtime; iam does not.
  service_account_email = google_service_account.iam_runtime.email

  # Warm by default (prod); dev tfvars flips these to scale-to-zero.
  min_instances     = var.iam_min_instances
  cpu_idle          = var.iam_cpu_idle
  startup_cpu_boost = var.iam_startup_cpu_boost
  concurrency       = 120
  max_instances     = 10

  # NOT internal-only. Cloud Run -> Cloud Run over the *.run.app URL egresses to
  # the public internet unless a VPC connector or Direct VPC egress is
  # configured, and neither is (that is the cost decision). Internal ingress is a
  # NETWORK filter evaluated before authentication, so it would reject every
  # gateway call with a 404 from Cloud Run's frontend, before reaching any code.
  # The real boundary is allow_unauthenticated=false + roles/run.invoker +
  # audience-scoped OIDC (pkg/gcpauth) — exactly what comms already relies on.
  ingress = "INGRESS_TRAFFIC_ALL"

  cloudsql_instances = local.cloudsql_instances
  env                = local.iam_env
  secret_env         = local.iam_secret_env

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
  environment           = var.env
  region                = var.region
  image                 = lookup(local.image, "students", "${local.registry_path}/students:${var.image_tag}")
  service_account_email = google_service_account.runtime.email

  min_instances     = 0    # scale-to-zero
  cpu_idle          = true # CPU billed only during requests
  startup_cpu_boost = false
  concurrency       = 80
  max_instances     = 3

  ingress = "INGRESS_TRAFFIC_ALL" # see the iam block: authenticated, not public

  cloudsql_instances = local.cloudsql_instances
  env                = local.students_env
  secret_env         = local.common_secret_env

  depends_on = [google_project_service.apis]
}

# ---- config: academic catalog (M04), SCALE-TO-ZERO ------------------------
module "config" {
  source = "./modules/cloud_run"

  name                  = "config"
  project_id            = var.project_id
  environment           = var.env
  region                = var.region
  image                 = lookup(local.image, "config", "${local.registry_path}/config:${var.image_tag}")
  service_account_email = google_service_account.runtime.email

  min_instances     = 0
  cpu_idle          = true
  startup_cpu_boost = false
  concurrency       = 80
  max_instances     = 3

  ingress = "INGRESS_TRAFFIC_ALL" # see the iam block: authenticated, not public

  cloudsql_instances = local.cloudsql_instances
  env                = local.common_env
  secret_env         = local.common_secret_env

  depends_on = [google_project_service.apis]
}

# ---- finance: adeudos (M05), SCALE-TO-ZERO --------------------------------
module "finance" {
  source = "./modules/cloud_run"

  name                  = "finance"
  project_id            = var.project_id
  environment           = var.env
  region                = var.region
  image                 = lookup(local.image, "finance", "${local.registry_path}/finance:${var.image_tag}")
  service_account_email = google_service_account.runtime.email

  min_instances     = 0
  cpu_idle          = true
  startup_cpu_boost = false
  concurrency       = 80
  max_instances     = 3

  ingress = "INGRESS_TRAFFIC_ALL" # see the iam block: authenticated, not public

  cloudsql_instances = local.cloudsql_instances
  env                = local.finance_env
  secret_env         = local.common_secret_env

  depends_on = [google_project_service.apis]
}

# ---- grades: grade capture (M07), SCALE-TO-ZERO ---------------------------
module "grades" {
  source = "./modules/cloud_run"

  name                  = "grades"
  project_id            = var.project_id
  environment           = var.env
  region                = var.region
  image                 = lookup(local.image, "grades", "${local.registry_path}/grades:${var.image_tag}")
  service_account_email = google_service_account.runtime.email

  min_instances     = 0
  cpu_idle          = true
  startup_cpu_boost = false
  concurrency       = 80
  max_instances     = 3

  ingress = "INGRESS_TRAFFIC_ALL" # see the iam block: authenticated, not public

  cloudsql_instances = local.cloudsql_instances
  env                = local.grades_env
  secret_env         = local.common_secret_env

  depends_on = [google_project_service.apis]
}

# ---- attendance: daily pass list (M06), SCALE-TO-ZERO ---------------------
module "attendance" {
  source = "./modules/cloud_run"

  name                  = "attendance"
  project_id            = var.project_id
  environment           = var.env
  region                = var.region
  image                 = lookup(local.image, "attendance", "${local.registry_path}/attendance:${var.image_tag}")
  service_account_email = google_service_account.runtime.email

  min_instances     = 0
  cpu_idle          = true
  startup_cpu_boost = false
  concurrency       = 80
  max_instances     = 3

  ingress = "INGRESS_TRAFFIC_ALL" # see the iam block: authenticated, not public

  cloudsql_instances = local.cloudsql_instances
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
  environment           = var.env
  region                = var.region
  image                 = lookup(local.image, "comms", "${local.registry_path}/comms:${var.image_tag}")
  service_account_email = google_service_account.runtime.email

  min_instances     = 0
  cpu_idle          = true
  startup_cpu_boost = false
  concurrency       = 80
  max_instances     = 3

  # ALL, for the same reason as every other service here (see the iam block):
  # Cloud Run -> Cloud Run does not qualify as internal traffic without VPC
  # egress. NOTE: the original comment here blamed Pub/Sub push and was wrong —
  # Pub/Sub IS an allowed internal-ingress source, which is why students and
  # finance receive pushes fine. Either way this is NOT public exposure:
  # allow_unauthenticated stays false, so every caller must present a valid OIDC
  # token carrying run.invoker — the push SA for /pubsub (main.tf) and the
  # runtime SA for gateway->comms RPCs (internal_invoker below).
  ingress = "INGRESS_TRAFFIC_ALL"

  cloudsql_instances = local.cloudsql_instances
  env                = local.comms_env
  secret_env         = local.common_secret_env

  depends_on = [google_project_service.apis]
}

# ---- uniforms: store + inventory (M12), SCALE-TO-ZERO ---------------------
# NOT a stub. 35 Go files, its own go.mod and migrations, ADRs 0007-0010: a full
# Clean-Architecture service with a 7-state delivery machine and FIFO stock
# allocation. deploy/COST-AND-DEPLOY.md listed it among the stubs long after that
# stopped being true, and the web already renders its "Uniformes" tile — so
# leaving it undeployed both dropped a module a school administrator can actually
# touch AND broke the demo seed, which calls /api/v1/uniforms/*.
module "uniforms" {
  source = "./modules/cloud_run"

  name                  = "uniforms"
  project_id            = var.project_id
  environment           = var.env
  region                = var.region
  image                 = lookup(local.image, "uniforms", "${local.registry_path}/uniforms:${var.image_tag}")
  service_account_email = google_service_account.runtime.email

  min_instances     = 0
  cpu_idle          = true
  startup_cpu_boost = false
  concurrency       = 80
  max_instances     = 3

  ingress = "INGRESS_TRAFFIC_ALL" # see the iam block: authenticated, not public

  cloudsql_instances = local.cloudsql_instances
  env                = local.uniforms_env
  secret_env         = local.common_secret_env

  depends_on = [google_project_service.apis]
}

# ---- web: the Next.js front end, PUBLIC, SCALE-TO-ZERO --------------------
# The only thing the browser talks to. It proxies /api/* to the gateway from its
# own origin (apps/web/app/api/[...path]/route.ts) because the auth cookies are
# SameSite=Strict and run.app is on the Public Suffix List, so a browser would
# never send them across two different *.run.app hosts.
#
# No Cloud SQL and no secrets: it holds no credentials and reaches nothing but
# the gateway.
module "web" {
  source = "./modules/cloud_run"

  name                  = "web"
  project_id            = var.project_id
  environment           = var.env
  region                = var.region
  image                 = lookup(local.image, "web", "${local.registry_path}/web:${var.image_tag}")
  service_account_email = google_service_account.runtime.email

  min_instances = 0
  cpu_idle      = true
  # On for the one path a visitor actually experiences. With min_instances=0 the
  # boost is billed only while starting, so it costs ~nothing at rest.
  startup_cpu_boost = true
  concurrency       = 80
  max_instances     = 2

  ingress               = "INGRESS_TRAFFIC_ALL"
  allow_unauthenticated = true # the public entry point for humans

  cloudsql_instances = []
  env = {
    APP_ENV = var.env
    # Read per request by the proxy route handler, so pointing the web at a
    # different gateway is an env update, not an image rebuild.
    GATEWAY_URL = module.gateway.uri
  }
  secret_env = {}

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
