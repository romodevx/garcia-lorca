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
