# MODULES.md — Backlog · Sistema de Gestión Escolar García Lorca

Module backlog for the school-management platform. Ordered by **build sequence /
dependency**, not by area number. Architecture mandate: build **vertical** — a thin
end-to-end slice (`iam` → `students`) deployed and integration-tested before widening.

## Conventions (apply to every module)
- **Flujo fijo por módulo**: backend (TDD, tests primero) → frontend (tests primero) → feature flag.
- **Feature flag**: every module is born behind `module_<name>`, **default OFF**. Disabled module = server returns 404 (security boundary), invisible in UI.
- **TDD estricto**: failing test first, always. No production code without a prior red test.
- **RLS**: sensitive tables get `ENABLE + FORCE ROW LEVEL SECURITY` from their first migration.
- **GORM only in adapters**; domain imports no framework.
- **Front build blindado**: minified, no source maps shipped.
- **Command Palette (cross-cutting)**: cada módulo registra sus comandos en el buscador Cmd+K
  (nav + crear + acciones rápidas + records con `Search` RPC scoped por permisos). Parte del DoD —
  un módulo que no aparece en el palette no está terminado. Ver `.claude/docs/design/PLAN-command-palette.md` y [[command-palette]].
- **DoD baseline** (in addition to the per-module reminder): OpenAPI contract frozen + linted; ≥80% coverage on business logic; flag-ON and flag-OFF suites green; RLS isolation tests where applicable; tech-lead gate + security gate both pass; docs generated from contract. Only then may the flag be enabled in a shared env.

## MoSCoW lens (6-month MVP)
- **Must** — the spine the school cannot operate without on day one.
- **Should** — high value, ships within MVP if the spine lands on time.
- **Could** — valuable but deferrable; first to be cut under pressure or external blockers.
- **Won't (for MVP)** — explicitly out of the 6-month scope (none of the 11 areas are dropped, but advanced sub-scopes are).

---

## Build sequence

### Foundations & spikes

| ID | Module | Microservice | Area | Clean Arch | MoSCoW | Depends on | Events published | RLS-sensitive tables | Flag key | DoD reminder |
|----|--------|--------------|------|-----------|--------|-----------|------------------|---------------------|----------|--------------|
| M00 | **Cimientos (Fase 0)** | platform / infra | — (foundational) | n/a | **Must** | — | — | — | _(no flag — infra)_ | Monorepo + buf + go.work + turborepo + CI/CD + Terraform GCP (Cloud Run, Cloud SQL, Pub/Sub); hello-world service deployed end-to-end with a passing integration test. |
| S02 | **Spike CONTPAQi XML** | finance (research) | 2 | n/a | **Should** _(spike)_ | M00 | — | — | _(no flag — spike)_ | Belongs to **finance (M05) export**, not billing. Produce + validate a sample CONTPAQi-format XML export; confirm field mapping feasibility. Stays in MVP scope. |
| S01 | **Spike CFDI/IEDU (PAC sandbox)** | billing (research) | 9 | n/a | **Won't (for MVP)** _(deferred research)_ | M00 | — | — | _(no flag — spike)_ | **Deferred with M11-billing — not on the MVP critical path.** Future billing-track research only: stamp a CFDI 4.0 + IEDU complement against the PAC sandbox. No CSD dependency for MVP; **CSD del colegio is a prerequisite for the future billing track only.** |

### Vertical slice (the spine — build first, deploy end-to-end)

| ID | Module | Microservice | Area | Clean Arch | MoSCoW | Depends on | Events published | RLS-sensitive tables | Flag key | DoD reminder |
|----|--------|--------------|------|-----------|--------|-----------|------------------|---------------------|----------|--------------|
| M01 | **IAM (auth/roles/permisos)** ✅ _+ admin CRUD_ | iam | 11 | **Yes** (permisos) | **Must** | M00 | — | `users`, `roles`, `user_roles` | `module_iam` | JWT issuance/validation, role+permission model; everything downstream authorizes against it. RLS session vars set per transaction. **Phase 6 (2026-08): user/role admin shipped** — CreateUser (temp password + forced change), role CRUD with a validated `resource:action` permission catalog, assign/suspend, and **teacher↔user link** (`config.teachers.user_id`, gateway-orchestrated). Admin surface behind `module_iam` + `users:*`/`roles:*`. **Deferred:** "grant only permissions you hold" (single-owner MVP ok), teacher-scoped grade capture, token revocation on suspend. |
| M02 | **Gateway / BFF** | gateway | 10 | — | **Must** | M01 | — | — | `module_gateway` | Connect edge; **feature-flag middleware** (disabled module → 404); read-model plumbing for dashboard. The flag boundary lives here. |
| M03 | **Students / Admisión** ✅ _+ enrollment_ | students | 1 | **Yes** | **Must** | M01, M02 | `StudentEnrolled`, `AdmissionStatusChanged` | `students`, `enrollments`, `enrollment_transfers` | `module_students` | First vertical slice (admission) + **Phase 4 (2026-08): enrollment shipped** — place a student in a group of a cycle (`enrollments`) with the cycle DERIVED from the group and group/term validated by RPC to `config` (first service-to-service call); **transfer between groups** with dated/reasoned history (`enrollment_transfers`, atomic) + withdraw. Gateway routes behind `module_students` + `students:enroll/read`. RLS on all three tables. |

### Core operations (Must — the school runs on these)

| ID | Module | Microservice | Area | Clean Arch | MoSCoW | Depends on | Events published | RLS-sensitive tables | Flag key | DoD reminder |
|----|--------|--------------|------|-----------|--------|-----------|------------------|---------------------|----------|--------------|
| M04 | **Cycles, roles & config** ✅ _catalog done_ | config | 11 | **Yes** | **Must** | M01 | `CycleRolled` _(pending)_ | _(config tables; tenant-scoped)_ | `module_config` | **Phase 2 (2026-08): academic catalog shipped** — `config` service (Clean Arch, RLS) with cycles/levels/grade_levels/subjects/terms/groups (hybrid `enrollment_granularity`); gateway routes behind `module_config` + `config:read/write`; web admin screens. **Phase 3 (2026-08): teachers + teaching_assignments (the knot) shipped** — teacher catalog + assign teacher↔group↔subject↔cycle (cycle derived from the group; subject must match the group's grade_level — enforced in the use case; RLS on both new tables). Enrollment (Ph4), `CycleRolled` roll-over still pending. |
| M05 | **Finance / Adeudos** ✅ _core done_ | finance | 2 | **Yes** | **Must** | M01, M03, M04 | `PaymentRegistered`, `DebtOverdue` _(overdue read only; event deferred)_ | `charge_concepts`, `charges`, `payments` | `module_finance` | **Phase 7 (2026-08): finance core shipped** — new Clean-Arch `finance` service (money = integer **cents**): dynamic **charge concepts** catalog, **charges** issued single or **generated in bulk per cycle/group** (idempotent), **window payments** (cash/card/transfer + reference/"con qué se pagó"), **discounts**, **account statement** (charges/payments/balance), **overdue** read. The **payment knot** is atomic (row lock + overpay/cancelled vetoed in-tx). `finance`→students+config RPC (validate student/cycle; expand cycle→enrollments for bulk). RLS on all three tables. Emits `PaymentRegistered`. Gateway behind `module_finance` + `finance:read/write`; web surface (charges/pago-ventanilla/conceptos/generar/estado-de-cuenta). **Deferred to Phase 7b:** Scholarships (M08) ledger integration, CONTPAQi XML export (S02, `ExportContpaqi` stub returns Unimplemented), scheduled `DebtOverdue` event emission, monthly-recurring bulk, `received_by` cashier attribution. **Hardening follow-ups (tech-lead gate 2026-08-06):** ~~(I1) payment **idempotency key**~~ **DONE 2026-08-07** (`idempotency_key` + partial-unique + replay, no double charge); (I2) move `derivePaidStatus` onto the entity so use case + integration test bind the same domain fn; (I3) `ApplyDiscount`/`CancelCharge` are two-tx read-modify-write with no row lock → generalise the payment-knot seam to `MutateCharge` (`FOR UPDATE`); (I4) concurrency test for the row lock; (I5) **partly done 2026-08-07** — `ListCharges` now requires a student/cycle filter + all list reads bounded (500, ordered); the single-`LEFT JOIN` paid-sum rewrite still pending; (I7) enforce or drop the unused `StudentRef.Status` guard. See ADR 0002. **Security pre-shared-env (B1/B2):** audit trail on money ops still pending; per-object authz on `finance:read` **mitigated 2026-08-07** (iam grant-guard blocks it on non-owner roles + bounded reads; full per-student scoping in Phase 9) — see `deploy/README.md`. **Cross-cutting done 2026-08-07:** CI live; `gormTxRunner` hoisted to `pkg/authctx/gormtx`. |
| M06 | **Attendance** ✅ _daily pass list done_ | attendance | 5 | thin | **Must** | M01, M03, M04 | `AttendanceMarked`, `AbsenceRecorded` | `attendance` | `module_attendance` | **Phase 8 (2026-08): daily pass list shipped** — thin `attendance` service (handler→service→repo): per-group **daily** roster **default-present, only exceptions stored** (`present|absent|late|excused`); **reconcile-on-save** in one tx (dropped marks revert to present); `GetDailyRoster`/`SaveDailyAttendance`/`ListAttendance`; validates group + enrollment-in-group by RPC (attendance→config+students); RLS on `attendance`; emits `AttendanceMarked`/`AbsenceRecorded` (best-effort, dev bus). Gateway behind `module_attendance` + `attendance:read/write` (iam `0008`); web default-present pass list. **Deferred to Phase 8b:** **QR check-in** (static or signed token), per-class/subject granularity, summaries/reports, scheduled `DebtOverdue`-style reminders. `AbsenceRecorded` feeds Comms in Phase 9 (must swap the dev Pub/Sub bus for Cloud Pub/Sub first — see `deploy/README.md`). |
| M07 | **Grades / Boletas** ✅ _capture done_ | grades | 6 | **Yes** (Strategy por nivel SEP) | **Must** | M01, M03, M04 | `GradesPublished` _(deferred)_ | `grade_entries` | `module_grades` | **Phase 5 (2026-08): grade capture shipped** — teacher opens a teaching_assignment → roster (students in the group, via RPC to students) → captures/edits a numeric 0–10 grade per (enrollment, subject, term); subject/teacher DERIVED from the assignment; `grades`→config+students RPC (two service-to-service seams); RLS on `grade_entries`; gateway behind `module_grades` + `grades:read/write`. **Deferred:** boleta generation + Strategy-per-SEP-level + `GradesPublished`/`published_at` publish flow; auth-principal→teacher linkage (any `grades:write` holder can capture until roles/users land). |

### Extended value (Should — ships within MVP if spine lands on time)

| ID | Module | Microservice | Area | Clean Arch | MoSCoW | Depends on | Events published | RLS-sensitive tables | Flag key | DoD reminder |
|----|--------|--------------|------|-----------|--------|-----------|------------------|---------------------|----------|--------------|
| M08 | **Scholarships / Becas** | scholarships | 3 | **Yes** | **Should** | M01, M03, M05 | `ScholarshipApplied`, `ScholarshipCancelled` | _(scholarship rows reference students; tenant-scoped)_ | `module_scholarships` | Scholarship application/cancellation affecting finance; consistent with finance ledger. |
| M09 | **Docs / Trámites** | docs | 4 | thin | **Should** | M01, M03 | `DocumentRequested` | _(document requests reference students; tenant-scoped)_ | `module_docs` | Document/procedure requests tracked per student. |
| M10 | **Comms / Agenda / Push** ✅ _event backbone done_ | comms | 8 | thin | **Should** | M01, M03, M06 | _(consumer)_ — pushes FCM | `notifications`, `devices`, `processed_events` | `module_comms` | **Phase 9a (2026-08): event backbone shipped** — the repo's FIRST async messaging: `pkg/pubsub` gained an in-memory bus + `pkg/pubsub/gcp` Cloud Pub/Sub adapter (emulator-aware); producers (finance/attendance/students) now **publish for real** (dev-bus fallback); the thin `comms` service **consumes** `AbsenceRecorded`/`PaymentRegistered`/student events via a subscriber loop (errgroup + graceful shutdown), persists a **Notification idempotently** (`processed_events` ledger — at-least-once safe), `RegisterDevice`(FCM token), `ListNotifications`/`MarkNotificationRead`; push via a **log stub** (real FCM behind env). RLS on all 3 tables. Gateway behind `module_comms` + `comms:read/write` (iam `0009`). See ADR 0003. **Deferred:** 9b guardian model + family auth (guardian=iam user + Bearer) + guardian-scoped targeting + `comms:read` per-object scoping; 9c Expo family app; 9d **mensajes personalizados** + **agenda/calendar**; real FCM (Firebase + Secret Manager). |

### Could (deferrable; first to cut under pressure or external blockers)

| ID | Module | Microservice | Area | Clean Arch | MoSCoW | Depends on | Events published | RLS-sensitive tables | Flag key | DoD reminder |
|----|--------|--------------|------|-----------|--------|-----------|------------------|---------------------|----------|--------------|
| M12 | **Uniforms store** | uniforms | 7 | thin (State) | **Could** | M01, M03, M05 | `UniformReserved`, `ReservationExpired` | _(reservations reference students; tenant-scoped)_ | `module_uniforms` | Reservation lifecycle via **State pattern**; expiry emits event. |
| M13 | **Reports / Dashboard (advanced read-models)** | gateway + read models | 10 | — | **Could** | M02 + all event producers | _(consumer of all events)_ | — | `module_reports` | Cross-module read-models / dashboard. Basic gateway read-model plumbing is in M02 (Must); the rich dashboard is here (Could). |

### Won't (for MVP) — post-MVP, future-ready (reserved, not built this cycle)

| ID | Module | Microservice | Area | Clean Arch | MoSCoW | Depends on | Events published | RLS-sensitive tables | Flag key | DoD reminder |
|----|--------|--------------|------|-----------|--------|-----------|------------------|---------------------|----------|--------------|
| M11 | **Billing CFDI 4.0 + IEDU** | billing | 9 | **Yes** (IEDU complejo) | **Won't (for MVP)** _(post-MVP, future-ready)_ | M01, M05, S01 | `InvoiceIssued` | `invoices` | `module_billing` | **NOT built this cycle. No implementation, no PAC integration.** Keep future-ready: (1) `billing/v1` proto contract defined as a **frozen stub**; (2) `module_billing` flag stays **registered, default OFF**; (3) service boundary **reserved in the monorepo layout**. Promotion to a real build is a post-MVP decision, gated by spike S01 + CSD del colegio. |

---

## Critical-path dependency chain

```
M00 Cimientos
   └─> M01 IAM ──> M02 Gateway/BFF ──> M03 Students   ← VERTICAL SLICE (deploy end-to-end here)
                          │                  │
                          │                  ├─> M04 Config (cycles) ┐
                          │                  ├─> M05 Finance ────────┤
                          │                  ├─> M06 Attendance ─────┤  (Must core ops)
                          │                  └─> M07 Grades ─────────┘
                          │
                          ├─> M08 Scholarships  (needs M05)        ┐
                          ├─> M09 Docs                              │ (Should)
                          └─> M10 Comms (needs M06)                 ┘
                          │
                          ├─> M12 Uniforms (needs M05)               ┐ (Could)
                          └─> M13 Reports  (needs all producers)     ┘

S02 CONTPAQi spike ── feeds M05 Finance export (in MVP scope)

— OUT OF MVP (post-MVP, future-ready / reserved) —
M11 Billing CFDI/IEDU  ← stub contract + registered flag + reserved boundary only
S01 CFDI/IEDU spike    ← deferred research for the future billing track (CSD prerequisite there, not in MVP)
```

**Longest must-have path:** M00 → M01 → M02 → M03 → {M04 → M05/M06/M07}. Everything Should/Could hangs off M03 + M05. Billing (M11) and its CFDI/IEDU spike (S01) are out of MVP and off the critical path.
