# Keyless CI/CD from GitHub Actions via Workload Identity Federation.
#
# The alternative is a service-account JSON key pasted into a GitHub secret. That
# key is a long-lived credential to this project sitting in a third party, with no
# expiry and no way to know if it leaked. WIF instead lets GitHub mint a short
# OIDC token that Google exchanges for a scoped, minutes-long access token — and
# only for workflows from THIS repository.
#
# The repository binding below is the security boundary: without it, any GitHub
# repo on earth could exchange a token for access to this project.

variable "github_repository" {
  description = "owner/repo allowed to deploy through Workload Identity Federation. Empty disables the whole CI/CD path."
  type        = string
  default     = ""
}

locals {
  cicd = var.github_repository == "" ? 0 : 1
}

resource "google_iam_workload_identity_pool" "github" {
  count                     = local.cicd
  project                   = var.project_id
  workload_identity_pool_id = "github-${var.env}"
  display_name              = "GitHub Actions (${var.env})"
  description               = "Keyless deploys from GitHub Actions."
}

resource "google_iam_workload_identity_pool_provider" "github" {
  count                              = local.cicd
  project                            = var.project_id
  workload_identity_pool_id          = google_iam_workload_identity_pool.github[0].workload_identity_pool_id
  workload_identity_pool_provider_id = "github"
  display_name                       = "GitHub OIDC"

  attribute_mapping = {
    "google.subject"       = "assertion.sub"
    "attribute.repository" = "assertion.repository"
    "attribute.ref"        = "assertion.ref"
  }

  # REQUIRED. Without a condition Google rejects the provider outright, and
  # rightly so: it would accept a token from any repository on GitHub.
  attribute_condition = "assertion.repository == '${var.github_repository}'"

  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }
}

# The deploy identity. Reuses the Cloud Build service account rather than minting
# another one: it already holds exactly the roles a deploy needs (push images,
# update Cloud Run, act as the runtime SA), and two identities for one job is two
# things to audit.
resource "google_service_account_iam_member" "github_impersonation" {
  count              = local.cicd
  service_account_id = google_service_account.build.name
  role               = "roles/iam.workloadIdentityUser"
  # Scoped to this repository. NOT to the whole pool.
  member = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github[0].name}/attribute.repository/${var.github_repository}"
}

output "workload_identity_provider" {
  description = "Full provider name for google-github-actions/auth. Empty when CI/CD is disabled."
  value       = local.cicd == 1 ? google_iam_workload_identity_pool_provider.github[0].name : ""
}
