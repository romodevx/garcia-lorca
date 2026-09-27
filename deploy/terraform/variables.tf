variable "project_id" {
  description = "Target GCP project id (one project per environment is recommended)."
  type        = string
}

variable "region" {
  description = "Single deployment region for Cloud Run, Cloud SQL and Artifact Registry."
  type        = string
  default     = "us-central1"
}

variable "env" {
  description = "Environment name (dev, staging, prod). Drives flag/config state and resource labels."
  type        = string
  validation {
    condition     = contains(["dev", "staging", "prod"], var.env)
    error_message = "env must be one of: dev, staging, prod."
  }
}

variable "artifact_registry_repo" {
  description = "Artifact Registry Docker repository id that holds service images."
  type        = string
  default     = "garcia-lorca"
}

# ---- Container image tags -------------------------------------------------
# Per-environment flag state is configuration (IaC), not code. Image tags are
# likewise configuration: CI writes the resolved tag into the env tfvars file.
variable "image_tag" {
  description = "Default image tag applied to every service unless overridden in image_tags."
  type        = string
  default     = "latest"
}

variable "image_tags" {
  description = "Optional per-service image tag overrides, e.g. { gateway = \"abc123\" }."
  type        = map(string)
  default     = {}
}

# ---- Owner temporary password (shared envs) -------------------------------
# Off by default so local-parity applies stay inert. Set true in a SHARED env's
# tfvars to have Terraform manage the owner-temp-password secret container +
# version (value from var.owner_temp_password, defined in secrets.tf). See
# secrets.tf and deploy/README.md ("Owner temporary password").
variable "seed_owner_secret" {
  description = "When true, create the owner-temp-password Secret Manager secret + version for a shared env (value from var.owner_temp_password). Default false keeps local/dev applies inert."
  type        = bool
  default     = false
}

# ---- Database backend -----------------------------------------------------
# The demo environment runs on an external Postgres (Neon free tier) because
# Cloud SQL cannot scale to zero and was the ENTIRE resting cost of this
# environment (~$9.70/mo). Switching costs nothing in code: pkg/dbdsn.Resolve()
# reads DATABASE_URL first and only assembles the Cloud SQL socket DSN when it is
# absent. Prod keeps Cloud SQL for parity and operational control.
variable "use_cloud_sql" {
  description = "Provision Cloud SQL and wire services to it via the /cloudsql socket. false = supply DATABASE_URL from Secret Manager instead (external Postgres, e.g. Neon)."
  type        = bool
  default     = true
}

variable "database_url" {
  description = "Full Postgres connection URL, used only when use_cloud_sql=false. Injected at apply time from a secure source; NEVER committed."
  type        = string
  default     = ""
  sensitive   = true
}

variable "iam_jwt_secret" {
  description = "HMAC signing key for iam access/refresh tokens. Injected at apply time; NEVER committed. Required in every shared env — iam refuses to boot without it (see services/iam/cmd/main.go)."
  type        = string
  default     = ""
  sensitive   = true
}

variable "web_origin" {
  description = "Browser origin(s) the gateway allows for CORS, comma-separated. Deliberately a plain string and NOT module.web.uri: referencing the web service here while web reads the gateway URI would make module.gateway and module.web depend on each other, and Terraform refuses the cycle. With the same-origin proxy the browser never makes a cross-origin call, so this exists for native mobile clients. Fill in after the first apply."
  type        = string
  default     = ""
}

# ---- Cloud SQL ------------------------------------------------------------
variable "db_tier" {
  description = "Cloud SQL machine tier. Start small (db-g1-small) and scale up later."
  type        = string
  default     = "db-g1-small"
}

variable "db_name" {
  description = "Application database name."
  type        = string
  default     = "garcia_lorca"
}

variable "db_app_user" {
  description = "Application DB role. NOT the instance owner and NOT BYPASSRLS — RLS is enforced against it."
  type        = string
  default     = "app"
}

variable "db_availability_type" {
  description = "ZONAL (cheap, dev) or REGIONAL (HA, prod)."
  type        = string
  default     = "ZONAL"
}

variable "db_point_in_time_recovery_enabled" {
  description = "Cloud SQL point-in-time recovery (WAL archiving). Default ON (prod-safe); set false for dev — daily backups suffice and PITR adds storage cost. See deploy/COST-AND-DEPLOY.md §C1."
  type        = bool
  default     = true
}

variable "db_backup_retained_count" {
  description = "Number of automated daily backups Cloud SQL retains. Prod default 7; dev uses 2-3 to trim storage."
  type        = number
  default     = 7
}

# ---- Cloud Run cost profile (warm vs scale-to-zero) -----------------------
# gateway + iam default to WARM (min=1, CPU always allocated, boost on) so prod
# has no cold starts. dev overrides these to scale-to-zero in its tfvars — the
# single biggest cost lever (~$95-100/mo). See deploy/COST-AND-DEPLOY.md §C2.
variable "gateway_min_instances" {
  description = "gateway minimum instances. 1 = always warm (prod); 0 = scale-to-zero (dev)."
  type        = number
  default     = 1
}

variable "gateway_max_instances" {
  description = "gateway maximum instances (cost ceiling). Dev caps at 1 because the login rate limiter is in-process/per-instance."
  type        = number
  default     = 20
}

variable "gateway_cpu_idle" {
  description = "gateway CPU allocation. false = CPU always allocated (warm, prod); true = CPU billed only per request (scale-to-zero, dev)."
  type        = bool
  default     = false
}

variable "gateway_startup_cpu_boost" {
  description = "gateway startup CPU boost. On for warm/prod to cut tail latency; off for dev."
  type        = bool
  default     = true
}

variable "iam_min_instances" {
  description = "iam minimum instances. 1 = always warm (prod); 0 = scale-to-zero (dev)."
  type        = number
  default     = 1
}

variable "iam_cpu_idle" {
  description = "iam CPU allocation. false = CPU always allocated (warm, prod); true = CPU billed only per request (scale-to-zero, dev)."
  type        = bool
  default     = false
}

variable "iam_startup_cpu_boost" {
  description = "iam startup CPU boost. On for warm/prod; off for dev."
  type        = bool
  default     = true
}

# ---- Budget ---------------------------------------------------------------
variable "billing_account_id" {
  description = "Billing account id the budget is attached to (format XXXXXX-XXXXXX-XXXXXX)."
  type        = string
}

variable "monthly_budget_amount" {
  description = "Monthly budget ceiling in budget_currency. Alerts fire at thresholds below."
  type        = number
  default     = 200
}

variable "budget_currency" {
  description = "Currency code for the budget amount. MUST match the billing account's own currency — the Budget API rejects a mismatch with a bare \"Request contains an invalid argument\" that never mentions currency, which is a genuinely awful hour to spend. Check yours in the console under Billing."
  type        = string
  default     = "MXN"
}

variable "budget_alert_emails" {
  description = "Emails notified when budget thresholds are crossed."
  type        = list(string)
  default     = []
}

# ---- M15 billing schedule ---------------------------------------------------

variable "billing_schedule_enabled" {
  description = "Create the Cloud Scheduler job that runs the monthly billing. Turning module_tuition off does NOT stop this — the scheduler calls finance directly, so stopping it is an apply."
  type        = bool
  default     = false
}

variable "billing_cycle_id" {
  description = "The school cycle the scheduled run bills. Deliberately explicit: a job that guessed the cycle would silently bill the wrong school year the day one rolls over. Empty disables the job."
  type        = string
  default     = ""
}

variable "billing_schedule_cron" {
  description = "When the billing run fires. Default: 06:00 on the 1st of each month."
  type        = string
  default     = "0 6 1 * *"
}

variable "billing_schedule_timezone" {
  description = "IANA timezone the cron is read in. A school's due dates are local dates."
  type        = string
  default     = "America/Mexico_City"
}

# ---- Invitations and outgoing mail -----------------------------------------

variable "public_web_url" {
  description = <<-EOT
    Public origin of the web app, with no trailing slash. Invitation links are
    built from it.

    It has no default on purpose. iam falls back to localhost when the variable
    is empty, which in a deployed environment produces a link that looks correct
    and goes nowhere — and neither the person receiving it nor the director
    sending it can tell why.
  EOT
  type        = string
}

variable "smtp_host" {
  description = <<-EOT
    Mail relay for invitations and password resets. EMPTY IS THE EXPECTED STATE
    until the school hands over a mailbox: with no relay, invitations still work
    and fall back to having the identity provider send its own link, and the
    admin screen says which road the message took.

    The destination is the school's own server (mail.ceglorca.com), whose SPF
    already authorises it — no DNS change needed, only a credential.
  EOT
  type        = string
  default     = ""
}

variable "smtp_port" {
  description = "SMTP port. 587 (STARTTLS) unless the relay says otherwise."
  type        = string
  default     = "587"
}

variable "smtp_username" {
  description = "SMTP account. Empty disables our own sending (see smtp_host)."
  type        = string
  default     = ""
}

variable "smtp_password" {
  description = <<-EOT
    SMTP password. Unlike the identity-provider web key, this one IS a secret:
    it can send mail as the school. It belongs in the secret manager and must
    never be committed to a tfvars file.
  EOT
  type        = string
  default     = ""
  sensitive   = true
}

variable "smtp_from" {
  description = "Envelope sender. Defaults to smtp_username when empty."
  type        = string
  default     = ""
}

variable "smtp_from_name" {
  description = <<-EOT
    The name a person actually reads in their inbox list, which matters more for
    delivery and trust than the address does.
  EOT
  type        = string
  default     = "Colegio García Lorca"
}

variable "push_provider" {
  description = <<-EOT
    How comms delivers push notifications: "log" only logs them (the durable
    in-app feed still works), "expo" sends them through the Expo Push Service.
  EOT
  type        = string
  default     = "log"
  validation {
    condition     = contains(["log", "expo"], var.push_provider)
    error_message = "push_provider must be one of: log, expo."
  }
}

variable "expo_access_token_enabled" {
  description = <<-EOT
    Mount the Expo access token secret into comms. Turn on only AFTER a version
    has been written to garcia-lorca-expo-access-token-<env>: a revision that
    references a secret with no version fails to start.
  EOT
  type        = bool
  default     = false
}
