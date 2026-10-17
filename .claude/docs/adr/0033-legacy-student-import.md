# ADR 0033: Migración de alumnos del sistema anterior — inscripción con origen "import"

- Status: Accepted
- Date: 2026-10-06
- Context: El colegio trae ~570 alumnos del sistema anterior en cinco Excel (uno
  por nivel). Plan aprobado: `.claude/docs/PLAN-student-import.md` (O1–O10). Esos
  alumnos ya están inscritos y ya pagaron en el sistema anterior: aquí entran
  **inscritos y sin cargo de inscripción** (O5). La colegiatura y las cuotas se les
  cobran solo **desde el mes de importación** (O8).

## Decision

- **Toda inscripción guarda su origen.** `enrollments.origin`
  (`manual`|`import`, NOT NULL DEFAULT `manual`) viaja en `Enrollment.origin`
  hasta finance (`ports.EnrollmentRef.Origin`).
- **Finance respeta el origen en cada camino que cobra inscripción**:
  `wantedInscriptions` (el run diario y la reprogramación al guardar un tabulador,
  `inscriptionMoves`) y `EnsureEnrollmentCharge` (el consumidor del evento)
  omiten las inscripciones `import`.
- **Cuotas extra desde el ingreso, solo para importados.**
  `TuitionPlan.FeeInstallmentsFrom` aplica la regla de `InstallmentsFrom`: se
  quitan los meses anteriores y el vencimiento del mes de ingreso se corre al día
  de ingreso, dentro del mismo mes para que la referencia no cambie. Un alumno
  inscrito a mano a mitad de año sigue debiendo todas las cuotas, como hoy. Se
  cambió solo lo que pidió el dueño.
- **Colegiatura**: no hubo que cambiar nada. `InstallmentsFrom(StartsOn, EnrolledAt)`
  ya cobra desde el mes de ingreso. La importación fija `enrolled_at = now()`
  explícitamente y nunca la retrocede al inicio del ciclo.
- **La importación no publica `StudentEnrolled`.** No hace falta para el dinero
  (el origen ya lo resuelve), pero evita un mensaje por alumno sin consumidor útil.

## Why not "just don't publish the event"

Fue la premisa de la v1 del plan y era falsa. `RunBilling` emite la inscripción
de *cualquiera inscrito sin una* (`billing.go`, `wantedInscriptions`) con solo
leer la tabla `enrollments`. Sin la marca, la primera corrida diaria habría
cobrado ~570 inscripciones que el colegio ya cobró.

## Consequences

- Hay una regresión que lo vigila: `usecase/imported_enrollment_test.go` (0
  inscripciones, colegiatura y cuotas desde el mes de importación, 0 recargos, el
  manual sin cambios).
- Si un día se quiere reimportar con cargo, se agrega un tercer origen en lugar
  de quitar la regla.

## Notes from the gates (tech-lead + security, 2026-10-06)

- **Import by day, not after 18:00 local on the last day of a month.**
  `enrolled_at` is a UTC instant and finance's day is the UTC day
  ([[school-day-is-utc-day]]): an import late on 31 October would read as
  November and skip October's tuition and fees. The runbook says so; the general
  fix is the school-timezone work, not a patch here.
- **The BBVA reference belongs to the students module**, not to the import: it
  is read and edited with the pupil's file (`students:read` / `students:update`)
  whether or not `module_student_import` is on. Once finance reconciles deposits
  by it, editing it should require a finance-level permission (follow-up).
- **Audit.** Every pupil an import creates carries its `import_batch_id`; the
  gateway logs batch, acting user and counts (never names or CURPs). A stored
  batch record (who/when/file) is a follow-up if the school asks for an undo.
- **One commit at a time per school** (409 `IMPORT_IN_PROGRESS`), its own write
  deadline (2 min), and a plan hash that includes the file's bytes, so a commit
  writes exactly the file the office previewed.

## Load test (2026-10-06/07)

Synthetic 1,000-pupil file against a throwaway database on the dev Cloud SQL
instance (`db-f1-micro`), services run locally through cloud-sql-proxy — so
every query paid a ~85 ms Mexico→us-central1 round trip. Measured:

- Preview of 1,000 rows on an empty school: 2.5–3.6 s. With its 24 groups
  already created: ~37 s, ≈ 410 round trips (students checks each group with
  config: 3 RPCs per group). Validation lookups are batched (`IN (…)`).
- Commit: ~0.58 s per pupil ≈ 7 round trips (tx, CURP, reference, matrícula,
  pupil, enrolment, commit). The single-call commit wrote 128 of 1,000 and then
  answered a 500 that lost even those.
- In region (Cloud Run ↔ Cloud SQL, ~1–2 ms per round trip) the same counts give
  ~1 s for the preview and ~15–25 s for 1,000 pupils — inside the 2-minute
  route budget. Not measured in region; the first real import in dev is that
  measurement.

Fixes from it: the commit writes in chunks of 50 and stops between chunks when
time runs out (unanswered rows come back `not_answered`; re-uploading the same
file finishes them); the preview has the same route deadline as the commit; and
`pkg/gormlog` stops slow-query logs from writing CURPs (they did, in every
service). Optional later: batch students' group checks (one config call per
import instead of three per group).

## Enmienda 2026-10-17 (T14): inscritos desde el inicio del ciclo

Decisión del dueño: los alumnos migrados estuvieron en el colegio desde el primer
día y **se cobran desde septiembre como todos**. Reemplaza O8 y la línea
"`enrolled_at = now()`… nunca la retrocede" de arriba.

- La importación fecha la inscripción `import` en el **primer día del ciclo**, a
  las 00:00 de Morelia (ADR 0035). Si el ciclo todavía no abre, se fecha en el
  momento de la importación (nunca en el futuro); finanzas y asistencia leen eso
  igual que el ciclo completo.
- Sigue sin cobrarse la inscripción (O5): eso lo decide `origin = 'import'`, no
  la fecha.
- **Volver a subir el archivo repara** las importaciones anteriores: a cada
  alumno que ya estaba, su inscripción `import` del ciclo de la fila con
  `enrolled_at` posterior al primer día se mueve a ese día, en una sola sentencia
  por ciclo (idempotente; nunca toca inscripciones `manual` ni `withdrawn_on`). La
  vista previa dice cuántas se moverían y el resultado cuántas se movieron.
- Efecto en dinero: la siguiente corrida emite septiembre (y la parte de
  septiembre de cada cuota) ya vencido, y el recargo fijo se acumula por cada mes
  vencido completo. Queda a decisión del dueño; las reglas de recargo no cambiaron.
