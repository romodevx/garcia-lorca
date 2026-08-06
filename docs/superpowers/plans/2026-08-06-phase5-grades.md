# Phase 5 — Grade Capture (roster + `grade_entries`) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]` checkboxes.

**Goal:** Close the academic spine — a teacher opens one of their **teaching assignments** (group+subject), sees the **roster** (students enrolled in that group), and **captures/edits a numeric grade (0–10)** per student for a term. This is the most cross-service phase: `grades` combines **config** (which assignment = group+subject+teacher+cycle) with **students** (who's enrolled in the group) and stores `grade_entries`. Boleta generation + the SEP-level Strategy are **out of scope** (a later phase).

**Architecture:** Build out the existing `grades` service skeleton (Clean Arch; today only `doc.go` stubs + `cmd/main.go`; already in `go.work` + docker-compose). TWO cross-service seams, both consumer-defined ports implemented by Connect clients: **AssignmentReader** (→ config `GetTeachingAssignment`/`GetTerm`) and **EnrollmentReader** (→ students `GetEnrollment`/`ListEnrollments`). Cross-service refs (`enrollment_id`, `subject_id`, `term_id`, `teacher_id`) are plain UUID columns validated by RPC — **no cross-service FKs**. `subject_id`/`teacher_id`/`cycle` are **DERIVED from the assignment**, never client-supplied. Same `pkg/authctx` RLS.

**Tech Stack:** Go 1.24, GORM (adapters only), Connect/gRPC, Postgres 16 + RLS, buf, Next.js, testcontainers-go.

**Design source:** `docs/superpowers/specs/2026-07-31-academic-spine-design.md` §5.3.

---

## Delivery model

Integration branch `feature/phase5-grades` (off `dev`, created). Three sub-phases, each its own branch, merged `--no-ff`, checkpoint each:

| Sub-phase | Branch | Content |
|-----------|--------|---------|
| **5.1 Backend** | `feature/phase5-grades-be` | migration+RLS (`grade_entries`) + proto + domain + two cross-service reader clients + usecase (capture knot + roster) + adapter (RLS IT) + handler + main wiring. |
| **5.2 Gateway** | `feature/phase5-grades-gw` | REST routes behind `module_grades` + `grades:write`/`grades:read`; owner permission migration. |
| **5.3 Web** | `feature/phase5-grades-web` | Grade-capture surface: pick assignment → term → roster grid → enter/edit grades. Flag-gated; Cmd+K. |

**Per-sub-phase DoD:** gofmt/vet clean; unit + integration (RLS isolation on `grade_entries`) green; buf lint + breaking vs `dev` (additive); flag-ON/OFF; no secret literals; tech-lead (+ security for the flag) gates. `gen/`/`go.work.sum` never staged. buf via Docker; base `dev`. **Never bump the workspace to go 1.25 — keep `go 1.24`.**

---

## Relational model → DDL (sub-phase 5.1)

One tenant-scoped, `ENABLE + FORCE RLS` table in `grades` (policy `tenant_isolation` on `app.current_tenant`, copy an existing service's `0001`). All references are cross-service UUIDs — **no FKs**.

```
grade_entries
  id, tenant_id,
  enrollment_id uuid NOT NULL,   -- students.enrollments  (RPC-validated)
  subject_id    uuid NOT NULL,   -- config.subjects       (DERIVED from the assignment)
  term_id       uuid NOT NULL,   -- config.terms          (RPC-validated, in the assignment's cycle)
  teacher_id    uuid NOT NULL,   -- config.teachers        (DERIVED from the assignment)
  value         numeric(4,2) NOT NULL CHECK (value >= 0 AND value <= 10),
  published_at  timestamptz NULL,   -- reserved for the future boleta/publish flow; unused now
  created_at, updated_at
  UNIQUE(tenant_id, enrollment_id, subject_id, term_id)   -- one grade per student·subject·term
```

Indexes: `(tenant_id, enrollment_id)`, `(tenant_id, subject_id, term_id)`, `(tenant_id, teacher_id)`. No seed.

**The capture knot** (use case — derives subject/teacher from the assignment, validates the rest by RPC):
`CaptureGrade(assignmentID, enrollmentID, termID, value)`:
1. `value` in [0,10] → else `KindInvalid` (domain guard).
2. `asg := assignments.ByID(assignmentID)` (config RPC) → missing → `KindInvalid`. Yields `group_id, subject_id, teacher_id, cycle_id`.
3. `enr := enrollments.ByID(enrollmentID)` (students RPC) → missing → `KindInvalid`; require `enr.GroupID == asg.GroupID` ("student is not in this assignment's group") AND `enr.Status == active` AND `enr.CycleID == asg.CycleID` → else `KindInvalid`.
4. `term := assignments.TermByID(termID)` (config RPC) → missing → `KindInvalid`; require `term.CycleID == asg.CycleID` → else `KindInvalid`.
5. **Upsert** `grade_entries` with `subject_id = asg.SubjectID`, `teacher_id = asg.TeacherID` (DERIVED), `enrollment_id`, `term_id`, `value` — insert or update the value on the `UNIQUE(enrollment,subject,term)` key ("capturar/editar"). Returns the grade entry.

**The roster** (use case): `Roster(assignmentID, termID)`:
1. `asg := assignments.ByID(assignmentID)`; validate `term` in cycle (as above).
2. `enrs := enrollments.ByGroup(asg.GroupID)` (students RPC — active enrollments in the group).
3. For each enrollment, look up the current `grade_entries` value for `(enrollment_id, asg.SubjectID, termID)` (may be absent).
4. Return rows `{enrollment_id, student_id, value?}` (student **names are resolved in the web** from its students list — `grades` stays free of student PII).

---

## Proto contract (sub-phase 5.1)

Rewrite `proto/garcialorca/grades/v1/grades.proto` `GradesService` (the stub only has Health): keep `Health`; add `CaptureGrade`, `GetRoster`, `ListGradeEntries`. Messages: `GradeEntry {id, enrollment_id, subject_id, term_id, teacher_id, value (double), published_at}`, `RosterRow {enrollment_id, student_id, value (double), has_value (bool)}`. `CaptureGradeRequest {assignment_id, enrollment_id, term_id, value}` (NO subject_id/teacher_id/cycle_id — derived). `GetRosterRequest {assignment_id, term_id}` → `GetRosterResponse {repeated RosterRow}`. `ListGradeEntriesRequest {enrollment_id, term_id}` (optional filters). Keep the existing `GradesPublished` event message (unused this phase). buf breaking vs `dev` — a rewrite of a FASE-0 stub that only had Health should be additive for the service surface; if buf flags a change to the placeholder `Grade` message removal, that's acceptable (the stub was never consumed) — record the output.

---

# Sub-phase 5.1 — Backend (`feature/phase5-grades-be`)

Branch: `git checkout -b feature/phase5-grades-be feature/phase5-grades`.

## Task 5.1.1: Migration + RLS
- [ ] Create `migrations/grades/0001_init.sql`: `grade_entries` per the DDL, `ENABLE + FORCE RLS` + `tenant_isolation` (copy the shape from `migrations/students/0001_init.sql`). uuid PK, timestamps, the UNIQUE + CHECK + indexes. `BEGIN;…COMMIT;`. No seed. (`migrations/grades/` is a new folder; the compose `migrate` one-shot auto-applies it.)
- [ ] Verify via `migrate`: `\dt` shows `grade_entries`; RLS on; the value CHECK rejects 11 / -1. Tear down.
- [ ] Commit: `feat(grades): schema + RLS for grade_entries`.

## Task 5.1.2: Proto
- [ ] Rewrite `grades.proto` per the "Proto contract" section; `buf generate` + `lint` + `breaking --against '.git#branch=dev'` (record output) + `cd gen/go && go build ./...`.
- [ ] Commit: `feat(grades): proto — CaptureGrade / GetRoster / ListGradeEntries`.

## Task 5.1.3: Domain
- [ ] `services/grades/internal/domain/grade_entry.go` (+ test): `NewGradeEntry(id, tenantID, enrollmentID, subjectID, termID, teacherID string, value float64, capturedAt time.Time) (GradeEntry, error)` — validate ids non-empty and **`value` in [0,10]** (`KindInvalid` on out-of-range); accessors; a `WithValue(v)` helper returning a new entry (immutable) for the upsert-update path if useful. Mirror the config/students domain style. No framework imports.
- [ ] TDD red→green; commit: `feat(grades): domain grade_entry (0–10 value invariant)`.

## Task 5.1.4: Cross-service reader clients (two seams)
- [ ] Consumer-defined ports (in `ports.go`): 
  - `type AssignmentRef struct { ID, GroupID, SubjectID, TeacherID, CycleID string }`, `type TermRef struct { ID, CycleID string }`, `type AssignmentReader interface { ByID(ctx,id) (AssignmentRef, error); TermByID(ctx,id) (TermRef, error) }` (missing → `glerrors.ErrNotFound`).
  - `type EnrollmentRef struct { ID, StudentID, GroupID, CycleID, Status string }`, `type EnrollmentReader interface { ByID(ctx,id) (EnrollmentRef, error); ByGroup(ctx, groupID string) ([]EnrollmentRef, error) }`.
- [ ] Adapters: `internal/adapters/configclient/client.go` (impl `AssignmentReader` over `configv1connect.ConfigServiceClient` `GetTeachingAssignment`/`GetTerm`) and `internal/adapters/studentsclient/client.go` (impl `EnrollmentReader` over `studentsv1connect.StudentsServiceClient` `GetEnrollment`/`ListEnrollments`). Map `CodeNotFound`→`glerrors.ErrNotFound`, else `KindInternal`. Mirror `services/students/internal/adapters/configclient/client.go`. Light unit test of the NotFound mapping each.
- [ ] Commit: `feat(grades): config + students reader clients (service-to-service)`.

## Task 5.1.5: Ports (repo) + usecase (capture knot + roster)
- [ ] Port `GradeEntryRepository`: `UpsertGradeEntry(ctx, domain.GradeEntry) (domain.GradeEntry, error)` (ON CONFLICT(tenant,enrollment,subject,term) DO UPDATE value+updated_at), `GetForKey(ctx, enrollmentID, subjectID, termID) (domain.GradeEntry, error)` (NotFound if absent), `ListGradeEntries(ctx, enrollmentID, termID string) ([]domain.GradeEntry, error)`.
- [ ] `usecase/grades.go` `GradesService` holding `entries GradeEntryRepository`, `assignments AssignmentReader`, `enrollments EnrollmentReader`, `newID`, `now`. Implement `CaptureGrade` (the knot: steps 1–5; subject/teacher DERIVED from the assignment) and `Roster` (assignment → `enrollments.ByGroup` → join current values) and `ListGradeEntries`.
- [ ] Usecase tests (fakes for all three ports): capture happy (assert stored subject/teacher == the assignment's, not client input); missing assignment/enrollment/term → Invalid; enrollment-not-in-group → Invalid; non-active enrollment → Invalid; term-not-in-cycle → Invalid; out-of-range value → Invalid; **re-capture updates the value** (upsert). Roster: returns a row per active enrollment with value present/absent. Neutral literals.
- [ ] Commit: `feat(grades): capture-grade knot + roster use cases`.

## Task 5.1.6: Postgres adapter + RLS integration
- [ ] `models.go`/`mapper.go`/`repository.go` + `txrunner.go` (copy an existing service's). `UpsertGradeEntry` uses GORM's `OnConflict` (columns tenant_id,enrollment_id,subject_id,term_id → DoUpdates value,updated_at) inside `withTenant`/authctx. 23505 shouldn't surface (upsert), FK n/a (no FKs), not-found→NotFound.
- [ ] Integration test (`//go:build integration`, mirror the template): round-trip an upsert (insert then update same key → one row, new value); `TestRLSIsolatesGradeEntries` (read+write, non-owner `app` role); value CHECK rejects out-of-range at the DB. Neutral literals.
- [ ] `cd services/grades && go test ./... && go test -count=1 -tags=integration ./internal/adapters/postgres/`. Commit: `feat(grades): postgres adapter + RLS IT for grade_entries`.

## Task 5.1.7: gRPC handler + composition root
- [ ] `internal/adapters/grpc/grades_handler.go`: implement the generated `gradesv1connect.GradesServiceHandler` (CaptureGrade/GetRoster/ListGradeEntries/Health; reuse the `connectErr` mapper pattern). `CaptureGrade` passes `{assignment_id, enrollment_id, term_id, value}` (no subject/teacher). Handler tests (fakes) happy + knot error mappings.
- [ ] `cmd/main.go`: composition root — `DATABASE_URL`, `GRADES_TENANT_ID` (default `…0001`), `CONFIG_BASE_URL` (compose `http://config:8080`), `STUDENTS_BASE_URL` (`http://students:8080`); build the two reader clients + repo + usecase + handler; mount Connect under h2c + `/healthz`. Add `config` + `students` to the grades service `depends_on` + those two `*_BASE_URL` envs in `docker-compose.yml`.
- [ ] `cd services/grades && go test ./... && go build ./...`. Compose smoke: `docker compose up -d grades config students` boots; grades reaches both. Commit: `feat(grades): gRPC handlers + composition root (config+students clients)`.

**Checkpoint 5.1** — merge `feature/phase5-grades-be` → `feature/phase5-grades`.

---

# Sub-phase 5.2 — Gateway (`feature/phase5-grades-gw`)

- [ ] **5.2.1 client** — `services/gateway/internal/client/grades.go`: `GradesClient` over `gradesv1connect` (CaptureGrade / GetRoster / ListGradeEntries). Gateway base URL `GRADES_BASE_URL` (compose `http://grades:8080`).
- [ ] **5.2.2 handler** — `internal/handler/grades.go` (+ test): `CaptureGrade` body `{assignmentId, enrollmentId, termId, value}` → grade entry; `GetRoster` query `assignmentId`+`termId` → `{roster:[...]}`; `ListGradeEntries`. Consumer-defined interface + fake.
- [ ] **5.2.3 routes** — a `gradesWrite`/`gradesRead` helper pair (mirror `configWrite`/`configRead`): `guard.Protect("grades", requireAuthChanged(RequirePermission("grades:write"|"grades:read")))`. `POST /api/v1/grades` (capture → grades:write), `GET /api/v1/grades/roster` (read), `GET /api/v1/grades` (list → read). Add `gradesClient` to `newMux` + a `GRADES_BASE_URL` client in `main`; add `config`... no — the gateway calls `grades`, and `grades` calls config/students. Add `grades` to the gateway `depends_on` + `GRADES_BASE_URL` env. Update `main_test.go` with route-security tests.
- [ ] **5.2.4 permissions** — `migrations/iam/0005_grades_permissions.sql`: idempotently grant `grades:read`,`grades:write` to the owner (mirror `0004`).
- [ ] Verify: gateway `go test ./...`; E2E — with a cycle/group/subject/teacher/assignment (config) + an enrolled student (students), owner captures a grade via the gateway, GETs the roster (shows the value), re-captures (updates); module OFF → 404; out-of-range value → 400. Commit.

**Checkpoint 5.2** — merge → `feature/phase5-grades`.

---

# Sub-phase 5.3 — Web (`feature/phase5-grades-web`)

Flag-gated (`module_grades`).

- [ ] **5.3.1 api** — `apps/web/app/grades/api.ts` (+ test): `captureGrade({assignmentId, enrollmentId, termId, value})`, `getRoster({assignmentId, termId})`, `listGradeEntries`. `credentials:"include"`; envelope `{roster}`. Value sent as number; no subject/teacher in the body.
- [ ] **5.3.2 screens** — a **grade-capture** surface: pick a teaching assignment (from config `listTeachingAssignments`, showing group+subject+teacher) → pick a term (config `listTerms` for the cycle) → a **roster grid**: one row per student (name resolved from the students list) with an editable 0–10 number input; save per-row (or batch) via `captureGrade`; show the existing value when present. Client-side range validation (0–10) mirroring the server. Loading/empty/error, `role="alert"`, disabled-while-saving, aria-labels. RTL tests: capture (assert `{assignmentId, enrollmentId, termId, value}`, no subject/teacher); roster render with a pre-existing value; range guard.
- [ ] **5.3.3 flag gate + palette + nav** — wire behind `useFlag("module_grades")` (the `grades`/"Calificaciones" module in `_design/data.ts` routes to this surface; stub when OFF); Cmd+K "capture grades" command; i18n keys. Registry test ON/OFF.
- [ ] Verify: `pnpm --filter @garcia-lorca/web test && tsc --noEmit && lint`. Commit.

**Checkpoint 5.3** — merge → `feature/phase5-grades`.

---

## Final gates (after 5.1–5.3 merged)
- Full suite (Go race + grades/config/students integration incl. RLS isolation on `grade_entries`; web) green.
- buf lint + breaking vs `dev`.
- Security gate: RLS on `grade_entries`; the capture knot can't be bypassed (subject/teacher derived; enrollment/term/assignment RPC-validated; enrollment-in-group check); grades→config/students are server-to-server (both stay network-private in shared envs — `deploy/README.md` checklist); flag boundary + `grades:read/write`; value range enforced (domain + DB CHECK); no secret literals.
- Tech-lead gate. Update `.claude/docs/MODULES.md` (M07 note: capture shipped, boleta deferred). Hand `feature/phase5-grades` to the user for review (do NOT merge to `dev`).

## Out of scope (later)
Boleta generation + the SEP-level grading Strategy + `GradesPublished`/`published_at` flow (a later phase); linking the auth principal to a teacher (roles/users phase — capture derives teacher from the assignment for now, so any authorized user can capture); qualitative (non-numeric) grade schemes.

## Testing strategy
Unit (domain 0–10 invariant, the capture knot + roster with fake readers, handler mapping), integration (RLS isolation on `grade_entries`, upsert round-trip, value CHECK), contract (proto), web (Vitest/RTL), flag-ON/OFF. Deterministic (injected id/clock, fake RPC). Neutral literals.
