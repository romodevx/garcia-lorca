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
  count     = var.use_cloud_sql ? 1 : 0
  project   = var.project_id
  secret_id = "garcia-lorca-db-app-password-${var.env}"
  labels    = local.labels

  replication {
    auto {}
  }

  depends_on = [google_project_service.apis]
}

# GENERATED, not supplied. An operator-supplied password that defaulted to ""
# produced either an empty credential or a permanent split-brain: the value
# written out of band afterwards became Cloud Run's "latest" while Postgres kept
# the one Terraform had set, and ignore_changes meant no apply ever reconciled
# them. Generating it here keeps exactly one source of truth.
# Rotate with: terraform apply -replace=random_password.db_app
resource "random_password" "db_app" {
  count   = var.use_cloud_sql ? 1 : 0
  length  = 32
  special = false # avoids quoting hazards in DSNs and shell one-liners
}

resource "google_secret_manager_secret_version" "db_app_password" {
  count       = var.use_cloud_sql ? 1 : 0
  secret      = google_secret_manager_secret.db_app_password[0].id
  secret_data = random_password.db_app[0].result
}

# ---- DATABASE_URL (external Postgres, when use_cloud_sql = false) ----------
# pkg/dbdsn.Resolve() reads DATABASE_URL before assembling the Cloud SQL socket
# DSN, so this is the whole switch: no service code changes.
resource "google_secret_manager_secret" "database_url" {
  count     = var.use_cloud_sql ? 0 : 1
  project   = var.project_id
  secret_id = "garcia-lorca-database-url-${var.env}"
  labels    = local.labels

  replication {
    auto {}
  }

  depends_on = [google_project_service.apis]
}

resource "google_secret_manager_secret_version" "database_url" {
  count       = var.use_cloud_sql ? 0 : 1
  secret      = google_secret_manager_secret.database_url[0].id
  secret_data = var.database_url

  lifecycle {
    precondition {
      condition     = var.database_url != ""
      error_message = "database_url must be provided when use_cloud_sql = false; inject it at apply time (TF_VAR_database_url=...) and never commit it."
    }
    ignore_changes = [secret_data]
  }
}

# ---- iam token signing key ------------------------------------------------
# iam REFUSES TO BOOT without this outside a local environment. It used to fall
# back to a random per-process key, which under scale-to-zero silently
# invalidated every session each time the instance slept. See
# services/iam/cmd/main.go jwtSecret().
resource "google_secret_manager_secret" "iam_jwt_secret" {
  project   = var.project_id
  secret_id = "garcia-lorca-iam-jwt-secret-${var.env}"
  labels    = local.labels

  replication {
    auto {}
  }

  depends_on = [google_project_service.apis]
}

resource "random_password" "iam_jwt" {
  count   = var.iam_jwt_secret == "" ? 1 : 0
  length  = 64
  special = false
}

resource "google_secret_manager_secret_version" "iam_jwt_secret" {
  secret      = google_secret_manager_secret.iam_jwt_secret.id
  secret_data = var.iam_jwt_secret != "" ? var.iam_jwt_secret : random_password.iam_jwt[0].result

  lifecycle {
    # Rotating this logs every user out, so never let a re-apply do it by
    # accident. Deliberate rotation: terraform apply -replace=random_password.iam_jwt
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

# ---- SMTP relay password ---------------------------------------------------
# This one IS a secret, unlike the identity-provider web key: it can send mail
# as the school. It goes through Secret Manager and reaches only iam, mounted as
# secret_env rather than a plain variable — a plain one lands in the Cloud Run
# revision spec (readable with run.viewer), in terraform plan output, and in
# remote state.
#
# The secret exists even when empty: a container that mounts a missing secret
# fails to start, and "no relay configured yet" has to remain a bootable state.
resource "google_secret_manager_secret" "smtp_password" {
  project   = var.project_id
  secret_id = "garcia-lorca-smtp-password-${var.env}"
  labels    = local.labels

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "smtp_password" {
  secret = google_secret_manager_secret.smtp_password.id
  # Empty is the expected state until the school hands over a mailbox. The
  # mailer treats an empty password as "no relay" and falls back to having the
  # identity provider send its own link.
  secret_data = var.smtp_password
}

# ---- Expo push access token (comms) -----------------------------------------
# Optional: Expo accepts pushes without it until the project turns on "enhanced
# push security". The container is created empty and costs nothing; the value is
# written out of band, and only then does var.expo_access_token_enabled mount it
# into comms (services.tf comms_secret_env). No value ever lives in Terraform.
#   printf '%s' "$TOKEN" | gcloud secrets versions add \
#     garcia-lorca-expo-access-token-dev --data-file=-
resource "google_secret_manager_secret" "expo_access_token" {
  project   = var.project_id
  secret_id = "garcia-lorca-expo-access-token-${var.env}"
  labels    = local.labels

  replication {
    auto {}
  }

  depends_on = [google_project_service.apis]
}

# ---- Level-deletion token key (gateway, ADR 0021) --------------------------
# HMAC key that signs the level-deletion preview token. It MUST be the same on
# every gateway instance: without it each instance falls back to a per-process
# random key, so a preview issued by one instance reads as PREVIEW_STALE on
# another — random, unreproducible failures behind Cloud Run load balancing.
# services/gateway/internal/config/deletion_key.go (>= 32 bytes).
resource "google_secret_manager_secret" "level_deletion_token_key" {
  project   = var.project_id
  secret_id = "garcia-lorca-level-deletion-token-key-${var.env}"
  labels    = local.labels

  replication {
    auto {}
  }

  depends_on = [google_project_service.apis]
}

resource "random_password" "level_deletion_token_key" {
  length  = 64
  special = false
}

resource "google_secret_manager_secret_version" "level_deletion_token_key" {
  secret      = google_secret_manager_secret.level_deletion_token_key.id
  secret_data = random_password.level_deletion_token_key.result

  lifecycle {
    # Rotating only invalidates previews in flight (the director re-opens the
    # dialog), but keep it deliberate:
    # terraform apply -replace=random_password.level_deletion_token_key
    ignore_changes = [secret_data]
  }
}
