# PLAN-sprint-01 — Sistema de Gestión Escolar García Lorca

> ## ✅ APPROVED — human checkpoint complete (2026-06-11)
> Engineering Manager's execution plan, built from `MODULES.md` + `FLAGS.md` and approved by Andrés.
> Estimates are honest ranges (Clean Coder discipline: no untested-commitment optimism); soft estimates flagged ⚠️.
>
> **Checkpoint decisions baked in:**
> 1. **Sprint 1 approved exactly as drafted** — no scope changes. Proceed.
> 2. **M11 Billing/CFDI is OUT of MVP** — future-ready only (proto stub + `module_billing` flag OFF + reserved service boundary; no impl this cycle). Estimate moved to a separate Deferred/Future bucket.
> 3. **Spike S01 (CFDI/IEDU) demoted** to deferred/optional future-track research. CSD-del-colegio is no longer an MVP blocker (only a future-billing prerequisite). **S02 (CONTPAQi) stays in scope** inside M05 finance.
> 4. **M13 Dashboard stays Could** — minimal read-model plumbing folded into gateway M02.

---

## 0. Estimation unit & method

**Unit: ideal-engineer-days (IED)** = one focused engineer-day, no meetings, no context-switching.
Translate to calendar with a ~0.6 focus factor when scheduling.

Sizing already bakes in:
- **TDD overhead (~+30%)**: failing test first, then impl, then refactor. Non-negotiable, so it is *in* the number, not added on top.
- **Clean-Arch cost**: modules marked Clean Arch (`Yes`) carry domain + usecase + adapter + infra layering → more expensive than `thin` services.
- **Clean Coder +20% buffer** applied to the *range top*, not hidden.

T-shirt → IED mapping:

| Size | IED range | Meaning |
|------|-----------|---------|
| S  | 1–2   | one slice, thin, low unknowns |
| M  | 3–5   | a module with modest domain logic |
| L  | 6–10  | full Clean-Arch module, events, RLS |
| XL | 11–18 | foundational or high-unknown; should be split if it grows |

---

## 1. Estimates per module

### Committed MVP scope

| ID | Module | Arch | Size | IED (low–high) | Soft? | Notes driving the estimate |
|----|--------|------|------|----------------|-------|----------------------------|
| **M00** | Cimientos (infra) | n/a | **XL** | 11–16 | ⚠️ | Monorepo (turborepo + go.work) + buf + CI/CD + Terraform (Cloud Run, Cloud SQL, Pub/Sub) + hello-world deployed end-to-end w/ integration test. First-time GCP wiring = real unknowns. |
| **S02** | Spike CONTPAQi XML (inside M05) | n/a | **S** | 2–3 | ⚠️ | Produce + validate one sample export. Field mapping is the risk. Timeboxed. Stays in scope — feeds finance M05. |
| **M01** | IAM (auth/roles/permisos) | Yes | **L** | 7–10 | — | JWT issue/validate, role+permission model, RLS session vars per tx. Spine — everything authorizes against it, so quality bar is high. |
| **M02** | Gateway / BFF | — | **M** | 4–6 | — | Connect edge + flag-enforcement middleware (the 404 security boundary) + minimal dashboard read-model plumbing (folds in M13's basic surface). Cross-cutting but not deep domain. |
| **M03** | Students / Admisión | Yes | **L** | 6–9 | — | First true vertical slice; enroll end-to-end through gateway w/ integration test. RLS per student. Carries the "prove the pipeline" weight. |
| **M04** | Cycles, roles & config | Yes | **M** | 4–6 | — | Cycle lifecycle + per-cycle config; `CycleRolled` roll-over event. Domain is moderate; the event contract is the tricky part. |
| **M05** | Finance / Adeudos | Yes | **L** | 7–10 | — | Debt + payment ledger per student, overdue detection events. Money = correctness-critical, more tests, RLS on accounts. Includes S02 CONTPAQi export. |
| **M06** | Attendance (QR) | thin | **M** | 3–5 | — | QR check-in → mark attendance, absence events. Thin service = cheaper. QR token handling is the only wrinkle. |
| **M07** | Grades / Boletas | Yes (Strategy) | **L** | 7–11 | ⚠️ | Strategy pattern per SEP level + boleta generation. SEP-level rules are a domain-knowledge unknown → soft top. |
| **M08** | Scholarships / Becas | Yes | **M** | 4–6 | — | Application/cancellation affecting finance ledger; consistency w/ M05 is the cost. |
| **M09** | Docs / Trámites | thin | **S** | 2–4 | — | Request tracking per student. Thin. |
| **M10** | Comms / Agenda / Push | thin | **M** | 4–6 | ⚠️ | Event consumer (Pub/Sub) + FCM push + agenda. FCM integration + idempotent consumers = soft. |
| **M12** | Uniforms store | thin (State) | **M** | 3–5 | — | Reservation lifecycle via State pattern; expiry event. |
| **M13** | Reports / Dashboard (Could) | — | **M** | 4–7 | ⚠️ | Stays Could. Minimal read-model plumbing folded into M02; the rich dashboard is the deferrable remainder here. Cost scales with producers present when built. |

### Deferred / future-ready (NOT in committed MVP)

| ID | Module / spike | Size | IED (low–high) | Status |
|----|----------------|------|----------------|--------|
| **M11** | Billing CFDI 4.0 + IEDU | **L** | 8–12 | **Future-ready only** this cycle: proto contract stub + `module_billing` flag (default OFF) + reserved service boundary. **No implementation.** Promote post-MVP. |
| **S01** | Spike CFDI/IEDU (PAC sandbox) | **M** | 3–5 | **Deferred** to the future billing track. Optional research, not blocking the MVP. Requires CSD del colegio as a prerequisite when the billing track activates. |

### Totals (honest ranges)

| Bucket | IED low | IED high |
|--------|---------|----------|
| Foundations + spike (M00, S02) | 13 | 19 |
| Vertical slice (M01–M03) | 17 | 25 |
| Core ops (M04–M07) | 21 | 32 |
| Should (M08–M10) | 10 | 16 |
| Could (M12, M13) | 7 | 12 |
| **Committed MVP total** | **68** | **104** |
| Deferred/future (M11 + S01) | 11 | 17 |

> At ~0.6 focus factor and a small parallel agent team, the committed **68–104 IED** maps into
> the 26-week window with **healthier margin** than the original draft — removing M11 + S01 freed
> ~11–17 IED and lightened Mes 4. Remaining soft items: M00 (first-time GCP), M07 (SEP grading rules),
> M10 (FCM idempotency).

---

## 2. Module-ordered execution plan (mapped to the 6-month calendar)

**Critical-path principle: validate the deploy pipeline before business logic.**
M00's hello-world must deploy end-to-end *before* M01 starts. Then build vertical
(M01→M02→M03) and deploy the spine *before* fanning out.

| Phase | Calendar | Modules | Goal / gate |
|-------|----------|---------|-------------|
| **Fase 0** | ~Sem 1 | M00 | Pipeline green; hello-world live on Cloud Run w/ integration test. (No blocking spikes — S01 deferred.) |
| **Mes 1** | Sem 2–5 | M01 → M02 → M03 (vertical) | Deploy the spine end-to-end (`preset_spine`): enroll a student through the gateway. Both gates pass on M01/M02/M03. M02 includes the future-ready `module_billing` flag + proto stub. |
| **Mes 2** | Sem 6–9 | M04, then M05 (incl. S02 CONTPAQi); M08 starts after M05 | Cycles + finance ledger live. `preset_core_ops` begins to fill. |
| **Mes 3** | Sem 10–13 | M06, M07; **mobile (Expo) starts consuming** frozen protos | Attendance (QR) + grades/boletas. Family-app surface emerges. |
| **Mes 4** | Sem 14–17 | M09, M12, M10 (lighter — no billing) | Docs, uniforms, comms. `preset_full_mvp` reachable. Freed margin from M11 removal absorbs M07/M10 soft-estimate slack. |
| **Mes 5** | Sem 18–21 | M13 (rich dashboard, Could) + hardening + **EAS publish** | Performance, security sweep, dashboard, store builds. |
| **Mes 6** | Sem 22–26 | — | Capacitación + soporte; flag flips per presentation presets; buffer for soft-estimate overruns. |

---

## 3. Parallelism analysis

**Routing rule recap:** parallel dispatch needs **3+ independent tasks, no shared state, clear file boundaries**. Sequential when there are dependencies, shared files, or unclear scope.

### MUST be sequential (hard dependencies / shared spine)
- **M00 → everything.** No service deploys until the pipeline exists.
- **M01 (iam) → M02 → M03.** The spine is a chain; each gates the next. Build vertical, deploy, *then* fan out.
- **M05 (finance) → M08 (scholarships), M12 (uniforms).** They write/read the finance ledger → shared state.
- **M04 (config/cycles) → M05/M06/M07.** Core ops need cycle context.
- **M13 (rich dashboard, Could)** depends on event producers existing → naturally last.
- *(M11 billing deferred; its future impl will gate on S01 verdict + CSD when the billing track activates post-MVP.)*

### CAN run in parallel (independent microservices, clear file boundaries in the monorepo)
Once the spine is deployed and contracts are frozen:

| Parallel batch | Modules | Why safe |
|----------------|---------|----------|
| **Batch B (Mes 3)** | M06 (attendance) + M07 (grades) + M09 (docs) | Separate services, separate schemas, only shared dep is M03/M04 (already frozen). ✅ |
| **Batch C (Mes 4)** | M09 docs + M12 uniforms + M10 comms | Independent services; comms is a consumer (reads events, doesn't share write-tables). ✅ 3 tasks, no billing in the mix. |
| **Frontend ‖ backend** | any module's frontend-sr | **Frontend parallelizes against a buf/proto mock server once the contract is frozen** — this is the whole point of spec-first. Frontend never blocks on backend impl, only on the frozen proto. |

### Who-builds-what (agent assignment)
| Agent | Owns |
|-------|------|
| **architect** | proto/OpenAPI contracts per module (must precede impl); ADRs; freezes the contract that unblocks frontend. |
| **db-sr** | schemas, versioned migrations, RLS policies + isolation tests. Runs slightly ahead of backend-sr per module. |
| **backend-sr** | Go services (TDD), flag middleware, event publishers/consumers, integrations. |
| **frontend-sr** | Next.js + Expo UI against proto mocks; hardened builds (no source maps); `useFlag`/`FlagsProvider`. |
| **devops-sr** | M00 pipeline, Terraform, flag infra, EAS publish, build-leak CI gate. Co-owns flag registration w/ backend-sr. |
| **qa-sr** | test pyramid, contract tests vs proto, flag-ON/OFF suites, E2E preset smokes, k6 load. |
| **security** | gates every flag + deploy; RLS audit, OWASP, secret handling. |
| **tech-lead** | canon review gate on every PR. |
| **doc-writer** | docs generated from contracts. |

> **Frontend-vs-backend ordering:** backend ALWAYS precedes frontend *for a module's
> truth*, but frontend work happens concurrently against the **frozen mock**. The
> dependency is on the contract, not the implementation.

---

## 4. Sprint 1 (2 weeks) — concrete scope

**Sprint 1 = Fase 0 Cimientos + start of M01 (iam).**
**Governing principle: validate the deploy pipeline before any business logic.**

### Sprint goal
A hello-world Connect service deployed to Cloud Run through CI/CD with a passing
integration test, the monorepo + buf + go.work + turborepo scaffolding in place,
RLS helper foundation ready, and M01 (iam) contract frozen + first red tests written.

### Tasks

| # | Task | Owner | Size | DoD |
|---|------|-------|------|-----|
| 1 | Monorepo scaffold: turborepo + go.work + buf workspace + lint/format config | devops-sr + architect | M | `buf lint` + `turbo run lint` green; layout matches Clean Arch (domain/usecase/adapter/infra). |
| 2 | CI/CD pipeline (build, test, govulncheck, build-leak gate, deploy) | devops-sr | M | Pipeline runs on PR; fails on `*.map`/sourceMappingURL; deploys to Cloud Run. |
| 3 | Terraform: Cloud Run + Cloud SQL (Postgres) + Pub/Sub topics | devops-sr | M | `terraform plan` clean; infra provisioned in a dev project. |
| 4 | Hello-world Connect service + **integration test** deployed end-to-end | backend-sr | S | Service answers through Cloud Run; integration test green in CI. ⬅️ *pipeline-validation milestone* |
| 5 | **Cloud SQL / RLS helper foundation** (session-var setter, app-role not owner / not BYPASSRLS, SET LOCAL per tx) | db-sr | M | RLS helper unit-tested; app role provisioned; one demo table proves tenant isolation. |
| 6 | M01 iam **proto contract** frozen + linted | architect | S | `module_iam` proto reviewed, frozen; unblocks frontend mock. |
| 7 | M01 iam: first **red tests** (JWT issue/validate, role/permission model) | backend-sr | S | Failing tests committed first (TDD); no production code yet beyond stubs. |

> Sprint 1 carries **no spike work** — S01 is deferred to the future billing track and S02 lives inside M05 (Mes 2). Sprint 1 stays exactly as drafted: foundation + start of iam.

### Gates required to close Sprint 1
- **tech-lead gate**: scaffolding + hello-world + RLS helper reviewed against the canon (Clean Arch layering, idiomatic Go, no AutoMigrate in shared envs).
- **security gate**: pipeline secret handling, app-role privileges (not owner / not BYPASSRLS), build-leak CI gate verified, no secrets in client bundles.
- *Note:* no flag is flipped in Sprint 1 — M00 has no flag, and M01 is not yet DoD-complete.

### Sprint 1 Definition of Done
- [ ] Hello-world deployed end-to-end with a passing integration test (pipeline validated).
- [ ] Monorepo + buf + go.work + turborepo + CI/CD scaffolding green.
- [ ] Terraform provisions Cloud Run + Cloud SQL + Pub/Sub in dev.
- [ ] RLS helper foundation in place and tested.
- [ ] M01 proto frozen; M01 first red tests committed.
- [ ] `module_billing` flag + proto stub reserved (future-ready boundary; no impl).
- [ ] tech-lead + security gates pass on the foundation.

---

## 5. Risks & dependencies register

**Top 3 MVP risks:** R1 Cloud SQL/RLS helper · R2 SEP grading-rules source (M07) · R3 focus-factor / parallel-team assumption.

| # | Risk / dependency | Impact | Likelihood | Owner | Mitigation |
|---|-------------------|--------|------------|-------|------------|
| R1 | **Cloud SQL / RLS helper is foundational** — everything tenant-scoped depends on it | High | Low | db-sr | Built in Sprint 1, gated by security. A bug here cascades to every module. |
| R2 | **M07 SEP-level grading rules** source is a domain unknown | Med | Med | architect | Confirm/document the Strategy-per-level rules with the school before Mes 3; soft top reflects this. |
| R3 | **0.6 focus-factor / small parallel-team assumption** underpins the calendar | Med | Med | eng-manager | Re-validate after Sprint 1's actuals; if focus or team size is lower, re-cut Could items (M12/M13) first. |
| R4 | **M00 first-time GCP wiring** overruns (XL, soft) | Med | Med | devops-sr | Buffer in estimate; if it slips, M01 contract+tests proceed in parallel (no Cloud dep). |
| R5 | **Finance ledger (M05) is shared state** for M08/M12 | Med | Med | backend-sr | Freeze finance event/ledger contract early; downstream modules consume, never co-write. |
| R6 | **FCM/comms idempotency** (M10) | Med | Low | backend-sr | Idempotent consumers + dedupe keys; test with replayed events. |
| R7 | **S02 CONTPAQi field mapping** uncertain (in M05) | Low | Med | backend-sr | Timeboxed inside M05; in scope. |
| F1 | *(future track)* **CSD del colegio** is a prerequisite for billing | — | — | PM | Not an MVP blocker. Procure only when the post-MVP billing track (M11 + S01) activates. |

**Blocking chain summary:** RLS helper → all RLS modules. M00 → all. iam → all. finance → {scholarships, uniforms}. *(Future: CSD → S01 → M11, post-MVP only.)*

---

## 6. Critical-path Gantt-ish view (by month)

```
        Mes0/1          Mes 2          Mes 3          Mes 4          Mes 5          Mes 6
        Sem1  2-5       6-9            10-13          14-17          18-21          22-26
        ┌────┬──────────┬──────────────┬──────────────┬──────────────┬──────────────┬─────────┐
M00     │████│          │              │              │              │              │         │  infra (pipeline first)
        │    │          │              │              │              │              │         │
M01 iam │    │████████  │              │              │              │              │         │ ┐
M02 gw  │    │     ████ │              │              │              │              │         │ │ VERTICAL SLICE
M03 stu │    │       ███│█             │              │              │              │         │ ┘ → DEPLOY (preset_spine)
        │    │          │              │              │              │              │         │
M04 cfg │    │          │████          │              │              │              │         │ ┐
M05 fin │    │          │  ██████      │              │              │              │         │ │ CORE OPS (fan-out)
S02     │    │          │  ░░          │              │              │              │         │ │ CONTPAQi inside M05
M08 bec │    │          │      ████    │              │              │              │         │ ┘ (M08 after M05)
        │    │          │              │              │              │              │         │
M06 att │    │          │              │████          │              │              │         │ ┐ parallel batch
M07 grd │    │          │              │██████        │              │              │         │ ┘ (Mes 3)
mobile  │    │          │              │ ░░░░░░░░░░░░░░│░░░░░░░░░░░░░░│░░░░░░         │         │  Expo consumes frozen protos
        │    │          │              │              │              │              │         │
M09 doc │    │          │              │              │███           │              │         │ ┐
M12 uni │    │          │              │              │ ████         │              │         │ │ Should/Could (lighter
M10 com │    │          │              │              │  ████        │              │         │ ┘  Mes 4 — no billing)
        │    │          │              │              │              │              │         │
M13 rpt │    │          │              │              │              │████          │         │  rich dashboard (Could)
harden  │    │          │              │              │              │████████      │         │  perf + security sweep
EAS pub │    │          │              │              │              │      ██      │         │  store builds
train   │    │          │              │              │              │              │█████████│  capacitación + soporte
        └────┴──────────┴──────────────┴──────────────┴──────────────┴──────────────┴─────────┘

Legend: ████ active build   ░░░░ background/parallel   → deploy milestone
Critical path: M00 → M01 → M02 → M03 (deploy) → M04 → {M05,M06,M07}. Everything else hangs off M03+M05.
Deferred (post-MVP, not shown): M11 billing + S01 spike — future-ready boundary only this cycle.
```

---

## Appendix — checkpoint resolutions (2026-06-11)
1. **CSD del colegio** — no longer an MVP concern; it is a prerequisite only for the deferred billing track (M11 + S01). Procure when that track activates post-MVP.
2. **0.6 focus factor / small parallel team** — assumption retained; tracked as risk R3 and re-validated after Sprint 1 actuals.
3. **SEP-level grading rules (M07)** — open dependency; confirm/document with the school before Mes 3 (risk R2).
4. **Billing/CFDI (M11) + S01** — out of committed MVP; future-ready boundary only (proto stub + flag OFF + reserved service). No alternate-PAC decision needed this cycle.
