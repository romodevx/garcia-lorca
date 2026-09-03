# Google Cloud Identity Platform — F1 of .claude/docs/PLAN-identity-platform.md.
#
# Provisioning ONLY. Nothing authenticates against this yet: the gateway still
# validates tokens minted by the `iam` service, and will keep doing so until the
# `auth_provider` flag is cut over (F6). Standing this up early is deliberate —
# it is the one phase with no blast radius, so it can land and be verified
# without touching a login anyone is using.
#
# WHY AT ALL: the `iam` service has no password recovery. Its RPCs are Login,
# RefreshToken, ChangePassword (which demands the CURRENT password), CreateUser
# and SetUserStatus — there is no admin reset. A teacher who forgets their
# password is locked out and the director cannot help them. With twenty teachers
# using this daily, that happens.
#
# COST: free below 50,000 monthly active users. The school is ~1,500 counting
# guardians, so this stays inside the free tier by two orders of magnitude and
# adds nothing to the bill this project is deliberately keeping near zero.

variable "enable_identity_platform" {
  description = <<-EOT
    Provision Identity Platform in this environment. Follows the house pattern of
    enable_billing_killswitch: optional infrastructure is opt-in per environment
    rather than switched on everywhere by a merge. Provisioning is inert on its
    own — no user can authenticate through it until the gateway is cut over.
  EOT
  type        = bool
  default     = false
}

variable "identity_platform_authorized_domains" {
  description = <<-EOT
    Domains allowed to complete OAuth redirects and password-reset action links.
    Include localhost to exercise the flow against the local stack, plus the
    public host of the web app (no scheme, no trailing slash).
  EOT
  type        = list(string)
  default     = ["localhost"]
}

resource "google_project_service" "identitytoolkit" {
  count = var.enable_identity_platform ? 1 : 0

  project            = var.project_id
  service            = "identitytoolkit.googleapis.com"
  disable_on_destroy = false
}

resource "google_identity_platform_config" "auth" {
  count = var.enable_identity_platform ? 1 : 0

  project = var.project_id

  sign_in {
    # Email + password, and the password is REQUIRED. Leaving password_required
    # false would also allow email-link ("magic link") sign-in, which is a second
    # way into every account — and a second thing to reason about — for a school
    # office that signs in from a shared front desk. One door.
    email {
      enabled           = true
      password_required = true
    }

    # Anonymous sign-in is off. This system holds minors' records; an
    # unauthenticated identity has no business existing in it, not even briefly.
    anonymous {
      enabled = false
    }

    # Phone/SMS is off. It is the one part of Identity Platform that costs money
    # outside the free tier, and nothing in the product needs it.
    phone_number {
      enabled = false
    }

    # The email address IS the identity: `users.email` is unique per tenant and
    # is what maps an Identity Platform account back to our own user row and its
    # permissions. Duplicates would make that mapping ambiguous.
    allow_duplicate_emails = false
  }

  # Domains allowed to complete OAuth redirects and password-reset action links.
  #
  # Passed in rather than read from module.web.uri. Deriving it looked tidier and
  # was worse: it made this resource depend on the whole Cloud Run stack, so a
  # `-target`ed plan for JUST this config dragged in all ten services and their
  # pending image-tag drift. An auth config has no business being coupled to a
  # deploy.
  #
  # ceglorca.com is NOT here yet: the school owns it but we have no access. It is
  # not needed to migrate — Identity Platform sends reset mail from its OWN
  # sender to whatever address the user has, so owning the recipient domain was
  # never a requirement. Adding it later (F7) only buys a nicer From: line.
  authorized_domains = var.identity_platform_authorized_domains

  depends_on = [google_project_service.identitytoolkit]
}

output "identity_platform_enabled" {
  description = "Whether Identity Platform is provisioned in this environment."
  value       = var.enable_identity_platform
}
