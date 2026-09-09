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

variable "identity_platform_api_key" {
  description = <<-EOT
    The Identity Platform WEB api key, used by iam for exactly one thing: asking
    the provider to MAIL an invitation or password-reset link. Nothing else in
    the service needs it.

    It is not a secret, and treating it as one would be cargo cult: this is the
    same public key the browser bundle carries, Google documents it as public,
    and the only thing it can do on this endpoint is send a link to an address
    that already owns the account — an attacker holding it can mail somebody
    their own reset link and nothing more. The service-account credentials, which
    ARE sensitive, stay in Application Default Credentials.

    Empty is a valid, deliberate state: sign-in keeps working, and an attempt to
    invite refuses with a message naming the missing configuration instead of
    creating an account nobody can ever reach.
  EOT
  type        = string
  default     = ""
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

  # Nobody signs themselves up. Accounts are created by the school, through
  # AdminService.InviteUser, and that is the ONLY way in.
  #
  # Without this the web API key — which is public by design and ships in the
  # browser bundle — lets anyone create an account at any address in the project.
  # They could not impersonate a real user (the verifier demands a VERIFIED
  # email, so the address has to be one they actually receive mail at), but they
  # would control the provider's entry for addresses the school has not invited
  # yet, and the school would lose ownership of its own identity lifecycle:
  # inviting that person then fails with EMAIL_EXISTS and needs a console
  # deletion to unblock.
  client {
    permissions {
      disabled_user_signup = true
    }
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

# ---- What the runtime needs from the provider ------------------------------
# A role with EXACTLY two permissions, and it is custom because no predefined
# one fits:
#
#   firebaseauth.viewer  has users.get but not users.create — InviteUser fails.
#   firebaseauth.editor  and firebaseauth.admin are the SAME 16 permissions, and
#                        both include users.delete and configs.update. A running
#                        service that can rewrite the provider config could
#                        re-enable the self sign-up disabled above, which is a
#                        security fix undoing itself from inside.
#
# So: get (the revocation check in VerifyIDTokenAndCheckRevoked) and create
# (InviteUser). Nothing else.
resource "google_project_iam_custom_role" "identity_runtime" {
  count = var.enable_identity_platform ? 1 : 0

  project     = var.project_id
  role_id     = "garciaLorcaIdentityRuntime"
  title       = "García Lorca — identity runtime"
  description = "Verify ID tokens, create invited accounts, and write the password their owner chose. Nothing else."
  permissions = [
    "firebaseauth.users.get",
    "firebaseauth.users.create",
    # Writing the password its owner just chose, when they arrive through OUR
    # activation page (AdminService.AcceptInvitation → Provisioner.SetPassword).
    #
    # This is a real widening and worth naming: it lets whatever holds this role
    # set the password of ANY account in the project, the director's included.
    # That is why it no longer sits on the shared runtime account — see
    # google_service_account.iam_runtime below.
    #
    # The alternative was to keep only get+create and let Identity Platform's own
    # link be the only way anybody ever sets a password. That works, and it is
    # what the provider-sends path does — but it caps every invitation at
    # Google's one hour, which is the requirement the school asked us not to cap.
    "firebaseauth.users.update",
  ]
}

# iam's OWN service account.
#
# The note that used to sit here said every service shared
# google_service_account.runtime, so this grant reached students, finance and the
# rest — and that the clean fix was an account per service, left as its own task.
# Adding firebaseauth.users.update is what made that task due: a permission that
# can set the director's password had no business reaching nine services that
# never talk to the identity provider.
#
# Only iam is split out. The other nine keep sharing, because nothing about this
# change makes their situation worse, and a ten-way split inside a PR that
# already touches twenty files is how a deploy breaks.
resource "google_service_account" "iam_runtime" {
  account_id   = "gl-iam-runtime"
  display_name = "García Lorca — iam runtime"
  description  = "Runtime identity for the iam service only. Holds the identity-provider permissions no other service should have."
}

# What iam still needs after leaving the shared account.
#
# Verified against what the service actually does rather than copied wholesale:
# it opens Postgres and reads secrets, and that is all. It publishes no events
# (internal/adapters/pubsub holds only a doc.go) and calls no other Cloud Run
# service, so the Pub/Sub roles the shared account carries are deliberately NOT
# reproduced here — leaving the account is worth nothing if it arrives with
# everything it was leaving behind.
#
# Callers are unaffected: run.invoker is granted to the CALLER, and the gateway
# keeps the shared account.
resource "google_project_iam_member" "iam_runtime_grants" {
  for_each = toset(["roles/cloudsql.client", "roles/secretmanager.secretAccessor"])

  project = var.project_id
  role    = each.value
  member  = "serviceAccount:${google_service_account.iam_runtime.email}"
}

resource "google_project_iam_member" "runtime_identity" {
  count = var.enable_identity_platform ? 1 : 0

  project = var.project_id
  role    = google_project_iam_custom_role.identity_runtime[0].id
  member  = "serviceAccount:${google_service_account.iam_runtime.email}"
}
