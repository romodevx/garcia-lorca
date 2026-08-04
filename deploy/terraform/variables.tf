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
  description = "Currency code for the budget amount."
  type        = string
  default     = "USD"
}

variable "budget_alert_emails" {
  description = "Emails notified when budget thresholds are crossed."
  type        = list(string)
  default     = []
}
