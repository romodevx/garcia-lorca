# students — Students / Admisión (M03)

> Per-service context. Global team rules (`~/.claude/`) and the repo root
> `CLAUDE.md` always apply. This file is the repo-specific context for the
> students service. iam's CLAUDE.md is the template all services copy.

## Purpose
The first business vertical slice: register applicants and drive them through the
admission state machine, ending in enrollment. Enroll a student end-to-end
through the gateway. Everything student-scoped downstream (finance, attendance,
grades) references students.

## Architecture — Clean (rich domain)
`cmd/main.go` + `internal/{domain,usecase,ports,adapters/{postgres,grpc,pubsub,protoconv}}`.
Dependency direction: `adapters → usecase → domain`. The domain imports no
framework (no GORM, no Connect). GORM lives ONLY in `adapters/postgres` (double
model + `mapper.go`). Proto ↔ domain enum mapping lives ONLY in adapters
(`adapters/protoconv`, shared by the gRPC and Pub/Sub adapters so they never
drift).

## Domain invariants
- A `Student` has a stable identity and a `CURP` unique per tenant (boundary
  shape-check in the domain; the DB owns the authoritative uniqueness constraint).
- Admission is a state machine — the single source of truth is
  `allowedTransitions` in `domain/admission.go`:
  `pending → accepted → enrolled`, plus `rejected` (from pending) and
  `withdrawn` (from pending/accepted/enrolled). `rejected`/`withdrawn` are terminal.
- Transition methods (`Accept`/`Reject`/`Enroll`/`Withdraw`) are the only place
  admission rules live; an illegal move is `KindConflict`, surfaced by the gRPC
  adapter as `FailedPrecondition` (a duplicate CURP on create is `AlreadyExists`).
- Events are published only AFTER a successful persist, so a failed write
  publishes nothing.
- When an enrollment is withdrawn, the admission file becomes `withdrawn` unless
  another active enrollment has a later `enrolled_at`. Older active rows from
  previous cycles do not keep the admission file enrolled because cycle end
  does not automatically close those rows.

## Events (published)
- `StudentEnrolled` — on enroll into a cycle; carries `enrolled_at`.
- `StudentWithdrawn` — after a dated enrollment withdrawal; stored in the same
  transaction as the withdrawal and relayed from `students_outbox`.
- `AdmissionStatusChanged` — on every admission transition.

Both enrol RPCs take an optional `enrolled_on` (YYYY-MM-DD, T9): bounded by the
group's cycle `starts_on` and the school's today (Morelia), stored as 00:00
America/Mexico_City. Empty keeps the alta's instant. A past day also needs
`finance:write`, enforced by the gateway (it bills past months).

All go to the single per-domain topic **`student-events`** (mirrors finance's
`payment-events`), proto-marshaled, with `type` + `tenant_id` attributes.

The legacy import (`ImportStudents`, ADR 0033) publishes **neither**: the
`ImportService` has no publisher. Its pupils enter ENROLLED and their
enrollments carry `origin = 'import'`, which is what finance reads to never bill
their inscription. The import enrollment is dated the cycle's first day (00:00
school time; owner decision 2026-10-17, ADR 0033 amendment), so tuition runs
from September; a re-upload moves earlier imports dated later back to it
(`MoveImportedToCycleStart`, idempotent, never `withdrawn_on`). One
transaction per pupil; group capacity is not counted
(the import sizes its groups in config first). Permission (`students:import`)
and flags are enforced by the gateway.

> TRANSPORT: `cmd/main.go` currently wires a dev-only logging publisher (`devBus`,
> metadata only — no PII). The Cloud Pub/Sub adapter lands with the first
> consumer (comms, M10); the `ports.EventPublisher` port stays unchanged when it
> does.

## RLS-sensitive tables
`students` — `ENABLE + FORCE ROW LEVEL SECURITY` from the first migration. All
access is tenant-scoped through `adapters/postgres` (SET LOCAL per transaction);
isolation is covered by testcontainers tests as the app (non-owner) role. M03 is
single-tenant (tenant fixed at the composition root); RLS is tenant-scoped —
see `ADR 0001`.

## Feature flag
`module_students` — default OFF. The boundary is enforced at the gateway
(`ModuleGuard(students)` ⇒ 404 for a disabled module, evaluated before auth so a
hidden module is invisible). The gateway route is
`POST /api/v1/students` → RequireAuth → RequirePermission("students:create").

## Command palette
Registered in `apps/web/app/command/registry.ts`: a nav command plus
`create.student`, both flag-gated by `module_students`. Layer-B record search is
CP-2 (post-M03).
