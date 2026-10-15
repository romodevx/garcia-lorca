# ADR 0034: La baja cancela cargos futuros sin pagos

- Status: Accepted
- Date: 2026-10-15
- Context: El plan `.claude/docs/PLAN-remediacion-qa-2026-10.md` (T1) define que una baja escolar detiene la facturación futura del alumno en ese ciclo, conserva el adeudo ya generado y termina sus inscripciones a actividades desde la fecha efectiva.

## Decision

- **La fecha efectiva es inclusiva para la baja.** Solo se cancelan cargos del alumno y ciclo cuyo `due_date` sea estrictamente posterior a `withdrawn_on`. Los cargos que vencen ese día o antes permanecen.
- **No se cancela una deuda que ya recibió dinero.** Todo cargo con pago total o parcial permanece; su saldo efectivo pendiente sigue contando en el adeudo. Un cargo ya cancelado no vuelve a tocarse. `PlanWithdrawal` es la regla pura; antes de invocarla, el caso de uso aplica el mismo `scholarshipPricer` y fecha que el estado de cuenta.
- **El write vuelve a decidir bajo bloqueo.** El caso de uso calcula la vista y luego usa `CancelChargeIfUnpaidWithReason` por cada cargo elegible. El repositorio toma el bloqueo de fila y revisa pagos dentro de la transacción, como en el camino de cobro. Si un pago gana la carrera, el cargo queda; un conflicto de esa carrera no cancela el procesamiento de la baja.
- **La cancelación deja motivo.** `charges.cancellation_reason` guarda `baja`, separado de `discount_reason`; la columna se agrega con la migración versionada `0027_charge_cancellation_reason.sql`. No cambia las políticas RLS.
- **La baja también termina actividades.** Para cada actividad con inscripción del alumno, finance llama `EndEnrollments` con `withdrawn_on`. La actividad no factura un período que inicie en o después de esa fecha.
- **Preview es de solo lectura.** `PreviewWithdrawal` comparte la carga y el precio del estado de cuenta, y responde cargos a cancelar, su importe y el adeudo restante. El endpoint no cancela cargos ni termina actividades.
- **La integración es por evento duradero.** Students publica `StudentWithdrawn` en `student-events` con `tenant_id` como atributo, junto con `student_id`, `cycle_id`, `enrollment_id` y `withdrawn_on` (`YYYY-MM-DD`) en el proto. Finance aplica el cálculo compartido mediante el consumidor y el RPC preview; el consumidor devuelve errores transitorios para retry y ACKea mensajes malformados.
- **Redelivery es idempotente.** Cargos cancelados ya no forman parte de la siguiente proyección, y `EndEnrollments` no cambia una inscripción que ya terminó en esa fecha. No se crea un ledger de idempotencia adicional.

## Consequences

- El saldo posterior a la baja conserva cargos pasados y abonos parciales con el mismo precio efectivo que el estado de cuenta; la excepción de pago parcial evita borrar dinero recibido.
- El repositorio y el consumidor tienen pruebas de integración con Postgres real (testcontainers), corridas en local y en CI.
- Si cambia la regla temporal (por ejemplo, cancelar también un cargo que vence el día de la baja), se modifica la regla pura y sus pruebas de frontera antes de adaptar los adaptadores.
