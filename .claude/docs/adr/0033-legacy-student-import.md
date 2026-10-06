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
