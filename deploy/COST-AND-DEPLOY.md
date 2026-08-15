# García Lorca — GCP Deployment & Cost-Minimization

Grounded in `deploy/terraform/**`, `deploy/cloudbuild/cloudbuild.yaml`, `docker-compose.yml`, `go.work`.
Prices are **estimates (us-central1) — verify against current GCP pricing before committing.**

Deployment model:
- **dev** → owner's PERSONAL GCP account (owner + school preview). Tolerates cold starts.
- **master/prod** → SCHOOL's GCP account, once the school contracts. Warm, HA.

---

## Headline

1. **The dominant cost today is NOT the database — it's the two always-warm Cloud Run
   services.** `services.tf` sets `gateway` + `iam` to `min_instances=1` + `cpu_idle=false`
   (1 vCPU allocated 24/7 each) ≈ **~$95-100/mo combined** — more than the rest of the
   stack plus the DB. For **dev**, letting them scale to zero takes the environment from
   **~$110-130/mo → ~$12-20/mo**. Cold start on the first morning request (~1-3s) is
   acceptable in dev.
2. **The Terraform is not deploy-ready yet.** Only `gateway`, `iam`, `students` are wired
   in `services.tf` + CI; `config`, `finance`, `grades`, `attendance`, `comms` are a
   TODO comment. Only gateway/iam have Dockerfiles. **This is the real work before dev can
   go live** (see §E blockers).
3. After tuning, **Cloud SQL is the true fixed floor** (~$12/mo, can't scale to zero).

**Realistic dev floor after tuning: ~$12-20/month.**

---

## A. Current infra inventory (from the Terraform)

| Resource | Setting | Source |
|---|---|---|
| Artifact Registry | 1 Docker repo, **no cleanup policy** | `main.tf:39-48` |
| Runtime SA | roles: cloudsql.client, secretAccessor, pubsub.publisher+subscriber | `main.tf:53-76` |
| Cloud SQL | POSTGRES_16, **db-g1-small**, ZONAL, 10GB SSD autoresize, **backups+PITR ON**, no public IP | `database.tf:4-40` |
| Cloud Run gateway | **min=1, max=20, cpu_idle=false**, conc=200, cpu=1, 512Mi, public | `services.tf:47-54` |
| Cloud Run iam | **min=1, max=10, cpu_idle=false**, conc=120, internal | `services.tf:73-79` |
| Cloud Run students | min=0, max=10, cpu_idle=true, internal | `services.tf:101-107` |
| config/finance/grades/attendance/comms | **NOT written** (TODO comment) | `services.tf:88-91` |
| Pub/Sub | `payment-events`(+dlq); `attendance-events`/`student-events` created at runtime, **not in TF** | `pubsub.tf` |
| Secret Manager | db-app-password; owner-temp-password (var-gated) | `secrets.tf` |
| Budget | monthly alert 50/80/100%, **alert-only, does NOT cap spend** | `budget.tf` |
| Remote state | GCS backend, bucket+prefix at init (per-env isolatable) | `versions.tf:11-16` |

**Real services to deploy (8):** gateway, iam, students, config, finance, grades, attendance, comms.
**Stubs — do NOT deploy (4):** billing, scholarships, docs, uniforms (health-only shells, flags OFF).

---

## B. Cost drivers ranked (dev, monthly, estimates)

| # | Driver | As-written | After dev tuning |
|---|---|---|---|
| 1 | **Cloud Run always-warm** gateway+iam (min=1, cpu_idle=false) | **~$95-100** | **~$0** (scale to zero) |
| 2 | **Cloud SQL** (db-g1-small ZONAL + backups/PITR) | ~$28-32 | **~$12** (db-f1-micro, PITR off) |
| 3 | Cloud Build (E2_HIGHCPU_8 machine) | ~$2-6 | ~$0-2 (default machine, free minutes) |
| 4 | Cloud Run request/CPU (real traffic) | ~$0-3 | ~$0-3 |
| 5 | Artifact Registry storage (no cleanup) | ~$0.20-1 growing | ~$0.20 |
| 6-9 | Secrets / Pub/Sub / Logging / Egress | ~$0-1 | ~$0-1 |

As-written: **~$110-130/mo.** Tuned dev: **~$12-20/mo** (Cloud SQL-dominated).

---

## C. Lowest-cost configuration (concrete)

### C1 · Cloud SQL (the floor)
- **Tier → `db-f1-micro`** for dev (~$9 vs ~$25). Keep **Enterprise** edition (shared-core tiers only exist there). Keep **ZONAL** (REGIONAL doubles cost — reserve for prod).
- **PITR OFF for dev** (`database.tf:22`) — daily backups suffice. Make it a var, ON only for prod.
- Retain 2-3 backups in dev. Storage 10GB SSD is already minimal.
- **Alternative for ~$0 dev:** Neon / Supabase free tier — real Postgres, **RLS + `SET LOCAL app.current_tenant` work unchanged**. Trade-off: off-GCP (breaks the one-project-per-env model, connect over public TLS). Fine for personal dev; **not** for the school's prod. → owner decision.

### C2 · Cloud Run (dev profile)
- **gateway + iam → `min_instances=0`, `cpu_idle=true`** in dev (tfvars-driven so prod stays warm). **The top lever (~$95-100/mo).**
- **gateway `max_instances=1` in dev** — the login rate limiter is in-process/per-instance; min=0/max=1 keeps it correct and caps cost.
- Keep cpu=1 / 512Mi / high concurrency. Cap internal-service max at 2-3.

### C3 · Topology — deploy the 8 real services separately, skip the 4 stubs. **Do NOT consolidate.**
Cloud Run bills per request/CPU-second, **not per service** — 8 scale-to-zero services cost the same as 1 (≈$0 idle). Consolidating into a monolith saves no dev money and costs a rewrite against the bounded-context design. Skipping the 4 stubs removes 4 images/services for zero loss — that's the only worthwhile trim.

### C4 · Web (Next.js)
- **Cloud Run scale-to-zero** (SSR via next-intl cookie-locale → not fully static). Add `output: 'standalone'` to `next.config.js` for a lean image/faster cold start.
- **Same-site auth constraint:** cookies are `SameSite=Strict`, so **web + gateway must share a registrable domain** (`app.example.com` + `app.example.com/api`). A split `*.run.app` domain **breaks auth** → a custom domain is required for a working end-to-end deploy. Mobile Expo app = no GCP cost.

### C5 · Guardrails
- Budget **only alerts, never caps** — keep alert-only for dev, tighten amount to ~$30. (True cap = Pub/Sub→Function disabling billing; heavy-handed for dev.)
- Artifact Registry **cleanup policy** (keep last 5 tags / delete untagged >30d). Cloud Build: drop `E2_HIGHCPU_8` in dev. Logging: 30-day retention (stays in free tier).

---

## D. Two-account / two-project model (already well-supported)

The Terraform is multi-env-ready: per-env `project_id`, `billing_account_id`, SA/SQL/secret names all `${env}`-suffixed, and **remote state bucket+prefix supplied at init** (fully isolated per account). `deletion_protection` auto-on only when `env==prod` (protects the school's data).

**Promotion path (maps to branch→dev→master):**
1. **DEV now** — `project_id=garcia-lorca-dev`, owner's billing, own state bucket, `db-f1-micro`, scale-to-zero everywhere.
2. **PROD later** — school's project + billing account, **separate** state bucket in the school's project, REGIONAL DB, warm gateway+iam, PITR on. Same Terraform + a `prod.tfvars`. Zero shared resources; each account builds into its own Artifact Registry.

**Needs attention for the split:**
- `env` var allows only `dev/staging/prod` (`variables.tf:15-18`) → the school env must be `prod` (not "master"/"school").
- `attendance-events`/`student-events` topics are created at runtime, not in TF → add them to TF (declarative per project) and confirm the runtime SA can create topics or pre-create them.

---

## E. "Start now" plan (get DEV live cheaply)

### Blockers to close first (these prevent a working deploy today)
1. **Only 3 of 8 real services are in IaC/CI.** Add `services.tf` module blocks + Dockerfiles (from `deploy/Dockerfile.service.template`) + CI build/deploy steps for **config, finance, grades, attendance, comms**. Skip the 4 stubs.
2. **Gateway→internal wiring missing.** Inject each internal service's `*.run.app` URL as `*_BASE_URL` + `ENABLED_MODULES` into the gateway; grant the runtime SA `roles/run.invoker` on each internal (internal-ingress) service, OR add a Serverless VPC connector (module supports it, currently null).
3. **Owner seed:** set `seed_owner_secret=true` + inject `TF_VAR_owner_temp_password` at apply; owner rotates at first login.

### Apply order (once blockers closed)
1. Create `garcia-lorca-dev` project on owner's billing; create GCS state bucket; enable billing.
2. `terraform init -backend-config=bucket=… -backend-config=prefix=cloud-run/dev`.
3. `dev.tfvars`: `billing_account_id`, `db_tier="db-f1-micro"`, dev scale-to-zero overrides, `monthly_budget_amount≈30`.
4. `make tf-plan ENV=dev` → review → `make tf-apply ENV=dev`.
5. Write DB app password to Secret Manager; run `db/setup_app_role.sql` as superuser (NOBYPASSRLS).
6. Apply migrations + owner seed.
7. Build+deploy images (Cloud Build, `_DEPLOY=true`).
8. Map one registrable domain fronting web + gateway same-site (required for cookie auth).

### Top 5 cost levers — checklist
- [ ] gateway + iam → `min_instances=0`, `cpu_idle=true` in dev (**~$95-100/mo — the big one**)
- [ ] Cloud SQL → `db-f1-micro`, PITR off, 2-3 backups, ZONAL (~$18/mo)
- [ ] Deploy only the 8 real services; skip billing/docs/uniforms/scholarships
- [ ] Artifact Registry cleanup policy + drop E2_HIGHCPU_8 Cloud Build machine in dev
- [ ] max-instances caps (gateway=1) + budget alert ~$30

### Open decisions for the owner
1. **Custom domain?** Required for `SameSite=Strict` cookie auth (web+gateway same-site).
2. **Dev DB:** GCP `db-f1-micro` (~$12/mo, clean single-project model) vs Neon/Supabase free (~$0, off-GCP, RLS still works).
3. **HA/PITR for prod** (REGIONAL doubles DB cost) — defer to school contract.
4. **Consolidate services?** Recommendation: **no** (saves nothing at dev scale, costs a rewrite).
5. **Hard billing cap** vs alert-only — recommend alert-only for dev.
