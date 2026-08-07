# Phase 8 — Attendance (daily, default-present pass list) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]` checkboxes.

**Goal:** A per-group **daily pass list** where **everyone is present by default and the teacher marks only the exceptions** (absent / late / excused), saved in one action — minimum clicks. Emits `AttendanceMarked` / `AbsenceRecorded` (the latter feeds Comms in Phase 9).

**Architecture:** `attendance` is a **thin** service (handler gRPC → service → repo — NOT full Clean Arch; matches the repo's layout mandate for CRUD+I/O services). GORM stays in `repo` only. Cross-service seams (consumer-defined interfaces + Connect clients, like `grades`): **EnrollmentReader** (→ students `ListEnrollments` by group, to build the roster + validate an enrollment belongs to the group) and **GroupReader** (→ config `GetGroup`, to validate the group). Cross-service refs (`enrollment_id`, `group_id`) are plain UUIDs validated by RPC — no cross-service FKs. Same `pkg/authctx` RLS. Emits events best-effort via `pkg/pubsub` (dev logging bus locally, like finance/students).

**Decisions (confirmed):** granularity = **per (enrollment, date)** day-level; **QR deferred to Phase 8b** (this phase is the pass list only). Default-present is modeled by **storing only the exceptions** — an enrollment with no row for a date is implicitly `present`.

**Tech Stack:** Go 1.24, GORM (repo only), Connect/gRPC, Postgres 16 + RLS, buf, Next.js, testcontainers-go, Pub/Sub emulator.

**Scope source:** MODULES.md M06 + the school note "asistencia default todos presentes, solo se desactivan los ausentes".

---

## Model & rules
- **Default present:** a student with NO `attendance` row for a date is `present`. Rows store only explicit marks. Setting a student back to present deletes their row for that date.
- Status ∈ `present | absent | late | excused` (a stored `present` row is allowed but the bulk save normalizes present→no-row so the table stays exception-only).
- One mark per student per day: `UNIQUE (tenant_id, enrollment_id, date)`.
- **Bulk save reconciles** a group's day: given the group's active enrollments (RPC) + the submitted non-present marks, in ONE tx delete every attendance row for those enrollments on that date, then insert the submitted non-present marks. Idempotent (re-saving the same pass list is a no-op net effect).
- `AbsenceRecorded` is emitted for each enrollment whose saved status is `absent`; `AttendanceMarked` for the save as a whole. Best-effort (publish failure never fails the save).
- Deterministic: the service takes an injected `now`/clock and `newID`; the domain/service reads no clock directly.

## Relational model → DDL (sub-phase 8.1)
Tenant-scoped, `ENABLE + FORCE RLS`, `tenant_isolation` on `app.current_tenant` (copy grades' `0001`).
```
attendance
  id            uuid PK DEFAULT gen_random_uuid(),
  tenant_id     uuid NOT NULL,
  enrollment_id uuid NOT NULL,   -- students.enrollments (RPC-validated, NOT a FK)
  date          date NOT NULL,
  status        text NOT NULL CHECK (status IN ('present','absent','late','excused')),
  note          text NOT NULL DEFAULT '',   -- optional reason for late/excused
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, enrollment_id, date)
```
Indexes: `attendance (tenant_id, enrollment_id)`, `attendance (tenant_id, date)`. RLS ENABLE+FORCE + `tenant_isolation` (USING + WITH CHECK). No seed.

## Proto contract (sub-phase 8.1)
Rewrite `attendance.proto` `AttendanceService` (keep `Health`): 
- `GetDailyRoster(GetDailyRosterRequest{group_id, date}) → GetDailyRosterResponse{ repeated RosterRow rows }` where `RosterRow{ enrollment_id, student_id, status }` (status defaults to `present` when no row exists). 
- `SaveDailyAttendance(SaveDailyAttendanceRequest{ group_id, date, repeated Mark marks }) → SaveDailyAttendanceResponse{ int32 absent_count, int32 marked_count }` where `Mark{ enrollment_id, status, note }` (marks carry only the non-present exceptions; the server reconciles). 
- `ListAttendance(ListAttendanceRequest{ enrollment_id, from_date, to_date }) → ListAttendanceResponse{ repeated AttendanceRecord records }` where `AttendanceRecord{ id, enrollment_id, date, status, note }`. 
- Fill out the event messages `AttendanceMarked{ record_id, enrollment_id, student_id, tenant_id, date, status }` and `AbsenceRecorded{ record_id, enrollment_id, student_id, tenant_id, date }`. `date` fields are ISO "YYYY-MM-DD" strings.

---

# Sub-phase 8.1 — Backend (`feature/phase8-attendance-be`)
Branch: `git checkout -b feature/phase8-attendance-be feature/phase8-attendance`.

## Task 8.1.1: Migration + RLS
- [ ] `migrations/attendance/0001_init.sql` per the DDL above (mirror `migrations/grades/0001_init.sql` header/RLS style). Verify via a throwaway Postgres: table exists, RLS ENABLE+FORCE, the status CHECK rejects a bad value, the UNIQUE holds. Commit.

## Task 8.1.2: Proto — rewrite the attendance surface
- [ ] Per the contract; mirror `proto/garcialorca/grades/v1/grades.proto` style. `make gen`; `cd gen/go && go build ./...`; `buf lint` (container) + `buf breaking --against '.git#branch=dev'` (the placeholder `AttendanceRecord{id,student_id}` change is the expected break). Commit ONLY the `.proto` (pathspec).

## Task 8.1.3: Service + repo + readers (thin, TDD)
- [ ] `services/attendance/internal/service/service.go`: `AttendanceService` with injected `Repo`, `EnrollmentReader`, `GroupReader`, `Publisher`, `tenantID`, `newID`, `now`. Consumer-defined interfaces declared here. Methods:
  - `GetDailyRoster(ctx, groupID, date)` → validate group (RPC), list active enrollments of the group (RPC), load stored marks for those enrollments on that date (repo, ONE batched query — no N+1), return one row per active enrollment with effective status (stored or `present`), ordered by enrollment_id.
  - `SaveDailyAttendance(ctx, groupID, date, marks)` → validate group; resolve the group's active enrollments (RPC); every mark's enrollment must be in that set + status valid + not `present` (present marks are dropped, they mean "clear"); reconcile in ONE tx (delete rows for those enrollments on that date, insert the non-present marks); emit `AbsenceRecorded` per absent + one `AttendanceMarked` (best-effort, nil-publisher safe); return {absent_count, marked_count}.
  - `ListAttendance(ctx, enrollmentID, from, to)` → repo passthrough.
  - Status validation helper (`present|absent|late|excused`) — invalid → KindInvalid.
- [ ] `services/attendance/internal/repo/postgres.go`: GORM repo behind the `Repo` interface (double-model + mapper; GORM ONLY here). `withTenant` via `pkg/authctx` (copy grades' txrunner). Methods: `MarksForDate(enrollmentIDs, date) map[enrollmentID]record` (batched), `ReconcileDay(enrollmentIDs, date, marks) error` (delete+insert in one tenant tx), `ListByEnrollment(enrollmentID, from, to)`. `var _ service.Repo = (*Repo)(nil)`.
- [ ] `services/attendance/internal/adapters/{studentsclient,configclient}`: implement `EnrollmentReader` (students `ListEnrollments` by group → active only) + `GroupReader` (config `GetGroup`) over Connect; NotFound→ErrNotFound. Mirror grades' clients. Light error-mapping unit tests.
- [ ] Service unit tests (fakes): roster defaults present + overlays stored marks; save reconciles (absent set, then re-save with fewer absentees clears the missing ones); a mark for an enrollment not in the group → KindInvalid; invalid status → KindInvalid; AbsenceRecorded emitted per absent; nil publisher safe; ListAttendance passthrough. Neutral literals, injected clock.
- [ ] Repo RLS integration tests (`//go:build integration`, testcontainers, non-owner `app` role, mirror grades): `TestRLSIsolatesAttendance` (read+write), `TestReconcileDayReplacesMarks` (idempotent; clearing works), `TestMarksForDateBatched`. Commit (sensible commits).

## Task 8.1.4: gRPC handler + composition root
- [ ] `services/attendance/internal/handler/grpc.go` implementing the generated handler (translate only; connectErr copied from grades; date formatting "2006-01-02"). `cmd/main.go`: envs `DATABASE_URL`, `ATTENDANCE_TENANT_ID`, `CONFIG_BASE_URL`, `STUDENTS_BASE_URL`, dev Pub/Sub bus; build repo + 2 readers + service + handler; mount + `/healthz`. `docker-compose.yml`: add `CONFIG_BASE_URL`/`STUDENTS_BASE_URL` + `depends_on` (postgres/migrate/config/students) to the `attendance` block (mirror grades). Compose config parses. Commit.

**Checkpoint 8.1** — merge → `feature/phase8-attendance`.

---

# Sub-phase 8.2 — Gateway (`feature/phase8-attendance-gw`)
- [ ] `client/attendance.go` + `handler/attendance.go` (+ test) mirroring grades. Routes: `GET /api/v1/attendance/roster?groupId=&date=` (read), `POST /api/v1/attendance` (save daily, write), `GET /api/v1/attendance?enrollmentId=&from=&to=` (read). Pair `attendanceWrite`/`attendanceRead` = `guard.Protect("attendance", requireAuthChanged(RequirePermission("attendance:write"|"attendance:read")))`. `migrations/iam/0008_attendance_permissions.sql` grants `attendance:read/write` to owner (mirror `0007`). Add `attendance` to gateway `ENABLED_MODULES` + `ATTENDANCE_BASE_URL` + `depends_on`. Update `newMux` signature + `main_test.go` (finance-style flag-off-404 / 401 / 403 / happy).
- [ ] Tests green; compose parses. Commit.

**Checkpoint 8.2** — merge → `feature/phase8-attendance`.

---

# Sub-phase 8.3 — Web (`feature/phase8-attendance-web`)
- [ ] `apps/web/app/attendance/api.ts` (+ test): `getDailyRoster(groupId, date)`, `saveDailyAttendance(groupId, date, marks)`, `listAttendance(...)`. Mirror grades/api.ts.
- [ ] `_screens/attendance/` pass-list screen: pick group + date → roster renders **all present by default**; the teacher toggles absentees (and late/excused) with minimum clicks; one **Guardar** submits the exceptions; success toast. Flag-gated `module_attendance`; wire into AppShell + Cmd+K (nav.attendance already exists); i18n es/en parity (new `attendance.json`). RTL tests: roster defaults present, toggle an absentee + save sends only the exception, re-render reflects saved state. `pnpm ... test && tsc && lint` green. Commit.

**Checkpoint 8.3** — merge → `feature/phase8-attendance`.

---

## Final gates (after 8.1–8.3 merged)
- Full suite: Go `-race` + attendance/config/students integration (RLS isolation + reconcile idempotency); gateway; web. buf lint + breaking vs dev.
- **Security gate:** RLS on `attendance`; flag boundary (OFF ⇒ 404) + `attendance:read/write`; enrollment-in-group validated by RPC (no forging a mark for another group); no client-injected fields; no PII beyond ids; internal service network-private (carry-forward); single-tenant `ATTENDANCE_TENANT_ID` forwarding (carry-forward); dev Pub/Sub swap (carry-forward).
- **Tech-lead gate:** thin-layout discipline (GORM only in repo; service testable; consumer-defined reader interfaces), no N+1 on the roster, reconcile atomicity, TDD/coverage ≥80% on the service, DRY vs grades.
- Update MODULES.md (M06) + FLAGS.md (`module_attendance`). ADR only if a non-obvious decision arises (e.g. exception-only storage) — likely a short ADR 0003. Hand `feature/phase8-attendance` for review (PR → dev; do NOT merge until approved).

## Out of scope (Phase 8b / later)
QR check-in (static or signed token), per-class/subject granularity, attendance summaries/reports, guardian-facing views (Phase 9 comms consumes `AbsenceRecorded`), scheduled reminders.

## Testing strategy
Unit (status validation, roster default-present overlay, reconcile with fakes + injected clock, handler mapping), integration (RLS isolation, reconcile idempotency, batched roster read), contract (proto), web (Vitest/RTL), flag ON/OFF. Deterministic. Neutral literals.
