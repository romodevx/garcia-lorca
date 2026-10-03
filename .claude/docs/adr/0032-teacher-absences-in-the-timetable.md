# ADR 0032: Ausencias de profesores en el horario (T19)

- Status: Accepted
- Date: 2026-10-02
- Context: T19 of the review sprint (plan §9), on top of the timetable of
  ADR 0023 (T16) and the school's local day (`SCHOOL_TIMEZONE`, ADR 0028).
  The school wants to register that a teacher will be away — sick leave,
  vacation, a personal matter, a medical appointment from 08:00 to 10:00 —
  and see which classes are left without anybody in front of the group until
  a substitute is assigned or the class is cancelled for that date. It is NOT
  teacher attendance: nobody takes a roll of teachers.

## Decision

- **config owns absences, next to the timetable.** Migration `config/0018`:
  `teacher_absences (tenant, cycle, teacher, kind, starts_on, ends_on,
  start_minute/end_minute NULL = full day, note, created_by/name, timestamps,
  cancelled_at/by/name)` with RLS ENABLE + FORCE + tenant_isolation. Kinds:
  `sick` (Incapacidad), `vacation` (Vacaciones), `personal` (Asunto personal),
  `other` (Otro). Dates are school-local calendar dates (midnight UTC like
  every date in config), both inclusive and inside ONE cycle; an empty cycle
  on create means "the cycle whose dates contain `starts_on`" (so Docentes,
  which has no cycle on screen, can register one). A time range applies to
  every date of the span, half-open like classes.
- **Never deleted.** Editing changes kind/dates/hours/note (never the
  teacher); cancelling stamps `cancelled_*`. Every create/edit/cancel goes to
  `schedule_events` (new `absence_id` and actions `absence_*`).
- **One rule, one place.** `domain.TeacherAbsence.Covers(date, slot)` is "is
  the teacher away on D during R" (active, D in the span, full day or the
  ranges overlap). It drives:
  - the effective timetable of a date: `MarkUncovered` turns a non-cancelled
    class whose teacher — the substitute when there is one — is away into
    status `uncovered`, carrying the absence;
  - substitute suggestions: an absent teacher is never listed;
  - date-specific changes: a substitute who is away, or a swap that moves a
    class into its teacher's absence, is refused (`SCHEDULE_TEACHER_ABSENT`).
    A swap that moves a class OUT of an absence is allowed;
  - "Clases por cubrir": `ListUncoveredClasses(cycle, from, to)` (≤ 31 days),
    and the count returned when an absence is saved ("Quedan N clases sin
    cubrir en esas fechas").
  Two active absences of one teacher never share a minute
  (`SCHEDULE_ABSENCE_OVERLAP`). Absence writes take the cycle's timetable
  lock (ADR 0023), so they cannot race a change that puts that teacher in class.
- **Non-working days (closes the T16 gap).** A date the school calendar
  (`non_working_periods`) closes has no classes: the day reads empty with its
  label, it is skipped when counting/listing uncovered classes, suggestions
  are empty, and a date-specific change on it is refused
  (`SCHEDULE_DATE_NON_WORKING`). This amends ADR 0023's consequence "a change
  can be recorded on a holiday".
- **Privacy is enforced at the edge.** The kind, the note and who registered
  or cancelled an absence reach ONLY `schedule:write` holders; the gateway
  shapes every absence it returns (`/schedule/absences`, `/schedule/uncovered`,
  `/schedule/day`) by the caller's permissions, and the teacher's own
  `/teaching/schedule/day` never carries them. The history summary never
  names the kind or the note (the history is `schedule:read`).
- **Edge.** `GET/POST /api/v1/schedule/absences`, `PUT …/{id}`,
  `POST …/{id}/cancel`, `GET /api/v1/schedule/uncovered`, behind
  `module_config` + `module_schedule`; reads `schedule:read`, writes
  `schedule:write` — no new permission. A caller cannot register their own
  absence (403 `SCHEDULE_ABSENCE_OWN`, checked against the teacher record
  linked to the caller).
- **Web.** "Registrar ausencia" in Horarios and on each teacher's row in
  Docentes (a ≤ 8-field drawer, hours only when "Todo el día" is off); the
  week view gains a week stepper and paints the absent teacher's classes
  "Sin cubrir" in red; "¿Qué está pasando ahora?" shows them with
  "Asignar suplente"; the new tab "Por cubrir" (today / a week, by level and
  absent teacher) covers each class with the T16 change drawer, date and
  reason prefilled, and lists the period's absences to edit or cancel.

## Consequences

- No notification to families and no attachment (owner's answer, plan §9).
- The teacher's own WEEK view does not paint absences (their "¿Qué está
  pasando ahora?" does, as "Ausente"); a teacher's week is read from the base.
- Cancelling an absence is not undoable from the screen (it is confirmed);
  registering it again is the way back.
- An absence registered AFTER a substitute was assigned to that teacher
  leaves that cover uncovered too — on purpose: nobody is in the classroom.
- Undoing a substitute of an absent teacher's class makes it uncovered again;
  it is not refused.
