# Deploy — García Lorca Infrastructure & CI/CD

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

1. **Contracts**: `buf lint` + `buf generate` (CI regenerates `gen/`; never committed).
2. **Quality gates** (fail-fast): `go vet` → `go test -race` → `govulncheck` + `go mod verify`.
3. **Frontend**: `npm ci` → `turbo run test lint build`.
4. **Build-hardening gate**: fail if any `*.map` or `sourceMappingURL` exists in
   web artifacts. Uses `apps/web/scripts/check-no-sourcemaps.sh` when present,
   otherwise `deploy/cloudbuild/check-no-sourcemaps.sh`.
5. **Images**: build each `services/<name>/Dockerfile` from the repo root, tag
   with `$SHORT_SHA`, push to Artifact Registry.
6. **Infra**: `make tf-plan ENV=dev` → review → `make tf-apply ENV=dev`. CI writes
   the built `$SHORT_SHA` into `image_tags` so infra and revisions agree.
7. **Deploy**: `gcloud run deploy` (pipeline, `_DEPLOY=true` on main/release) or
   `make deploy-gateway ENV=dev TAG=<sha>` locally.

Production uses blue/green or canary at the Cloud Run revision level (split
traffic across revisions). **Rollback = redeploy a prior `$SHORT_SHA`** (or shift
100% traffic back to the previous revision); document each rollback in the
service runbook.

## Cost model

Single region, one small Cloud SQL instance, Cloud Run scale-to-zero everywhere
except the two hot-path services.

| Service                                                              | min-instances | CPU allocation       | Why                              |
| -------------------------------------------------------------------- | ------------- | -------------------- | -------------------------------- |
| `gateway`                                                            | **1 (warm)**  | always + boost       | public edge; no cold start       |
| `iam`                                                                | **1 (warm)**  | always + boost       | every request authenticates      |
| `students`, `finance`, `grades`, `scholarships`, `attendance`, `docs`, `comms`, `uniforms`, `billing` | **0 (scale-to-zero)** | per-request | pay only while serving           |

High per-instance concurrency packs requests onto fewer instances. All services
share one reusable `cloud_run` module — the cost profile is just the variables
passed in (`min_instances`, `cpu_idle`, `startup_cpu_boost`, `concurrency`).

**Cloud SQL** starts at `db-g1-small` with auto storage increase; bump `db_tier`
and set `db_availability_type = "REGIONAL"` for prod HA.

**Budget alerts** are provisioned from day one (`budget.tf`): email at 50/80/100%
of actual spend and 100% of forecast. Alerts warn — they do not cap spend.

## Security posture

- **No secrets in code or tfvars.** Secret Manager holds values; Cloud Run injects
  them at runtime; `*.tfvars` is gitignored.
- **App DB role** (`app`) is **not the instance owner** and **NOBYPASSRLS** — RLS
  policies always apply. Run `terraform/db/setup_app_role.sql` once per env as an
  owner connection to fix role attributes and baseline grants.
- **Cloud SQL** has no public IP; reached via the Cloud SQL proxy volume.
- Only `gateway` is public (`allow_unauthenticated`); all other services are
  `INGRESS_TRAFFIC_INTERNAL_ONLY`.
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

- **Same-site topology (required).** The web app and the gateway MUST be
  same-site — a shared registrable domain via reverse-proxy / path routing
  (e.g. `app.example.com` + `app.example.com/api`, or an edge that fronts both).
  With `SameSite=Strict`, cross-site cookies are NOT sent on the SPA's `fetch`
  calls, so a split-domain deployment breaks the auth flow.
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
