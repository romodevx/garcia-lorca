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
| M01 | **IAM (auth/roles/permisos)** | iam | 11 | **Yes** (permisos) | **Must** | M00 | — | `users`, `roles`, `user_roles` | `module_iam` | JWT issuance/validation, role+permission model; everything downstream authorizes against it. RLS session vars set per transaction. |
| M02 | **Gateway / BFF** | gateway | 10 | — | **Must** | M01 | — | — | `module_gateway` | Connect edge; **feature-flag middleware** (disabled module → 404); read-model plumbing for dashboard. The flag boundary lives here. |
| M03 | **Students / Admisión** | students | 1 | **Yes** | **Must** | M01, M02 | `StudentEnrolled`, `AdmissionStatusChanged` | `students` | `module_students` | First vertical slice: enroll a student end-to-end through the gateway with integration test. RLS per student. |

### Core operations (Must — the school runs on these)

| ID | Module | Microservice | Area | Clean Arch | MoSCoW | Depends on | Events published | RLS-sensitive tables | Flag key | DoD reminder |
|----|--------|--------------|------|-----------|--------|-----------|------------------|---------------------|----------|--------------|
| M04 | **Cycles, roles & config** | config | 11 | **Yes** | **Must** | M01 | `CycleRolled` | _(config tables; tenant-scoped)_ | `module_config` | School-cycle lifecycle + per-cycle config; `CycleRolled` drives roll-over across modules. (Sub-area of area 11 alongside M01.) |
| M05 | **Finance / Adeudos** | finance | 2 | **Yes** | **Must** | M01, M03, M04 | `PaymentRegistered`, `DebtOverdue` | `finance_accounts` | `module_finance` | Debt + payment ledger per student; overdue detection emits events. RLS on financial accounts. |
| M06 | **Attendance (QR)** | attendance | 5 | thin | **Must** | M01, M03, M04 | `AttendanceMarked`, `AbsenceRecorded` | `attendance` | `module_attendance` | QR check-in marks attendance; absence events feed comms. RLS per student attendance record. |
| M07 | **Grades / Boletas** | grades | 6 | **Yes** (Strategy por nivel SEP) | **Must** | M01, M03, M04 | `GradesPublished` | `grades` | `module_grades` | Grade capture + boleta generation with **Strategy per SEP level**; publishing emits event. RLS per student grades. |

### Extended value (Should — ships within MVP if spine lands on time)

| ID | Module | Microservice | Area | Clean Arch | MoSCoW | Depends on | Events published | RLS-sensitive tables | Flag key | DoD reminder |
|----|--------|--------------|------|-----------|--------|-----------|------------------|---------------------|----------|--------------|
| M08 | **Scholarships / Becas** | scholarships | 3 | **Yes** | **Should** | M01, M03, M05 | `ScholarshipApplied`, `ScholarshipCancelled` | _(scholarship rows reference students; tenant-scoped)_ | `module_scholarships` | Scholarship application/cancellation affecting finance; consistent with finance ledger. |
| M09 | **Docs / Trámites** | docs | 4 | thin | **Should** | M01, M03 | `DocumentRequested` | _(document requests reference students; tenant-scoped)_ | `module_docs` | Document/procedure requests tracked per student. |
| M10 | **Comms / Agenda / Push** | comms | 8 | thin | **Should** | M01, M03, M06 | _(consumer)_ — pushes FCM | — | `module_comms` | Consumes events (e.g. `AbsenceRecorded`) and pushes FCM notifications + agenda. Consumer of many modules' events. |

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
