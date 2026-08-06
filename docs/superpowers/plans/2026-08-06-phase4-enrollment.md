# Phase 4 — Student Enrollment + Transfer History Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]` checkboxes.

**Goal:** Place a student into a **group of a cycle** (`enrollments`) and support **transferring** them between groups with a dated, reasoned history (`enrollment_transfers`). This is the join that later lets a teacher see their students and capture grades. Enrollment validates the group/term against the `config` service **by RPC** (first service-to-service call in the app) and **derives the cycle from the group**.

**Architecture:** EXTEND the existing `students` service (Clean Arch). New cross-service seam: a consumer-defined `CatalogValidator` port in `students`, implemented by a **Connect client to `config`** (`GetGroup`/`GetTerm`) — the usecase stays testable with a fake. Cross-service references (`cycle_id`, `group_id`, `term_id`) are plain UUID columns validated by RPC, **never cross-service FKs** (bounded-context discipline, even though all services share one Postgres). Same `pkg/authctx` RLS.

**Tech Stack:** Go 1.24, GORM (adapters only), Connect/gRPC, Postgres 16 + RLS, buf, Next.js, testcontainers-go.

**Design source:** `docs/superpowers/specs/2026-07-31-academic-spine-design.md` §5.2 + §5.4 (enrollment, transfers, RPC validation).

---

## Delivery model

Integration branch `feature/phase4-enrollment` (off `dev`, created). Three sub-phases, each its own branch, merged `--no-ff`, checkpoint each:

| Sub-phase | Branch | Content |
|-----------|--------|---------|
| **4.1 Backend** | `feature/phase4-enrollment-be` | migration+RLS (2 tables) + proto + domain + config-validator client + usecase (enroll knot via RPC + transfer) + adapter (RLS IT) + handler. |
| **4.2 Gateway** | `feature/phase4-enrollment-gw` | REST routes for enroll/transfer/withdraw/list behind `module_students` + auth + `students:enroll`/`students:read`; owner permission migration. |
| **4.3 Web** | `feature/phase4-enrollment-web` | Enroll a student into a group; transfer between groups (reason); view a group's roster. |

**Per-sub-phase DoD:** gofmt/vet clean; unit + integration (RLS isolation on the 2 new tables) green; buf lint + breaking vs `dev` (additive); flag-ON/OFF for the gateway routes; no secret literals; tech-lead (+ security for the flag) gates. `gen/`/`go.work.sum` never staged. buf via Docker; base `dev`. **Never let `go work sync`/`go mod tidy` bump the workspace to go 1.25 — keep `go 1.24`.**

---

## Relational model → DDL (sub-phase 4.1)

Two tenant-scoped, `ENABLE + FORCE RLS` tables in `students` (policy `tenant_isolation` on `app.current_tenant`, copy `migrations/students/0001_init.sql`'s shape). `student_id` is a real FK (same service); `cycle_id/group_id/term_id` are **UUID columns, NOT FKs** (validated by RPC to config).

```
enrollments
  id, tenant_id,
  student_id  → students(id) ON DELETE CASCADE,        -- same-service FK
  cycle_id  uuid NOT NULL,     -- config.cycles   (derived from group, RPC-validated)
  group_id  uuid NOT NULL,     -- config.groups   (RPC-validated)
  term_id   uuid NULL,         -- config.terms    (RPC-validated when present)
  status    text NOT NULL DEFAULT 'active' CHECK (status IN ('active','transferred','withdrawn')),
  enrolled_at timestamptz NOT NULL DEFAULT now(),
  created_at, updated_at
  UNIQUE(tenant_id, student_id, cycle_id)              -- one enrollment per student per cycle

enrollment_transfers   (dated, reasoned history — "traslado entre grupos con fecha y motivo")
  id, tenant_id,
  enrollment_id → enrollments(id) ON DELETE CASCADE,
  from_group_id uuid NOT NULL, to_group_id uuid NOT NULL,
  reason text NOT NULL,
  moved_at timestamptz NOT NULL DEFAULT now()
```

Indexes: `enrollments (tenant_id, student_id)`, `(tenant_id, cycle_id)`, `(tenant_id, group_id)`; `enrollment_transfers (tenant_id, enrollment_id)`. No seed.

> **Note on `students.grade_group`:** the free-text `grade_group` column (M03) is SUPERSEDED by `enrollments → group_id`. Do NOT drop it in this phase (avoid a breaking data migration); leave it deprecated-nullable and stop writing to it. Reconciliation is a later tech-debt task.

**The enroll knot** (use case, using the config RPC — the "todo bien relacionado"):
1. `student` exists locally (`StudentRepository.FindByID`) → else `KindNotFound`.
2. `grp := catalog.GroupByID(groupID)` (RPC to config) → missing → `KindInvalid` ("group not found").
3. `cycleID := grp.CycleID` — **DERIVED** from the group (never a client arg).
4. If `termID != ""`: `term := catalog.TermByID(termID)`; require `term.CycleID == cycleID` → else `KindInvalid` ("term does not belong to the group's cycle").
5. Insert `enrollments` (status `active`); duplicate `(student, cycle)` → `KindConflict` ("already enrolled this cycle").

**The transfer** (use case): `TransferStudent(enrollmentID, toGroupID, reason)`:
1. `enr := repo.GetEnrollment(id)`, must be `active` → else `KindInvalid`.
2. `toGrp := catalog.GroupByID(toGroupID)` → missing → `KindInvalid`; require `toGrp.CycleID == enr.CycleID` (no cross-cycle transfer) → else `KindInvalid`.
3. `if toGroupID == enr.GroupID` → `KindInvalid` ("already in that group").
4. In ONE tx: insert `enrollment_transfers{enrollment_id, from_group_id: enr.GroupID, to_group_id: toGroupID, reason, moved_at}` and update `enrollments.group_id = toGroupID`.

**Withdraw** (use case, rounds out the status enum): `WithdrawEnrollment(id)` → set status `withdrawn` (idempotent-ish: withdrawing a withdrawn enrollment → `KindInvalid` or no-op).

---

## Proto contract (sub-phase 4.1)

Extend `proto/garcialorca/students/v1/students.proto` `StudentsService` (additive): `Enroll`, `GetEnrollment`, `ListEnrollments(student_id | cycle_id | group_id)`, `TransferStudent`, `WithdrawEnrollment`. Messages: `Enrollment {id, student_id, cycle_id, group_id, term_id, status, enrolled_at}`, `EnrollmentTransfer {id, enrollment_id, from_group_id, to_group_id, reason, moved_at}`, an `EnrollmentStatus` enum, + the request/response messages. `EnrollRequest {student_id, group_id, term_id}` (NO cycle_id — derived). `TransferStudentRequest {enrollment_id, to_group_id, reason}`. Optionally publish `StudentEnrolled` (the event already exists) on enroll.

---

# Sub-phase 4.1 — Backend (`feature/phase4-enrollment-be`)

Branch: `git checkout -b feature/phase4-enrollment-be feature/phase4-enrollment`.

## Task 4.1.1: Migration + RLS
- [ ] Create `migrations/students/0002_enrollments.sql`: the two tables above, uuid PK, timestamps, the `student_id`/`enrollment_id` FKs (CASCADE), the UNIQUE + CHECK + indexes, both `ENABLE + FORCE RLS` + `tenant_isolation` (copy `students/0001`). `BEGIN;…COMMIT;`. No seed.
- [ ] Verify via the compose `migrate` one-shot: `\dt` shows `enrollments` + `enrollment_transfers`; RLS on. Tear down.
- [ ] Commit: `feat(students): schema + RLS for enrollments & transfers`.

## Task 4.1.2: Proto (additive)
- [ ] Extend `students.proto` per the "Proto contract" section; `buf generate` + `lint` + `breaking --against '.git#branch=dev'` (additive → clean) + `cd gen/go && go build ./...`.
- [ ] Commit: `feat(students): proto — enrollment & transfer RPCs`.

## Task 4.1.3: Domain
- [ ] `services/students/internal/domain/enrollment.go` (+ test): `EnrollmentStatus` (Active/Transferred/Withdrawn) with parse/String; `NewEnrollment(id, tenantID, studentID, cycleID, groupID, termID string, status EnrollmentStatus, enrolledAt time.Time) (Enrollment, error)` — validate id/tenantID/studentID/cycleID/groupID non-empty (termID may be ""); accessors incl. `GroupID()/CycleID()/Status()`. A `WithGroup(newGroupID)`/`Transferred`/`Withdrawn` transition helper OR keep transitions in the usecase — prefer small domain methods `CanTransfer()`/`CanWithdraw()` guarding on status.
- [ ] `enrollment_transfer.go` (+ test): `NewEnrollmentTransfer(id, tenantID, enrollmentID, fromGroupID, toGroupID, reason string, movedAt time.Time) (EnrollmentTransfer, error)` — validate non-empty (reason required), `fromGroupID != toGroupID`.
- [ ] TDD red→green; commit: `feat(students): domain enrollment + transfer`.

## Task 4.1.4: Config-validator client (the cross-service seam)
- [ ] Consumer-defined port (in `students/internal/ports/ports.go` or the usecase file): `CatalogValidator interface { GroupByID(ctx, id) (GroupRef, error); TermByID(ctx, id) (TermRef, error) }` where `GroupRef{ID, CycleID, GradeLevelID}` and `TermRef{ID, CycleID}` are small students-owned structs. Missing → `glerrors.ErrNotFound`.
- [ ] Adapter `services/students/internal/adapters/configclient/client.go`: implement `CatalogValidator` over the generated `configv1connect.ConfigServiceClient` (mirror `services/gateway/internal/client/config.go`). Map config's `NotFound` Connect code → `glerrors.ErrNotFound`; other errors wrapped `KindInternal`. Constructor `New(baseURL string, httpClient connect.HTTPClient)`.
- [ ] Unit test the adapter's error mapping with a fake Connect client OR a small httptest server (light — the usecase tests use a fake `CatalogValidator`, so this can be minimal).
- [ ] Commit: `feat(students): config catalog-validator client (service-to-service)`.

## Task 4.1.5: Ports + usecase (enroll knot + transfer + withdraw)
- [ ] Extend ports: `EnrollmentRepository` (`CreateEnrollment`, `GetEnrollment(id)`, `ListEnrollments(studentID, cycleID, groupID string)`, `TransferEnrollment(ctx, enrollmentID, toGroupID string, transfer domain.EnrollmentTransfer) error` — does the history-insert + group-update in ONE tx, `SetStatus(id, status)`). Reuse the existing `StudentRepository`.
- [ ] `usecase`: add an `EnrollmentService` (or extend the student usecase) holding `students StudentRepository`, `enrollments EnrollmentRepository`, `catalog CatalogValidator`, `newID`, `now`, optional `events EventPublisher`. Implement:
  - `Enroll(ctx, studentID, groupID, termID)` — the enroll knot (steps 1–5 above); returns the enrollment; publish `StudentEnrolled` if the publisher is wired.
  - `Transfer(ctx, enrollmentID, toGroupID, reason)` — the transfer (steps 1–4).
  - `Withdraw(ctx, enrollmentID)` — status→withdrawn (guard on active).
  - `GetEnrollment`/`ListEnrollments` passthroughs.
- [ ] Usecase tests (fakes for repo + `CatalogValidator`): enroll happy path (cycle derived from the group; assert the persisted cycleID == group's cycle); missing student → NotFound; missing group → Invalid; term-not-in-cycle → Invalid; duplicate → Conflict. Transfer: happy (history row written + group updated in the fake); non-active → Invalid; cross-cycle toGroup → Invalid; same group → Invalid. Withdraw: active→withdrawn; already-withdrawn → Invalid. Neutral literals.
- [ ] Commit: `feat(students): enrollment use cases (enroll knot via RPC, transfer, withdraw)`.

## Task 4.1.6: Postgres adapter + RLS integration
- [ ] `models.go`/`mapper.go`/`repository.go`: `enrollmentModel` (term_id nullable via `*string`), `enrollmentTransferModel`; implement `EnrollmentRepository` inside the existing `withTenant`/authctx. `TransferEnrollment` runs the history-insert + `UPDATE enrollments SET group_id=...` in ONE `withTenant` tx. 23505→Conflict, 23503→Invalid, not-found→NotFound. List filters optional.
- [ ] Extend `repository_integration_test.go`: round-trip an enrollment + a transfer (assert the enrollment's group changed AND a transfer row exists after `TransferEnrollment`, in the same tenant); `TestRLSIsolatesTenants`-style coverage for `enrollments` + `enrollment_transfers` (read+write, non-owner `app` role); UNIQUE(student,cycle) → Conflict. Seed a `students` row (same service) to hang the enrollment off. Neutral literals.
- [ ] `cd services/students && go test ./... && go test -count=1 -tags=integration ./internal/adapters/postgres/`. Commit: `feat(students): postgres adapter + RLS IT for enrollments & transfers`.

## Task 4.1.7: gRPC handler + composition root wiring
- [ ] Extend `students_handler.go` with the 5 RPCs (translate proto↔usecase, reuse the error mapper). `Enroll` passes `{student_id, group_id, term_id}` (no cycle_id). Handler tests (fakes) for happy + knot/transfer error mappings.
- [ ] `services/students/cmd/main.go`: add `CONFIG_BASE_URL` (default `http://localhost:8101`-style; compose `http://config:8080`), build the `configclient`, wire it into the enrollment usecase. Add `config` to the students service `depends_on` + `CONFIG_BASE_URL` env in `docker-compose.yml`.
- [ ] `cd services/students && go test ./... && go build ./...`. Compose smoke: `docker compose up -d students config` boots; students reaches config. Commit: `feat(students): enrollment gRPC handlers + config client wiring`.

**Checkpoint 4.1** — merge `feature/phase4-enrollment-be` → `feature/phase4-enrollment`.

---

# Sub-phase 4.2 — Gateway (`feature/phase4-enrollment-gw`)

Reuse the gateway's students client/handler + the `module_students` guard + permission pattern.

- [ ] **4.2.1 client** — extend `services/gateway/internal/client/students.go` with Enroll/ListEnrollments/Transfer/Withdraw/GetEnrollment.
- [ ] **4.2.2 handler** — REST handlers (JSON) mirroring the config handler style; `Enroll` body `{studentId, groupId, termId?}` → returns the enrollment (derived cycleId); `Transfer` body `{toGroupId, reason}`.
- [ ] **4.2.3 routes** — under `/api/v1/students/{...}` (or `/api/v1/enrollments`), behind `guard.Protect("students", requireAuthChanged(RequirePermission("students:enroll"|"students:read")(...)))`. Reads → `students:read`, writes (enroll/transfer/withdraw) → `students:enroll`. Add these two permissions to the owner role via `migrations/iam/0004_enrollment_permissions.sql` (idempotent, `SET LOCAL app.current_tenant`, mirror `0003`).
- [ ] Verify: gateway `go test ./...`; E2E — owner enrolls an existing student into a group (needs a cycle+group created via config first), then transfers them (reason), then lists enrollments; module OFF → 404; missing group → 400. Commit.

**Checkpoint 4.2** — merge `feature/phase4-enrollment-gw` → `feature/phase4-enrollment`.

---

# Sub-phase 4.3 — Web (`feature/phase4-enrollment-web`)

Flag-gated (`module_students`).

- [ ] **4.3.1 api** — `apps/web/app/students/api.ts` (or extend the existing students client) with enroll/transfer/withdraw/listEnrollments (+ test).
- [ ] **4.3.2 screens** — an **Enroll** flow: pick a student → cycle → group (dropdowns from config) → enroll (term optional); a **Transfer** action (pick new group in the same cycle + reason); a group **roster** view (students enrolled in a group). Fewest-clicks (prefill active cycle). RTL tests: enroll (assert args, no cycleId sent); transfer (reason required).
- [ ] **4.3.3 palette/nav** — Cmd+K "enroll student" / "transfer student" under the students surface.
- [ ] Verify: `pnpm --filter @garcia-lorca/web test && tsc --noEmit && lint`. Commit.

**Checkpoint 4.3** — merge → `feature/phase4-enrollment`.

---

## Final gates (after 4.1–4.3 merged)
- Full suite (Go race + students/config/iam integration incl. RLS isolation on the 2 new tables; web) green.
- buf lint + breaking vs `dev` (additive).
- Security gate: RLS on `enrollments`/`enrollment_transfers`; the enroll knot can't be bypassed (cycle derived; group/term RPC-validated); the students→config call is server-to-server (config stays network-private in shared envs — already in the `deploy/README.md` checklist); flag boundary + `students:enroll`/`students:read`; no secret literals.
- Tech-lead gate. Update `.claude/docs/MODULES.md` (M03/M04 note: enrollment shipped). Hand `feature/phase4-enrollment` to the user for review (do NOT merge to `dev`).

## Out of scope (later)
Grade capture / teacher roster with grades (Phase 5); enforcing the term-required-when-granularity=TERM rule from `students` (would need group→grade_level→level→granularity lookups — note it, keep term optional+cycle-validated for now); dropping the deprecated `students.grade_group` (reconciliation tech-debt).

## Testing strategy
Unit (domain, the enroll knot + transfer with a fake CatalogValidator, handler mapping), integration (RLS isolation on the 2 tables, transfer tx atomicity, UNIQUE conflict), contract (proto additive), web (Vitest/RTL), flag-ON/OFF. Deterministic (injected id/clock, fake RPC). Neutral literals.
