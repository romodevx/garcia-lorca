# ADR 0019: Monthly grades — transfers, scale changes, refusal codes

- Status: Accepted
- Date: 2026-09-27
- Context: Task 6 (calificaciones mensuales) code review. The owner decided the
  transfer rule and the scale-change rule; this records them and how they are
  built.

## Decisions

### A transfer splits the months between the two groups

A transfer rewrites the enrollment's `group_id` in place (same enrollment id),
so without a rule the new group's teacher could write the months the pupil
spent in the old group.

- The new group's teacher may READ every month but WRITE only months on or
  after the month of the pupil's latest transfer into the group (UTC
  `moved_at`, from `students.ListTransfers(to_group_id)`).
- Earlier months belong to the previous group. The old teacher cannot write
  them either, because the enrollment is no longer in their group. The director
  (`school_wide` on the director's routes) may edit any month.
- The check is in `MonthlyGradesService.ensureEditable`, the same hook T7 uses
  for period locks, so writes, the sheet flag and each row flag share it.
- The sheet answers `rowEditable` / `rowReadOnlyReason` per row. PUT/DELETE
  answer 400 "Ese mes corresponde al grupo anterior del alumno" with
  `X-Error-Code: MONTH_PREVIOUS_GROUP`.

### A level's scale may loosen freely and tighten only when nothing breaks

- Loosening is always allowed: a lower min, a higher max, more decimals or
  another passing mark.
- Tightening (a higher min, a lower max, fewer decimals) or disabling grading
  is refused with 409 `SCALE_TIGHTENING` "Ya hay calificaciones capturadas en
  este ciclo que no cumplirían la nueva escala" when grades captured in the
  level's ACTIVE cycle would not fit. For disabling, any grade blocks it.
- Config must not depend on grades. So the GATEWAY runs the check: it calls
  `grades.CountScaleViolations` first, then `config.UpdateLevel`. Loosening
  can never count a violation, so the gateway asks on every grading change and
  does not compare scales itself. If the check fails, the change is refused.
  The check and the write are two calls: a grade captured between them is
  judged when that grade is written, not by this change.
- `config.UpdateLevel` applies the attendance mode and the grading scale in ONE
  transaction, validated as a whole, so a half-valid PATCH changes nothing.

### Refusals carry machine codes

The grades service puts a refusal's code in the Connect error metadata
`X-Error-Code`. The gateway forwards it as a response header, but only when it
looks like a code (`^[A-Z][A-Z0-9_]{0,63}$`), and CORS exposes the header.
Codes: GRADE_OUT_OF_RANGE, GRADE_DECIMALS, GRADE_NOT_NUMBER,
MONTH_OUTSIDE_CYCLE, LEVEL_NOT_GRADED, ENROLLMENT_NOT_IN_GROUP,
MONTH_PREVIOUS_GROUP, LEVEL_SCALE_NOT_CONFIGURED and SCALE_TIGHTENING. MONTH_LOCKED
is reserved for T7.

### Smaller rules

- A grade is typed as plain decimal text: at most 16 characters, and no
  exponent, hex or binary form. Both grades and the gateway check this before
  `big.Rat` reads the text.
- The parent's view never hides grades that were already captured. If a level
  stops grading numerically, those grades are still returned, and the scale
  shown is the level's current one (a grade does not record the scale it was
  captured under). Of two active enrollments (a pupil re-enrolled for next
  year), the one in the ACTIVE cycle wins, then the one whose cycle contains
  today (UTC).
- The legacy term capture reads only the group's level id, so a scale problem
  cannot break it.
- The sheet gets pupil names in one `ListStudents(ids)` read (additive filter,
  at most 200 ids), never the whole school.

## Deploy order

**config → students → grades → gateway.**

- grades reads `Level.grading` from config. With an older config, every grading
  level answers "La escala de calificación del nivel no está configurada"
  (400, LEVEL_SCALE_NOT_CONFIGURED).
- grades calls `ListTransfers(to_group_id)`. An older students service rejects
  that call, so the sheet would fail.
- The gateway calls `config.UpdateLevel` and `grades.CountScaleViolations`.
- Migration `students/0015` (index on transfers by target group) runs with
  students.
