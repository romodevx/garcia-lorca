# ADR 0018: Attendance is taken per level mode — daily or per class

- Status: Accepted
- Date: 2026-09-26
- Context: Task 3 of the September adjustments (asistencia por lista). The rules
  were approved by the owner.

## Problem

Phase 8 stored one mark per student per day. In Preescolar and Primaria one
homeroom teacher takes the list. In Secundaria and Preparatoria each subject's
teacher takes their own, so a student can be present in Matemáticas and absent
in Historia on the same day. The model also had no way to tell a day where
everybody was present from a day nobody took the list, and every re-save got
new record ids, so each correction re-notified the parents.

## Decision

- **The mode lives on the level.** `levels.attendance_mode` is `daily` or
  `per_class`. New levels get a default from their name (secundaria,
  preparatoria and bachillerato → `per_class`, everything else → `daily`), and
  `migrations/config/0008` gives the same default to levels that already exist.
  `PATCH /api/v1/config/levels/{id}` changes it.
- **A list is keyed by (group, date, subject).** A NULL subject is the daily
  list. `attendance.subject_id` plus a `UNIQUE NULLS NOT DISTINCT` key keeps one
  mark per student per list. Rows written before this change keep NULL (daily).
- **The attendance service stays mode-agnostic.** It stores whichever key it is
  given. The mode rules are enforced at the gateway, which already resolves
  ownership: per_class requires a subject, daily refuses one, and a per-class
  list belongs only to that subject's teacher.
- **Sessions.** `attendance_sessions` records who took each list and when
  (first save, plus `updated_at` for later corrections), under the same RLS as
  `attendance`.
- **Corrections are diffs.** A save loads the list, diffs it (`DiffList`) and
  applies the diff in one transaction. Changed rows are updated in place, so
  their ids are stable. Rows that became present are deleted. Events go out
  only for what changed: a new exception, a changed status, or `present` for a
  cleared exception. A note-only edit is silent.
- **Teacher window.** A teacher may save today and up to 7 days back, never the
  future. The director has no window. "Today" is the UTC date, the same known
  offset finance has (see memory `school-day-is-utc-day`). It is not patched
  here alone.

## Consequences

- Comms keys its idempotency ledger on `<event type>:<record_id>`. With stable
  ids, a status change on an existing row re-publishes `AttendanceMarked` under
  an id that was already processed, so comms drops it as a duplicate. (An
  `AbsenceRecorded` for a row that turns absent for the first time still goes
  through.) **T5** must key corrections
  differently, for example by (record_id, status) or by a new event id, before
  parents can be told about corrections. Until then parents hear about new
  exceptions and never twice about the same one.
- `ListAttendance` (and so the family attendance view) can now return several
  records for one day in a per-class school. `subject_id` tells them apart. T5
  renders them.
- A mark belongs to the list (group) that stored it (migration
  `attendance/0004`, `attendance.group_id`). A list never shows, clears,
  updates nor re-marks a pupil·day·subject another group's list holds (a
  transferred pupil, on a day before the move). A save skips such marks and
  says so: `SaveDailyAttendanceResponse.held_elsewhere` (gateway:
  `heldElsewhere`) lists the skipped enrollment ids, and `absentCount` /
  `markedCount` count only what this list stored, never what was submitted.
- **Known limitation — legacy marks.** A mark stored before `attendance/0004`
  has `group_id NULL` and is claimed by ANY list of its day: every such list
  reads it as its own and may correct or clear it (the correction stamps the
  correcting list's group). No backfill was written because those rows exist
  only in local dev databases today; a shared environment migrated before its
  first attendance save has none. If legacy rows ever reach a shared database,
  backfill `group_id` from the enrollment's group at mark time before relying
  on the ownership rule.
