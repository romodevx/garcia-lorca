# Deploy — García Lorca Infrastructure & CI/CD

> **To actually deploy, follow `deploy/RUNBOOK-dev.md`.** That file is the
> executable procedure and the authority on values and ordering; this one
> explains the shape of the environment.


Infrastructure-as-Code (Terraform), the Cloud Build CI/CD pipeline, and the
container build setup for the García Lorca monorepo.

```
deploy/
  terraform/                 # IaC: Cloud Run, Cloud SQL, Pub/Sub, Secrets, Budget
    modules/cloud_run/       # reusable Cloud Run service module
    db/setup_app_role.sql    # one-time app-role hardening (non-owner, NOBYPASSRLS)
    environments/            # per-env *.tfvars (gitignored; *.example tracked)
  cloudbuild/
    cloudbuild.yaml          # CI/CD pipeline
    check-no-sourcemaps.sh   # build-hardening gate (fallback)
  Dockerfile.service.template  # shared Go-service image template
```

## Prerequisites

| Tool        | Version        | Purpose                         |
| ----------- | -------------- | ------------------------------- |
| `gcloud`    | latest         | GCP auth, Cloud Run deploys     |
| `terraform` | >= 1.7.0       | provision infrastructure        |
| `buf`       | >= 1.50        | proto lint + codegen            |
| `go`        | 1.25.x         | build/test services             |
| `node`      | 22.x + npm     | frontend build (Turborepo)      |
| `docker`    | latest         | container images                |

GCP requires one project per environment (`garcia-lorca-dev`, `-staging`, `-prod`),
a billing account, and a GCS bucket for Terraform remote state.

## Deploy flow

Each commit is verified **once**, in one place. Cloud Build used to re-run the
quality gates on the very commit CI had just approved — 25.5 min of every build,
billed a second time by the GitHub runner watching the stream. It no longer
does. See `.claude/docs/PLAN-minutos-ci.md` for the measurements.

**Stage 1 — the gate: `.github/workflows/ci.yml`.** The only one.

1. **Contracts**: `buf lint` → `buf breaking` (advisory) → `buf generate`
   (`gen/` is regenerated, never committed), plus the OpenAPI contract lint and
   `actionlint`, folded into the same job because a job of its own costs a whole
   billed minute for nine seconds of work.
2. **Quality gates**: `go vet` **once** across the workspace, then per module
   `go test -race` → `-tags=integration` → `govulncheck` → `go mod verify`.
   The heavy three are pull-request-only; a push gets unit tests + mod verify.
3. **Frontend**: `pnpm install --frozen-lockfile` → `turbo run test lint` →
   `build --filter=@garcia-lorca/web...` (pnpm is the only permitted package
   manager — see the repo CLAUDE.md; the filter skips the Expo app, whose build
   needs native tooling).
4. **Build-hardening gate**: fail if any `*.map` or `sourceMappingURL` exists in
   web artifacts. Uses `apps/web/scripts/check-no-sourcemaps.mjs` when present,
   otherwise `deploy/cloudbuild/check-no-sourcemaps.sh`.

> A gate deleted from `ci.yml` is a gate that no longer exists **anywhere**.
> There is no second copy in Cloud Build to catch it.

**Stage 2 — images: `deploy/cloudbuild/cloudbuild.yaml`.** Builds and pushes,
nothing else.

5. **Images**: each `services/<name>/Dockerfile` (and `apps/web/Dockerfile`)
   built from the repo root, tagged with the deployed SHA, pushed to Artifact
   Registry. Every image also carries a moving `:cache` tag and is built with
   `--cache-from` against it: Cloud Build hands out a cold Docker daemon on
   every run, so without it each image re-downloads its modules and recompiles
   the whole dependency graph.

**Stage 3 — rollout: `.github/workflows/deploy-dev.yml`.** The only rollout
implementation. The `_DEPLOY=true` step that used to live in `cloudbuild.yaml`
is gone — it had never executed once, and it used `gcloud run deploy` where dev
has always used `gcloud run services update`.

6. **Refuse unverified commits**: assert a green `ci` check-run on that exact
   SHA. Applies to `workflow_dispatch` too — a manual deploy is a human
   decision, not an exemption from verification.
7. **Submit and poll**: `gcloud builds submit --async`, then poll for the
   terminal status with a deadline. Not fire-and-forget: the red X, the
   `deploy-dev` concurrency lock and the rollout below all depend on this runner
   staying alive until the build resolves. If the runner is cancelled, the build
   is cancelled with it rather than left to roll out on its own.
8. **Migrations, then code**: schema first, always. If they fail the job stops
   and **nothing** rolls out — dev keeps serving the previous version.
9. **Rollout**: `gcloud run services update` per service, idempotent. If it is
   interrupted, the job summary says which service is on which image.
10. **Smoke test**, then the "what is dev running" summary and the
    min-instances cost guard.

**Infra** stays out of the push path: `make tf-plan ENV=dev` → review →
`make tf-apply ENV=dev`. An apply can destroy a database; a push must never be
able to.

Production uses blue/green or canary at the Cloud Run revision level (split
traffic across revisions).

**Rollback on dev is `make rollback-dev TAG=<sha>`.** It is NOT "re-run the
deploy workflow on an older commit": `workflow_dispatch` takes the workflow file
from the ref it runs on and only accepts a branch or a tag, never a bare SHA, so
an old ref would run the OLD workflow. The images for every previously deployed
commit are already in Artifact Registry — rolling back is pointing the services
back at them, and it rebuilds nothing.

Rehearsed 2026-09-19 against the live dev environment: **146 s to roll back ten
services, 157 s to roll forward, `/login` and `/readyz` answering 200 in both
states, and zero billed Actions minutes.** The target refuses to start unless
all ten images exist for that tag — half a rollback is worse than none — and it
runs the same smoke checks the deploy workflow does.

The rollout is idempotent, so rolling back, rolling forward and finishing a
rollout that died halfway are all the same operation: run it again. Document
each rollback in the service runbook.

### What it costs

`make ci-minutes` reports billed Actions minutes per CI run and per deploy.
That is the acceptance criterion, as a **rate**: a total moves with how much
work the month had and cannot tell "we got faster" from "we did less".

## Cost model

Single region, one small Cloud SQL instance, Cloud Run scale-to-zero everywhere
except the two hot-path services.

| Service                                                              | min-instances | CPU allocation       | Why                              |
| -------------------------------------------------------------------- | ------------- | -------------------- | -------------------------------- |
| `web`, `gateway`, `iam`, `students`, `config`, `finance`, `grades`, `attendance`, `comms`, `uniforms` | **0 (scale-to-zero)** | per-request | pay only while serving |
| `billing`, `scholarships`, `docs` | not deployed | — | stubs: no implementation, flags off |

**dev runs everything at zero, and that is enforced, not intended.**
`modules/cloud_run` has a precondition that FAILS the apply if any service asks
for `min_instances > 0` outside prod. Two always-warm services are what produced a
sibling project's ~700 MXN/month bill; the control that failed there was a human
remembering to check. To warm the demo before a meeting use `make cloud-warm`,
which sets it with `gcloud` — so the next apply undoes it and warmth can never
quietly persist. prod flips `gateway` and `iam` to warm through its own tfvars.

High per-instance concurrency packs requests onto fewer instances. All services
share one reusable `cloud_run` module — the cost profile is just the variables
passed in (`min_instances`, `cpu_idle`, `startup_cpu_boost`, `concurrency`).

**Database.** dev runs on an EXTERNAL Postgres (`use_cloud_sql = false`), because
Cloud SQL cannot scale to zero and was the entire resting cost of the environment.
The DSN arrives as the `DATABASE_URL` secret; `pkg/dbdsn.Resolve()` reads it before
it assembles a Cloud SQL socket DSN, so the switch costs no service code. prod sets
`use_cloud_sql = true` and gets `db_tier` / `db_availability_type = "REGIONAL"`.

**Budget alerts** are provisioned from day one (`budget.tf`): email at 50/80/100%
of actual spend and 100% of forecast. Alerts warn — they do not cap spend.

## Security posture

- **No secrets in code or tfvars.** Secret Manager holds values; Cloud Run injects
  them at runtime; `*.tfvars` is gitignored.
- **App DB role** (`app`) is **not the instance owner** and **NOBYPASSRLS** — RLS
  policies always apply. `scripts/migrate-cloud.sh` runs
  `terraform/db/setup_app_role.sql` and the migrations on ONE connection, as one
  role — `ALTER DEFAULT PRIVILEGES` is per-grantor, so splitting them leaves the
  `app` role with no rights on any new table.
- **Only `web` is public** (`allow_unauthenticated`). Every other service has
  `allow_unauthenticated = false`, so the boundary is `roles/run.invoker` plus
  audience-scoped OIDC (`pkg/gcpauth`).
- Ingress is `ALL`, NOT `INTERNAL_ONLY`. Internal ingress is a network filter
  evaluated BEFORE authentication, and Cloud Run → Cloud Run over `*.run.app`
  egresses to the public internet unless a VPC connector or Direct VPC egress is
  configured — neither is, deliberately, because both cost money. Internal-only
  would therefore reject every gateway call with a 404 from Cloud Run's frontend,
  before reaching any code.
- `govulncheck` + `go mod verify` gate every build.

## Owner temporary password

The owner/director account is seeded with a one-time **temporary** password that
the owner is required to rotate at first login (enforced server-side by the
`must_change_password` claim; see the gateway). Sourcing differs by environment:

| Environment | Source of `OWNER_TEMP_PASSWORD`                                                   |
| ----------- | -------------------------------------------------------------------------------- |
| Local       | `.env` `OWNER_TEMP_PASSWORD` (see repo-root `.env.example`, default `change-me-owner`). `docker/seed-owner.sql` consumes it via pgcrypto `crypt(...)`. |
| Shared envs | **Secret Manager → Cloud Run env → the migrate/seed step.**                      |

**Shared-env flow (stub today — not a live deployment):**

1. Terraform manages the secret when the env opts in: set `seed_owner_secret = true`
   in the env tfvars and inject the value out of band at apply time
   (`TF_VAR_owner_temp_password=... terraform apply -var-file=...`). This creates
   `owner-temp-password-<env>` in Secret Manager (`deploy/terraform/secrets.tf`,
   var-gated). The value is **never** committed to code or tfvars.
2. Cloud Run injects the secret's `latest` version as the `OWNER_TEMP_PASSWORD`
   env on the **migrate/seed step** — the same `secret_env` mechanism
   `services.tf` uses for `DB_PASSWORD` (`local.common_secret_env`). The seed
   step runs `docker/seed-owner.sql`.
3. The owner rotates the password at first login; it is a one-time credential.
   `ignore_changes = [secret_data]` keeps later applies from clobbering it.

> The Terraform block is a **var-gated stub**: with `seed_owner_secret = false`
> (the default) nothing is created and the local path is unchanged.

## Auth deployment requirements

The gateway issues auth tokens **only** as cookies `gl_access` / `gl_refresh`
with `HttpOnly; Secure; SameSite=Strict` (Phase B). This constrains deployment:

- **Same-site topology (required) — and it does NOT require buying a domain.**
  With `SameSite=Strict` the browser never sends the cookies cross-site, and
  `run.app` is on the Public Suffix List, so `web-x.run.app` and
  `gateway-y.run.app` are DIFFERENT sites: login would appear to succeed and every
  subsequent `/me` would return 401.
  This is solved in the app, not in DNS: `apps/web/app/api/[...path]/route.ts`
  proxies `/api/v1/*` to the gateway from the web app's own origin, so the cookies
  are first-party. It reads `GATEWAY_URL` per request, which also removes the
  ordering problem that a build-time `rewrites()` destination would create — the
  gateway's URL only exists after `terraform apply`.
  A shared registrable domain (`app.example.mx` + `api.example.mx`) is the nicer
  answer when one exists, and the proxy can be dropped then. It is not a
  prerequisite for deploying.
- **TLS everywhere (required).** `Secure` cookies are only sent over HTTPS.
  Shared envs are HTTPS on Cloud Run; **local dev must run behind TLS** (or a
  TLS-terminating proxy) for the cookie flow to work end to end.
- **CSRF posture.** Auth is cookie-ambient; CSRF is mitigated by
  `SameSite=Strict`. Relaxing to `Lax` in the future would REQUIRE explicit
  CSRF tokens on state-changing endpoints — do not relax it without them.
- **Login rate limiter is in-process, per instance** (Phase C). It is correct
  only at `min-instances=1` (the gateway's profile). Scaling the gateway out
  needs a shared store (e.g. Redis) or the per-instance budget multiplies.
- **`TRUST_PROXY` stays OFF** until X-Forwarded-For parsing is confirmed for the
  real proxy topology. With it off, the limiter keys on `RemoteAddr`; turning it
  on before the trusted-hop position is verified lets clients spoof their IP and
  bypass the limit.

## Pre-shared-env checklist (blockers before enabling a module flag in staging/prod)

From the Phase-1 hardening security gate (2026-08-03). None block local/MVP use; all
must be resolved before turning a flag ON in a shared environment.

> **Update 2026-08-07 (hardening batch).** Several items are now closed or mitigated:
> **CI is live** (`.github/workflows/ci.yml`) — it runs `buf lint`/`buf breaking`, per-module
> `go vet` + `go test -race` + integration + **`govulncheck`** + `go mod verify`, and the web
> `test`/`lint`/`build` + **source-map gate** on every PR, so items 1 and the build-hardening
> gate are now enforced by pipeline (still confirm the toolchain/`x/text` bumps land green).
> **Payment idempotency** (finance I1) is DONE (`idempotency_key` + partial-unique + replay).
> **Items 14 & 20 (per-object authz on finance/attendance reads)** are now *mitigated, not yet
> fully closed*: a new iam **grant-guard** blocks assigning `finance:read`/`attendance:read` to
> any non-owner role, and the list reads are **bounded** (`ListCharges` requires a student/cycle
> filter; all list reads cap at 500 rows, ordered). True per-student scoping still lands with the
> Phase 9 guardian/teacher identity model before these reads go to a non-owner/family role.

1. **`govulncheck` clean in CI.** As of the gate, code-reachable vulns come from the
   Go toolchain and one dependency (pre-existing, not first-party): bump the toolchain
   to **Go 1.25.12** and **`golang.org/x/text` → v0.39.0** (fixes GO-2026-5970 and the
   stdlib advisories), then confirm `govulncheck ./...` is clean per module.
2. **`TRUST_PROXY` set + XFF verified** against the real Cloud Run / load-balancer
   header format (the limiter reads the trusted right-most hop — confirm that matches
   the topology, or per-IP isolation is wrong).
3. **Gateway `max-instances=1`** (or move the login rate limiter to a shared store)
   so the per-instance budget isn't multiplied.
4. **Owner secret from Secret Manager** (not a local `.env`); set `seed_owner_secret`
   + `owner_temp_password` (see "Owner temporary password" above).
5. Frontend rotates the session after a forced password change — **done** (the web
   calls `/auth/refresh` immediately after `change-password` so the cookie picks up
   `must_change_password=false`).

### Added by Phase 2 (`config` service) — before enabling `module_config` in a shared env
6. **Internal services must be network-private.** `config` (like `iam`/`students`)
   has no independent authn — the gateway is the sole auth boundary. The host-published
   port (`8101` in docker-compose) is LOCAL ONLY; in Cloud Run set `ingress=internal`
   so only the gateway can reach it.
7. **Multi-tenant forwarding.** `config` is single-tenant by construction
   (`CONFIG_TENANT_ID` is a process-wide constant; the client cannot set `tenant_id`).
   Before any multi-tenant shared use, the gateway must forward the principal's
   `TenantID` to `config` and the constant must be removed. (Same for `students`,
   `grades` — all carry a `*_TENANT_ID` constant.)

### Added by Phase 6 (roles & users) — before a shared, multi-admin environment
8. **"Grant only what you hold."** A `roles:write` admin can currently mint/assign a
   role with ANY catalog permission (e.g. grant `grades:write`). Fine for the single
   owner; before non-owner admins exist, restrict the assignable permission set to a
   subset of the caller's own effective permissions (enforce in the iam admin usecase).
9. **Token revocation on suspend.** `SetUserStatus(SUSPENDED)` blocks new logins and
   refresh, but an already-issued access token stays valid until it expires (15-min
   TTL). For immediate revocation, add a token denylist (or shorten the access TTL) —
   the 15-min window is acceptable for MVP only.
10. **Grade capture is not teacher-scoped yet.** Any `grades:write` holder can capture
    for any assignment (teacher derived from the assignment, not the caller). Once
    teacher login exists (link `teachers.user_id` → require the caller own the
    assignment), enforce it before grades is more than a presentation surface.

### Added by Phase 7 (`finance` service) — before enabling `module_finance` in a shared env
Finance is a money ledger; the security gate (2026-08-06) returned CONDITIONAL PASS.
None block local/demo (owner-only); all must be resolved before `module_finance` is ON
in a shared, multi-role, or family-facing environment.
11. **Internal service must be network-private.** `finance` has no authn of its own — the
    gateway is the sole boundary, and it trusts any caller reaching its Connect port. In
    Cloud Run set `ingress=internal`; do NOT publish `8093`. It also calls `students`/`config`
    server-to-server with a plain client (no credentials) — same private-network requirement.
12. **Multi-tenant forwarding.** `FINANCE_TENANT_ID` is a process-wide constant fixed at
    composition (not derived from the caller). Before multi-tenant use, the gateway must
    forward the principal's `TenantID` and the constant must be removed (same as iam/config/
    students/grades).
13. **Audit trail on money operations (B1).** `payments.received_by` is always NULL, the
    discount `reason` is accepted then discarded, and the gateway forwards no caller identity.
    Who took the cash / forgave the debt / voided the charge is unrecoverable. Forward the
    authenticated subject and persist it on payment, discount, and cancel before shared use.
14. **Per-object authz on finance reads (B2).** `finance:read` is tenant-wide: `GET
    /finance/accounts/{studentId}` returns any student's statement and `GET /finance/charges`
    (no filter) returns the whole tenant ledger. Fine for the single owner; **do NOT grant
    `finance:read` to any non-owner role** (parent/family portal in Phase 9, teacher roles)
    until account/charge reads are scoped to the caller's own students.
15. **Money-op permission split + hardening (recommended).** Split `finance:discount`/
    `finance:cancel` out of `finance:write` (a cashier should bank cash, not erase debt);
    paginate the unbounded ledger reads; rate-limit `POST /finance/charges/generate`.
16. **Real Pub/Sub + event emission.** Finance publishes `PaymentRegistered` onto a dev
    logging bus (`newDevBus`) and swallows publish failures by design; swap in the Cloud
    Pub/Sub adapter before shared use. `DebtOverdue` is a READ only (no event emitter yet);
    `ExportContpaqi` returns `Unimplemented` and is intentionally unrouted (S02 / Phase 7b).

### Added by Phase 8 (`attendance` service) — before enabling `module_attendance` in a shared env
17. **Internal service network-private.** `attendance` has no authn of its own — the gateway
    is the sole boundary. Cloud Run `ingress=internal`; do NOT publish `8095`. It calls
    `students`/`config` server-to-server with a plain client (no credentials) — same private
    network requirement.
18. **Multi-tenant forwarding.** `ATTENDANCE_TENANT_ID` is a process-wide constant fixed at
    composition; before multi-tenant use the gateway must forward the principal's `TenantID`
    and the constant must be removed (same as iam/config/students/grades/finance).
19. **Real Pub/Sub before Comms (Phase 9) depends on it.** `attendance` emits
    `AttendanceMarked`/`AbsenceRecorded` onto the dev logging bus and swallows publish
    failures by design. Comms (Phase 9) consumes `AbsenceRecorded` to notify guardians —
    swap in the Cloud Pub/Sub adapter (with delivery guarantees) before that dependency is
    live. The event payloads carry ids only (no student PII), which is correct for a
    guardian-facing notification pipeline — keep it that way.
20. **`ListAttendance` has no per-object authz.** `GET /api/v1/attendance?enrollmentId=…` goes
    straight to the repo with only tenant RLS ("same tenant", not "yours"). Fine for the single
    owner (0008 grants `attendance:read` to owner alone); the instant a teacher role or the
    Phase 9 family portal gets `attendance:read` it's a horizontal-authz hole — any holder reads
    any enrollment's absence history. Scope the read to the caller's own students/groups before
    granting `attendance:read` to a non-owner role (same shape as finance item 14).
21. **`note` is unbounded free text and likely sensitive PII.** The teacher's justification
    field (`attendance.note`) is plain `text` (only the 1 MiB body cap bounds it) and is returned
    in full by `ListAttendance` — so it inherits item 20. It routinely holds health data
    ("cita médica"). Cap its length (service + DB CHECK) and treat it as sensitive when roles
    expand.
22. **Attendance writes are not rate-limited** (only `/auth/login` is). One `POST /api/v1/attendance`
    reconciles a group's whole day. Authenticated + reconcile-scoped so low risk, but add a
    limiter before a shared, multi-writer environment.

### Added by Phase 9a (`comms` event backbone) — before enabling `module_comms` / real events in a shared env
23. **Real FCM via Secret Manager.** Push uses a `LogSender` stub; the real Firebase Cloud
    Messaging sender (service-account credentials) is injected by env/Secret Manager behind an
    `FCM_ENABLED` flag. No Firebase credential is committed. (See ADR 0003.)
24. **Cloud Pub/Sub subscription hygiene.** Delivery is at-least-once; the comms consumer is
    idempotent (`processed_events`), but the Cloud Pub/Sub subscriptions still need a
    **dead-letter topic + retry/backoff policy** (not needed on the local emulator) so a
    poison message can't loop forever. Provision in Terraform with the topics.
25. **Internal service + single-tenant.** `comms` (like the others) has no own authn → gateway is
    the boundary → `ingress=internal`, don't publish its host port. `COMMS_TENANT_ID` is a
    process-wide constant → gateway must forward the principal's tenant before multi-tenant use.
26. **`comms:read` is tenant-wide** — `ListNotifications(studentId)` returns any student's
    notifications. Owner-only today (0009); **added to the iam `IsOwnerOnly` guard in 9a** so it
    can't be granted to a non-owner role yet. Still scope reads to the caller's own students
    before granting it to a guardian/family or teacher role (Phase 9b).
27. **Device owner must come from the authenticated principal (B2/B3).** `RegisterDevice` takes a
    client-supplied `userId` and the upsert reassigns `user_id` on an `(tenant,fcm_token)` conflict.
    Harmless with tenant-wide fanout, but once targeting is per-guardian this lets anyone with a
    token redirect a device to another user (cross-family push). Derive `userId` from the principal
    and require the conflicting row's owner to match, before per-guardian targeting.
28. **Real FCM requires per-student targeting first (B4).** `DevicesForTenant` fans every
    notification to every device in the tenant. Inert with the `LogSender`; enabling the real FCM
    sender (`FCM_ENABLED`) MUST be gated on per-student/guardian targeting existing, not merely
    sequenced after it — else every family gets every family's notices.
29. **No silent dev-bus fallback in shared envs (B5/I3).** `selectPublisher` falls back to the log
    bus if the broker fails to init; with a broker CONFIGURED (`PUBSUB_PROJECT_ID` set) a failure
    must fail startup (let Cloud Run restart), not boot healthy and silently drop every event.
30. **Rate-limit comms writes (B6).** `RegisterDevice` is an authenticated write called on every
    app launch; add a limiter before a shared, multi-client environment (group with item 22).

### Added by Phase 9b (guardians + family auth)
Phase 9b introduces family identity + a **self-scoped** read path, closing the per-object-authz
spirit of items 14/20/26 FOR THE FAMILY PATH (`/api/v1/family/*` derives the caller's children from
the principal — a guardian can never name another family's student). The admin-side `finance:read`/
`attendance:read`/`comms:read` stay owner-only + `IsOwnerOnly`-guarded; the `guardian` role holds only
`family:read`. Still open before a shared family-facing env:
31. **Mobile token lifecycle.** Bearer tokens are returned in the body for the mobile path (secure
    storage is the 9c app's job). Consider a shorter access TTL for mobile and confirm MobileRefresh
    has the same rotation/revocation posture as the web refresh (ties to checklist item 9, token
    revocation on suspend). Real FCM (item 23) still required before push is meaningful.
32. **Guardian linking is admin-only** (`guardians:write`, owner). A guardian self-service invite/claim
    flow (a parent registering + proving their child) is deferred — until then, linking is a
    back-office action, so vet who holds `guardians:write`.
33. **BLOCKING — device-owner spoofing is armed (was item 27, latent; 9b fired its trigger).**
    Per-guardian push targeting shipped, so the deferred device-owner hole is now live. Gateway
    `RegisterDevice` now derives the owner from the authenticated principal (closed at the edge in 9b),
    but the comms repo upsert still reassigns `user_id` on an `(tenant,fcm_token)` conflict
    (defense-in-depth, open). Do NOT enable the real FCM sender, and do NOT grant the `guardian` role
    `comms:write`, until the repo upsert also refuses to reassign a token to a different owner.
34. **The `guardian` role holds only `family:read`** — so a guardian can't yet register a device or mark
    a notification read (both need `comms:write`). Fail-closed and correct for 9b; 9c must decide the
    guardian's `comms:write` grant, and that decision must land AFTER item 33's repo fix.
35. **`guardians:read/write` are now `IsOwnerOnly`-guarded** (they compose into a tenant-wide family
    read via self-linking + `/family/*`). This makes the grant-subset gap ("grant only permissions you
    hold") load-bearing, not cosmetic: a non-owner `roles:write` holder could still route around it —
    close the grant-subset gap before non-owner admins exist.

## Common commands

```bash
make generate            # buf lint + generate
make test                # go test -race + turbo test
make lint                # buf lint + go vet + turbo lint
make build               # go build + turbo build
make image-gateway       # build gateway image locally (needs PROJECT_ID)
make tf-plan  ENV=dev     # terraform plan
make tf-apply ENV=dev     # terraform apply
make deploy-gateway ENV=dev TAG=<sha>
```
