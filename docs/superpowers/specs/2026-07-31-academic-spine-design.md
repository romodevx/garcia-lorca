# Design Spec — Academic Spine (Login → Cycle → … → Grades)

**Date:** 2026-07-31
**Status:** Approved (design), pending implementation-plan
**Scope owner:** single "director/owner" role for now (roles/permissions/multi-user come later)
**Related:** `.claude/docs/MODULES.md` (M01, M02, M03, M04, M07), `.claude/docs/FLAGS.md`

---

## 1. Goal

Make the system usable end-to-end from a single owner account, exercising the full
relational spine of a school-management system:

```
owner login → school cycle → levels → grade-levels → groups → subjects → teachers
  → teaching assignment (teacher + group + subject) → student enrolled into a group
  → teacher sees their roster (by assignment) → teacher captures grades
```

Everything downstream (finance, attendance, comms, scholarships) hangs off this spine
and is **out of scope for this plan** (reserved, not built here).

## 2. Non-negotiable constraints (from repo canon)

- **TDD strict:** failing test first, every unit. No production code without a prior red test.
- **Per-module order:** backend (tests) → frontend (tests) → feature flag (default OFF).
- **Feature flag** `module_<name>`, enforced server-side at the gateway (disabled → 404).
- **GORM only in `adapters/postgres`;** domain imports no framework. Double model + `mapper.go`.
- **RLS** on tenant-scoped tables: `ENABLE + FORCE`, `SET LOCAL` per transaction.
- **One shared Postgres** (`garcia_lorca`), hardened `app` role; migrations per module folder.
  Cross-service references are **UUID + RPC validation**, never cross-service FKs.
- **`gen/` is generated;** change `.proto` → `buf generate`. Never hand-edit `gen/`.
- **pnpm only** for JS.

## 3. Current state (verified 2026-07-31)

**Real code:** `iam`, `students`, `gateway`. Everything else is `doc.go` stubs.

- **iam** — Login/RefreshToken/ValidateToken/CheckPermission/AssignRole/RevokeRole all wired.
  bcrypt hasher, HMAC JWT (access 15m / refresh 24h, refresh rotated). Single-tenant
  (`IAM_TENANT_ID` default `…0001`). **No CreateUser/ChangePassword, no user seed** →
  login is currently unusable without manual DB inserts.
- **students** — CreateApplicant/GetStudent/ListStudents/ChangeAdmissionStatus wired.
  `students.grade_group` is a **free-text** column ("4° A"); no link to cycles/groups.
- **gateway** — real `RequireAuth` (Bearer JWT → iam ValidateToken → `Principal` in ctx),
  `RequirePermission`, `ModuleGuard` (404 when flag off), `/api/v1/features`,
  `/api/v1/auth/{login,refresh}`, `/api/v1/me`, `POST /api/v1/students`.
  Bearer-only, no cookies/session. Only `students` create is routed.
- **web** — Next.js App Router; real UI is a client SPA in `app/AppShell.tsx`.
  **Login is fake** — a role-picker (`admin`/`teacher`) that calls `onLogin(role)` with
  no fetch/token. `app/(auth)/login/page.tsx` is a stub. Data is mock (`app/_design/data.ts`).
- **config service:** does **not exist** (no dir, no proto, no migration). Levels exist only
  as an enum/CHECK on `students.level`. No grades-catalog, groups, subjects, teachers, terms.
- **pkg/authctx** binds `app.current_user`; migrations key RLS on `app.current_tenant`;
  **authctx is unused** — each service rolls its own `set_config('app.current_tenant', …)`.

### 3.1 Defects this spec must fix

1. **RLS session-var mismatch.** Standardize on `app.current_tenant` for tenant isolation.
   Consolidate the binding into a single shared helper (`pkg/authctx`) used by every service;
   `app.current_user` may be added later for row-owner policies but is not required now.
2. **`students.grade_group` free text** is superseded by `enrollments → groups`. Reconcile
   via migration; keep the column deprecated-nullable until data is migrated, then drop.

## 4. Service topology

No new services beyond the one already reserved in the layout.

| Service | Bounded context | Added in this plan |
|---|---|---|
| **iam** (M01) | Identity | Owner seed (temp password + `must_change_password`); `ChangePassword` RPC; login signals `password_change_required`. No user CRUD yet. |
| **gateway** (M02) | Edge/BFF | New routes + `ModuleGuard` for `config` and `grades`; route the students Get/List/enrollment endpoints. |
| **config** (M04) — **new** | Academic structure | cycles, levels, grade_levels, groups, subjects, terms, **teachers**, **teaching_assignments**. |
| **students** (M03) | Students | **enrollments** (place a student in a group of a cycle); transfer history reserved. |
| **grades** (M07) | Grades | grade capture keyed by (enrollment, subject, term, teacher); boleta via Strategy-per-SEP-level. |

**Design decision — teachers live in `config`, not `iam`.** A teacher is a **catalog record
(person)**, not (yet) a login account. `teachers.user_id` is reserved (NULL) for a future
link to an IAM user. This lets the owner register teachers and assign them to groups without
building the user/role system first.

## 5. Relational model

Within a service: real FK. Across services: UUID validated by RPC (shown `┈┈▷`).
All tables tenant-scoped with `ENABLE + FORCE RLS` and the `tenant_isolation` policy.

### 5.1 `config` — academic structure

`cycles` is the per-cycle umbrella; `levels` is a cross-cycle catalog. They meet at the
per-cycle tables (`terms`, `groups`, `teaching_assignments`), each of which carries both a
`cycle_id` and a level-derived reference.

Legend: `──▶` real FK (same service); `[cycle]` = row carries `cycle_id` (per-cycle table).

```
CROSS-CYCLE CATALOG                         PER-CYCLE (carry cycle_id)
──────────────────                          ─────────────────────────
levels                                      cycles
  id, tenant_id, name, sep_code,              id, tenant_id, name "2026-2027",
  enrollment_granularity (CYCLE|TERM)         status (draft|active|closed),
  UNIQUE(tenant_id, name)                     starts_on, ends_on
  │  bachillerato = TERM, others = CYCLE      UNIQUE(tenant_id, name)
  │
  ├──▶ grade_levels (grados)
  │      id, tenant_id, level_id,
  │      name "1° primaria", ordinal
  │      UNIQUE(tenant_id, level_id, ordinal)
  │        │
  │        ├──▶ subjects (materias)          per grade_level
  │        │      id, tenant_id, grade_level_id, name "Matemáticas"
  │        │      UNIQUE(tenant_id, grade_level_id, name)
  │        │
  │        └──▶ groups (grupos) [cycle]      per (cycle, grade_level)
  │               id, tenant_id, cycle_id, grade_level_id,
  │               name "A", capacity
  │               UNIQUE(tenant_id, cycle_id, grade_level_id, name)
  │
  └──▶ terms (periodos) [cycle]              per (cycle, level)
         id, tenant_id, cycle_id, level_id, name "T1"/"2026-A",
         ordinal, starts_on, ends_on
         UNIQUE(tenant_id, cycle_id, level_id, ordinal)

teaching_assignments [cycle]  (teacher ↔ group ↔ subject ↔ cycle)  ← central knot
  id, tenant_id, cycle_id, group_id──▶groups, subject_id──▶subjects, teacher_id──▶teachers
  UNIQUE(tenant_id, cycle_id, group_id, subject_id)
  # Implemented in Phase 3. Two invariants are APPLICATION-enforced (not DB constraints),
  # safe because grade_level_id on groups/subjects is immutable (no update RPC):
  #   (a) cycle_id is DERIVED from group.cycle_id at create time (never client-supplied);
  #   (b) subject.grade_level_id must equal group.grade_level_id (teachability check).
  # DB backstops: FK RESTRICT/CASCADE + UNIQUE(cycle,group,subject). If groups/subjects
  # ever become mutable, promote (a)/(b) to composite-FK / trigger enforcement.

teachers (maestros)
  id, tenant_id, first_name, last_name, email, user_id (NULL, reserved for IAM link)
  UNIQUE(tenant_id, email)
```

**Cycle model — hybrid (approved).** One annual `cycle` is the umbrella ("current cycle" is
always singular for dashboards/finance). Each **level** declares `enrollment_granularity`:
`CYCLE` (K-12: one enrollment + one inscription charge per cycle) or `TERM` (bachillerato:
enrollment + charge per semester). `terms` carry the per-level calendar; for `CYCLE` levels
terms are evaluation-only, for `TERM` levels they drive enrollment/charging too.

> **Note on levels scope.** `levels` and their `grade_levels`/`subjects` are treated as a
> catalog reusable across cycles; `groups`, `terms`, and `teaching_assignments` are per-cycle.
> This keeps roll-over cheap: new cycle → new groups/terms/assignments, same catalog.

### 5.2 `students` — enrollment (UUID refs to config)

```
students (exists)
  id, tenant_id, curp, first_name, last_name, admission_status, …
    │ 1─N
    ▼
enrollments
  id, tenant_id, student_id(FK),
  cycle_id  ┈┈▷ config.cycles,  group_id ┈┈▷ config.groups,
  term_id   ┈┈▷ config.terms (NULL when the level's granularity = CYCLE),
  status (active|transferred|withdrawn), enrolled_at
  UNIQUE(tenant_id, student_id, cycle_id)   -- one enrollment per student per cycle
```

**Reserved (not built here):** `enrollment_transfers(enrollment_id, from_group_id,
to_group_id, reason, moved_at)` — the "historial de traslados entre grupos con fecha y
motivo" requested by the school. Spec'd now, implemented in a later phase.

### 5.3 `grades` — capture (UUID refs)

```
grade_entries
  id, tenant_id,
  enrollment_id ┈┈▷ students.enrollments,
  subject_id    ┈┈▷ config.subjects,
  term_id       ┈┈▷ config.terms,
  teacher_id    ┈┈▷ config.teachers,
  value, published_at
  UNIQUE(tenant_id, enrollment_id, subject_id, term_id)
```

**Strategy per SEP level** lives in the `grades` domain when computing the boleta
(e.g. numeric scale for primaria/secundaria vs. a different scheme for kinder/prepa).

### 5.4 Referential integrity across services

Cross-service UUIDs are validated at write time by RPC to the owning service (e.g. `students`
enrollment validates `group_id`/`term_id` against `config`; `grades` validates
`enrollment_id`/`subject_id`/`term_id`/`teacher_id`). No cross-service FKs; each service owns
its schema. The sequential flow is thereby enforced: no group without a grade-level, no
assignment without group+subject+teacher, no enrollment without a group, no grade without
enrollment+subject+term.

## 6. Auth flow — owner first login

1. **Migration seeds** the owner into `iam.users` with a bcrypt hash of a **temporary
   password** and `must_change_password = true` (new column). One `owner`/`director` role
   row with the permission set needed for this spine.
2. **Login** (`POST /api/v1/auth/login`): on valid credentials where
   `must_change_password = true`, the response signals `password_change_required` (tokens
   scoped/limited so the only allowed action is changing the password).
3. **ChangePassword** (new iam RPC + gateway route): sets the new hash, clears the flag,
   issues normal tokens.
4. **Web** replaces the fake role-picker with a real login form calling the gateway, storing
   the JWT, and routing to the password-change screen when required.

Decisions: temp-password-in-migration + forced change (approved). Single tenant/owner for now.

## 7. Feature flags

New/affected keys (all default OFF; server-enforced at gateway):
`module_iam`, `module_gateway`, `module_students` (exist), `module_config` (new wiring),
`module_grades` (new wiring). A demo preset `preset_academic_spine` turns on
`iam + gateway + students + config + grades`. Registered in `FLAGS.md` + migration.

## 8. UX principle — fewest clicks (school's ask)

- Sensible defaults everywhere (e.g. group name auto-suggested `A/B/C`; enrollment prefilled
  with the active cycle; grade capture as an inline editable roster grid).
- Every entity registered in the Cmd+K command palette (repo DoD): nav + create + quick actions.
- "Default all present" (attendance) and dynamic charge concepts are **later phases** — noted
  so the model doesn't preclude them, not built here.

## 9. Phased delivery (each phase: backend TDD → frontend → flag → your checkpoint)

1. **Login + owner.** Owner seed migration, `must_change_password`, `ChangePassword` RPC +
   route, real web login replacing the role-picker. *Checkpoint: real director login.*
2. **Academic catalog (`config`).** cycles → levels → grade_levels → groups → subjects →
   terms. New service scaffold (Clean Arch), proto, migrations+RLS, gateway routes, admin
   screens. Flag `module_config`.
3. **Teachers + assignment (`config`).** teachers CRUD + `teaching_assignments` (the knot).
4. **Enrollment (`students`).** `enrollments` table + RPCs; place students into groups;
   deprecate `grade_group`. Route Get/List/enroll through gateway.
5. **Grade capture (`grades`).** Teacher roster-by-assignment + `grade_entries` capture,
   Strategy-per-SEP-level boleta scaffold. Flag `module_grades`.
   *Checkpoint: end-to-end demonstrable system.*

## 10. Out of scope (reserved, future-ready)

Roles/permissions UI & multi-user CRUD; finance/charges & real-time payments; attendance
(default-present); comms/push & family app; scholarships; enrollment transfer history UI;
CONTPAQi export; billing/CFDI. The model leaves room for all of them; none are built here.

## 11. Testing strategy (per repo canon)

- Unit: domain logic (state machines, Strategy, granularity rules), no I/O.
- Integration: real Postgres via testcontainers-go; **RLS isolation tests** per tenant-scoped
  table (tenant A cannot see/write B).
- Contract: every endpoint against `docs/api/openapi.yaml`.
- Flag-aware: each module suite runs flag-ON and flag-OFF (OFF → 404).
- Coverage ≥ 80% on business logic; every fixed bug leaves a regression test.
