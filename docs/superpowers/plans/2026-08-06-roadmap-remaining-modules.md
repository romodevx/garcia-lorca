# Roadmap — Remaining Modules (post-spine), phased

**Date:** 2026-08-06
**Status:** Proposed sequencing — confirm order, then execute phase by phase.

The academic spine is complete and on `master` (login → cycle/catalog → teachers/assignments → enrollment/transfer → grade capture). This roadmap sequences the **remaining modules** into phases and does them **all**. Each phase follows the established rhythm: its **own integration branch off `dev`**, **backend → gateway → web sub-phases** (TDD), two-stage review per task (spec + tech-lead/security), **QA + security gates**, a **PR → `dev`** for your review, then squash-merge → `dev` + `--ff-only` sync → `master`. Each phase gets its **own detailed plan** (with the phase-start decisions resolved) when we start it — this doc is the sequence, not the per-phase detail.

Every module ships behind its `module_<name>` flag, **default OFF**, enforced at the gateway (OFF ⇒ 404); RLS on every sensitive table; cross-service references validated by RPC (no cross-service FKs).

---

## Phase order & rationale (dependency- and value-driven)

| Phase | Module(s) | Flag | Depends on | Why here |
|-------|-----------|------|-----------|----------|
| **6** | **Roles & Users** (auth expansion, area 11) | `module_iam` (extend) | Fase 1 (iam) | **Foundational.** Real multi-user + roles/permissions CRUD + **teacher↔user link** — closes the tracked authz gap (today any `grades:write` holder can capture) and lets every later module scope by real user/role. Do it first so the rest builds on real identity. |
| **7** | **Finance / Adeudos** (M05) + **Scholarships** (M08) sub-slice + **CONTPAQi export** (S02) | `module_finance` (+ `module_scholarships`) | M01, M03, M04 (done) | **Top school ask** (cuotas, conceptos dinámicos, pagos en ventanilla, descuentos, migrar cargos por ciclo). Scholarships affects the ledger → folded in right after the finance core. |
| **8** | **Attendance (QR)** (M06) | `module_attendance` | M01, M03, M04 (done) | Independent (deps done); **default-present** pass list; emits `AbsenceRecorded` which Comms consumes → must precede Comms. |
| **9** | **Comms / Agenda / Push + Family app** (M10) | `module_comms` | M01, M03, **M06** | The parent-facing side: FCM push, **mensajes personalizados**, **calendario/agenda**, real-time-ish payment/absence notifications, the **Expo family app**. Consumes attendance (and finance) events → after Phase 8. |
| **10** | **Boleta** (M07 remainder) | `module_grades` (extend) | Fase 5 (grades, done) | Capstone of grade capture: **Strategy per SEP level** (numeric primaria/secundaria vs. qualitative kinder/prepa), `GradesPublished` publish flow, boleta **PDF**. No new deps — could slot earlier, placed here as it's lower-urgency than finance/attendance for the school. |
| **Later batch** | **Docs/Trámites** (M09), **Uniforms** (M12), **Reports/Dashboard** (M13) | `module_docs`, `module_uniforms`, `module_reports` | M03 / M05 / all producers | **Should/Could.** Docs is thin; Uniforms needs finance; Reports consumes every event → naturally last (needs the other producers live). |

**The near-term "5":** Phases 6–10 (Roles/Users, Finance, Attendance, Comms/Family, Boleta). The Later batch (Docs, Uniforms, Reports) completes "todas" after those land.

**Out of MVP (unchanged):** M11 Billing CFDI/IEDU + spike S01 stay deferred/future-ready (stub contract + OFF flag only).

---

## Per-phase scope sketch (detail comes in each phase's own plan)

### Phase 6 — Roles & Users
- **iam**: `CreateUser`/`ListUsers`/`SetUserRoles`/`SuspendUser` + role CRUD (`CreateRole`/`ListRoles`/`SetRolePermissions`) RPCs (the admin surface iam left "not frozen"). Password set/reset for new users (temp-password + forced change, reusing Phase 1's flow). **Link a `config.teachers.user_id` to an iam user** (the reserved column) → per-teacher scoping.
- **gateway**: `/api/v1/users`, `/api/v1/roles` behind `module_iam` + an `iam:admin`/`users:*` permission set; the owner gets them.
- **web**: users & roles admin screens (invite user, assign roles, link a teacher to a user).
- **Decisions at phase start:** exact permission taxonomy (resource:action catalog); whether grade capture becomes teacher-scoped now (principal must own the assignment) or stays open until the mobile/teacher login exists.

### Phase 7 — Finance / Adeudos (+ Scholarships, + CONTPAQi)
- **finance** (new Clean-Arch service): `finance_accounts` per student + **dynamic charge concepts** (conceptos dinámicos), charges per cycle (migrate/generate charges on enrollment/cycle roll), payments (ventanilla: cash/card/transfer + reference), balances/adeudos, overdue detection (`DebtOverdue`), discounts. Validates student/cycle via RPC (students/config). Emits `PaymentRegistered`.
- **scholarships** (M08) sub-slice: apply/cancel a scholarship that adjusts the ledger.
- **S02**: a CONTPAQi-format XML export of the ledger (validated sample).
- **gateway/web**: charges & payments screens (fewest clicks: register a window payment; CRUD charges), student account statement.
- **Decisions:** charge-concept model (fixed catalog vs free-form), how charges are generated per cycle, discount model, money type (integer cents).

### Phase 8 — Attendance (QR)
- **attendance** (thin service): mark attendance per (enrollment, date) — **default all present, only mark the absentees**; QR check-in path; emits `AttendanceMarked`/`AbsenceRecorded`. Validates enrollment/group via RPC.
- **gateway/web**: a per-group daily pass-list (default present, toggle absentees); QR endpoint.
- **Decisions:** attendance granularity (per day vs per class/subject); QR token model.

### Phase 9 — Comms / Agenda / Push + Family app
- **comms** (thin service, event consumer): subscribes to `AbsenceRecorded`/`PaymentRegistered`/etc. via Pub/Sub → **FCM push**; **mensajes personalizados** (targeted messages to guardians); **calendario/agenda** (events). 
- **mobile (Expo family app)**: the parent-facing app (attendance, grades, avisos, agenda, account) — today mock; wire it to the gateway.
- **Decisions:** notification templates/targeting; FCM setup; guardian↔student data model; how the family app authenticates.

### Phase 10 — Boleta
- **grades** (extend): **Strategy per SEP level** to compute the report card from `grade_entries`; `GradesPublished` publish flow (sets `published_at`); **PDF** boleta (via `pkg/pdf`).
- **gateway/web**: generate/preview/publish boleta; family app shows the published boleta.
- **Decisions:** per-level grading scheme (numeric scales, qualitative kinder/prepa), boleta layout, publish semantics (per group/term).

### Later batch
- **Docs/Trámites (M09)**: request/track documents & procedures per student.
- **Uniforms (M12)**: reservation store (State pattern), expiry events; needs finance.
- **Reports/Dashboard (M13)**: cross-module read-models / dashboard; consumes all events → last.

---

## Cross-cutting, per phase (Definition of Done)
Same as Phases 2–5: OpenAPI/proto contract additive + buf lint/breaking clean; RLS + isolation tests on every new sensitive table; cross-service validation by RPC; flag ON/OFF tests (OFF ⇒ 404); ≥80% coverage on business logic; web flag-gated + Cmd+K + no source maps; two-stage review per task; QA + security gates; MODULES.md/FLAGS.md updated; PR → `dev` for review; then merge/sync.

Also carry the standing **pre-shared-env checklist** (`deploy/README.md`): govulncheck toolchain bump, services `ingress=internal`, multi-tenant `TenantID` forwarding, `TRUST_PROXY`/`max-instances=1`.

---

## Execution note
We execute **one phase at a time**, each ending in a PR you review before it merges to `dev`. New services (finance, attendance, comms) get scaffolded like `config` did (own Go module + compose + go.work). Event-driven modules (attendance→comms) introduce the **first Pub/Sub consumer** (the emulator is already in compose). The family app (Phase 9) is the first real work in `apps/mobile`.
