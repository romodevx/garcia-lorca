# Phase 3 — Teachers + Teaching Assignments (the knot) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]` checkboxes.

**Goal:** Extend the existing `config` service with **teachers** (catalog records) and **teaching_assignments** — the central knot binding *teacher ↔ group ↔ subject ↔ cycle*. This is what later lets a teacher see their groups and (Phase 5) capture grades.

**Architecture:** EXTEND the `config` service (do NOT create a new one). Same Clean Arch, same `pkg/authctx` RLS, same `ConfigService` proto (additive RPCs). Teachers are catalog **person records**, NOT login accounts — a nullable `user_id` is reserved for a future IAM link (Phase: roles/users). All new tables live in `config`, so `teaching_assignments`' references are **real FKs**.

**Tech Stack:** Go 1.24, GORM (adapters only), Connect/gRPC, Postgres 16 + RLS, buf, Next.js, testcontainers-go.

**Design source:** `docs/superpowers/specs/2026-07-31-academic-spine-design.md` §5.1 (teachers + teaching_assignments).

---

## Delivery model

Integration branch `feature/phase3-teachers` (off `dev`, created). Three sub-phases, each its own branch off it, merged back `--no-ff`, with a checkpoint:

| Sub-phase | Branch | Content |
|-----------|--------|---------|
| **3.1 Backend** | `feature/phase3-teachers-be` | migration+RLS (2 tables) + proto (additive) + domain + usecase (the knot validation) + adapter (RLS IT) + handler. |
| **3.2 Gateway** | `feature/phase3-teachers-gw` | REST routes for teachers + assignments behind `module_config` + `config:read/write` (reuse existing permissions — no new ones). |
| **3.3 Web** | `feature/phase3-teachers-web` | Teachers CRUD + "assign teacher to group+subject" screen; Cmd+K. |

**Per-sub-phase DoD:** gofmt/vet clean; unit + integration (RLS isolation on the 2 new tables) green; buf lint + breaking vs `dev` (additive only); flag-ON/OFF for the gateway routes; no secret literals; tech-lead (+ security for the flag) gates. `gen/`/`go.work.sum` never staged. buf via Docker; base branch `dev`. **Never let `go work sync`/`go mod tidy` bump the workspace to go 1.25 — keep `go 1.24`.**

---

## Relational model → DDL (sub-phase 3.1)

Two new tenant-scoped, `ENABLE + FORCE RLS` tables in `config` (policy `tenant_isolation` on `app.current_tenant`, identical shape to `migrations/config/0001_init.sql`).

```
teachers (catalog person; NOT a login account)
  id, tenant_id, first_name, last_name, email,
  user_id uuid NULL,               -- reserved for a future IAM link; unused now
  created_at, updated_at
  UNIQUE(tenant_id, email)

teaching_assignments  (the knot: teacher ↔ group ↔ subject ↔ cycle)
  id, tenant_id,
  cycle_id   → cycles       ON DELETE CASCADE,   -- derived from the group at create time
  group_id   → groups       ON DELETE RESTRICT,
  subject_id → subjects     ON DELETE RESTRICT,
  teacher_id → teachers     ON DELETE RESTRICT,
  created_at, updated_at
  UNIQUE(tenant_id, cycle_id, group_id, subject_id)   -- one teacher per (group, subject) in a cycle
```

Indexes on every FK + `(tenant_id)` + `(tenant_id, teacher_id)` (for "assignments of a teacher") + `(tenant_id, cycle_id)`.

**The knot invariants** (enforced in the use case — this is the "todo bien relacionado" the school cares about):
1. **group belongs to the cycle** — the assignment's `cycle_id` is DERIVED from `group.cycle_id` (never taken from the client), so it can't drift.
2. **subject is teachable for that group** — `subject.grade_level_id == group.grade_level_id` (you can't assign a 1°-primaria subject to a 2°-secundaria group) → else `KindInvalid`.
3. **teacher exists** (same tenant) → else `KindInvalid`.
4. DB `UNIQUE` backstops "one assignment per (cycle, group, subject)" → duplicate → `KindConflict`.

---

# Sub-phase 3.1 — Backend (`feature/phase3-teachers-be`)

Branch: `git checkout -b feature/phase3-teachers-be feature/phase3-teachers`.

## Task 3.1.1: Migration + RLS
- [ ] Create `migrations/config/0002_teachers_assignments.sql`: the two tables above, uuid PKs (`gen_random_uuid()`), `created_at/updated_at timestamptz default now()`, the FKs with the stated ON DELETE rules, the UNIQUE constraints, the indexes, and for BOTH tables `ENABLE + FORCE ROW LEVEL SECURITY` + a `tenant_isolation` policy (copy the shape from `0001_init.sql`). `BEGIN; … COMMIT;`. No seed.
- [ ] Verify: `docker compose down -v && docker compose up -d postgres && docker compose run --rm migrate`, `psql \dt` shows `teachers` + `teaching_assignments`; RLS on (unset tenant → 0 rows). Tear down.
- [ ] Commit: `feat(config): schema + RLS for teachers & teaching_assignments`.

## Task 3.1.2: Proto (additive)
- [ ] Extend `proto/garcialorca/config/v1/config.proto` `ConfigService` with: `CreateTeacher/GetTeacher/ListTeachers`; `CreateTeachingAssignment/GetTeachingAssignment/ListTeachingAssignments(cycle_id[, group_id, teacher_id])`. Add `Teacher` and `TeachingAssignment` messages + the Create/Get/List request/response messages (mirror the existing entities' message style, contiguous field numbers appended at the end). `Teacher` includes `user_id` (string, may be empty). `CreateTeachingAssignmentRequest` takes `group_id, subject_id, teacher_id` (NO `cycle_id` — the server derives it from the group); the response `TeachingAssignment` includes the derived `cycle_id`.
- [ ] `buf generate` + `buf lint` + `buf breaking --against '.git#branch=dev'` (additive → clean) + `cd gen/go && go build ./...`.
- [ ] Commit: `feat(config): proto — teachers & teaching-assignment RPCs`.

## Task 3.1.3: Domain
- [ ] `services/config/internal/domain/teacher.go` (+ test): `NewTeacher(id, tenantID, firstName, lastName, email, userID string) (Teacher, error)` — validate id/tenantID/firstName/lastName/email non-empty; reuse the email validation idiom from `services/iam/internal/domain/email.go` if trivial, else a simple non-empty + contains-"@" check; `userID` may be "" (accessor `UserID()`; `HasUser()` convenience). Mirror the `Cycle` template.
- [ ] `services/config/internal/domain/teaching_assignment.go` (+ test): `NewTeachingAssignment(id, tenantID, cycleID, groupID, subjectID, teacherID string) (TeachingAssignment, error)` — validate all ids non-empty. (Cross-entity relational invariants live in the use case, not here — the domain can't see other aggregates.)
- [ ] TDD red→green; commit: `feat(config): domain teacher + teaching_assignment`.

## Task 3.1.4: Ports + usecase (the knot)
- [ ] Extend `services/config/internal/ports/ports.go` `CatalogRepository` with: `CreateTeacher/GetTeacher(id)/ListTeachers`; `CreateTeachingAssignment/GetTeachingAssignment(id)/ListTeachingAssignments(ctx, cycleID, groupID, teacherID string /* "" = all */)`. (Get* for the existing entities already exist: `GetGroup`, `GetSubject` — the usecase uses them for the knot checks.)
- [ ] `services/config/internal/usecase/catalog.go`: add `CreateTeacher` (construct + repo, conflict on dup email) and **`CreateTeachingAssignment(ctx, groupID, subjectID, teacherID)`** implementing the knot:
  1. `grp, err := repo.GetGroup(groupID)` — missing → `KindInvalid` ("group not found").
  2. `subj, err := repo.GetSubject(subjectID)` — missing → `KindInvalid`.
  3. `if subj.GradeLevelID() != grp.GradeLevelID()` → `KindInvalid` ("subject does not belong to the group's grade level").
  4. `if _, err := repo.GetTeacher(teacherID); err != nil` → `KindInvalid` ("teacher not found").
  5. Build the assignment with `cycleID = grp.CycleID()` (DERIVED), `newID()`, then `repo.CreateTeachingAssignment` (duplicate → `KindConflict`).
  Plus `ListTeachers`, `ListTeachingAssignments` passthroughs.
- [ ] Usecase tests (fakes): teacher create + dup-email conflict; assignment happy path (cycle derived correctly); each knot failure (missing group/subject/teacher → Invalid; subject grade-level mismatch → Invalid; duplicate → Conflict). Neutral literals + `@example.com`.
- [ ] Commit: `feat(config): teaching-assignment knot use case (relational invariants)`.

## Task 3.1.5: Postgres adapter + RLS integration
- [ ] Add `teacherModel`/`teachingAssignmentModel` to `models.go`, mapping in `mapper.go`, repository methods in `repository.go` (reuse the existing `withTenant`/authctx). 23505→Conflict, 23503→Invalid, not-found→NotFound.
- [ ] Extend `repository_integration_test.go`: round-trip teacher + assignment; **`TestRLSIsolatesTenants`** coverage extended to (or a sibling test for) `teachers`/`teaching_assignments`; a knot test at the DB — an assignment whose subject's grade_level ≠ the group's grade_level is rejected by the USECASE (and the FK/unique backstops hold). Neutral literals.
- [ ] `cd services/config && go test ./... && go test -count=1 -tags=integration ./internal/adapters/postgres/`. Commit: `feat(config): postgres adapter + RLS IT for teachers & assignments`.

## Task 3.1.6: gRPC handler
- [ ] Extend `config_handler.go` with the 6 new RPCs (translate proto↔usecase, reuse `connectErr`); `CreateTeachingAssignment` passes `group_id/subject_id/teacher_id` (no cycle_id) and returns the derived assignment. Handler tests (fakes) for happy paths + the knot error mappings (mismatch → InvalidArgument, dup → AlreadyExists).
- [ ] `go test ./... && go build ./...`. Commit: `feat(config): gRPC handlers for teachers & teaching-assignments`.

**Checkpoint 3.1** — merge `feature/phase3-teachers-be` → `feature/phase3-teachers`.

---

# Sub-phase 3.2 — Gateway (`feature/phase3-teachers-gw`)

Reuse the Phase-2 config gateway patterns (`client/config.go`, `handler/config.go`, `configWrite`/`configRead` route helpers). NO new permissions — teachers/assignments use `config:write` (writes) / `config:read` (reads).

- [ ] **3.2.1 client** — extend `ConfigClient` with CreateTeacher/ListTeachers, CreateTeachingAssignment/ListTeachingAssignments.
- [ ] **3.2.2 handler** — REST handlers translating JSON ↔ client; `CreateTeachingAssignment` body `{groupId, subjectId, teacherId}` → returns the assignment incl. derived `cycleId`; list filters via query params (`cycleId`,`groupId`,`teacherId`). TDD with a fake.
- [ ] **3.2.3 routes** — under `/api/v1/config/teachers` and `/api/v1/config/teaching-assignments`, wrapped with the existing `configWrite`/`configRead` helpers (so `guard.Protect("config")` + `requireAuthChanged` + `RequirePermission` all apply). Update `newMux` + `main_test.go`.
- [ ] Verify: gateway `go test ./...`; E2E — owner (post password-change) creates a teacher, then assigns them to a group+subject via the gateway; a mismatched subject → 400; module OFF → 404. Commit.

**Checkpoint 3.2** — merge `feature/phase3-teachers-gw` → `feature/phase3-teachers`.

---

# Sub-phase 3.3 — Web (`feature/phase3-teachers-web`)

Extend the catalog admin surface (flag-gated `module_config`).

- [ ] **3.3.1 api** — extend `apps/web/app/config/api.ts` (+ test) with teachers + teaching-assignments (create + list), matching the gateway JSON.
- [ ] **3.3.2 screens** — a **Teachers** panel (CRUD: first/last name, email) and an **Assignments** screen: pick a cycle → group → subject → teacher and assign (dropdowns populated from the catalog; the subject list is scoped to the group's grade_level so mismatches are impossible in the UI; fewest-clicks). Show a group's current assignments (teacher per subject). RTL tests: create a teacher; make an assignment; assert the api client args.
- [ ] **3.3.3 palette/nav** — register "create teacher" / "assign teacher" Cmd+K commands under the `settings`/config surface.
- [ ] Verify: `pnpm --filter @garcia-lorca/web test && tsc --noEmit && lint`. Commit.

**Checkpoint 3.3** — merge → `feature/phase3-teachers`.

---

## Final gates (after 3.1–3.3 merged)
- Full suite (Go race + config integration incl. RLS isolation on the 2 new tables; web) green.
- buf lint + breaking vs `dev` (additive only).
- Security gate: RLS on `teachers`/`teaching_assignments`; flag boundary + `config:read/write`; the knot invariants can't be bypassed (cycle derived server-side; subject/grade-level checked); no secret literals.
- Tech-lead gate. Update `.claude/docs/MODULES.md` (M04 note: teachers/assignments done). Hand `feature/phase3-teachers` to the user for review (do NOT merge to `dev`).

## Out of scope (later)
Linking a teacher to an IAM `user` (roles/users phase — `user_id` stays nullable/unused); student **enrollment** (Phase 4); grade capture / teacher roster of students (Phase 5).

## Testing strategy
Unit (domain invariants, the knot use case, handler mapping), integration (RLS isolation on the 2 tables, the grade-level-mismatch rejection, FK/unique), contract (proto additive), web (Vitest/RTL), flag-ON/OFF. Deterministic (injected id). Neutral literals.
