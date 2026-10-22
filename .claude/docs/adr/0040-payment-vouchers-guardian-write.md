# ADR 0040: Comprobantes de pago: segunda escritura de un tutor

- Status: Accepted
- Date: 2026-10-21
- Context: las familias pagan por transferencia (la CLABE ya está en la app, #164) y no tienen cómo avisarle al colegio. La encargada se entera tarde o nunca y el mes sigue sumando recargos sobre dinero ya enviado. Diseño: `.claude/docs/design/DESIGN-comprobantes-pago.md`. Extiende ADR 0010 y se apoya en ADR 0002 (nudo de pagos), 0015 (Cobranza), 0026 (correcciones), 0027 (comprobantes de egreso) y 0035 (día de Morelia).

## Decisión (del dueño, 21-oct)

1. El tutor sube desde la app un comprobante (foto o PDF) para un hijo, con importe y fecha **opcionales**.
2. La encargada ve los comprobantes **pendientes primero** en Registro de pagos, confirma el importe y registra el pago con el `RegisterDeposit` **existente**. No hay un segundo camino de cobro.
3. Si registra a mano buscando al alumno, el comprobante pendiente de ese alumno se **vincula solo**, en la misma transacción.
4. Rechazar lleva motivo. El tutor ve `pendiente / aplicado / rechazado`.
5. **Invariante duro: nunca doble pago.**

## Por qué es un ADR

Es la **segunda escritura de un tutor** en la plataforma (la primera es `uniforms_own:write`, ADR 0010). Se acepta con las mismas cuatro condiciones:

1. **El permiso no dice de quién.** `vouchers_own:write` (iam 0033, al rol `guardian` y al owner para que pueda delegarlo). La pertenencia la impone el vínculo tutor→alumno en cada llamada; un hijo ajeno es **404, nunca 403**.
2. **El alcance se impone en el servicio.** `ListPaymentVouchers` y `GetPaymentVoucherFile` reciben un `VoucherScope` obligatorio: los hijos del tutor o la cola escolar **explícita** (`school_queue`). Una lista de hijos vacía es `InvalidArgument`, nunca "todos": un tutor sin alumnos ligados vería las transferencias de todo el colegio. Un comprobante de otro alumno es `NotFound`, también al cobrar.
3. **El importe del tutor nunca es dinero.** Sólo prefija; la encargada confirma y el cobro pasa por el planificador de siempre (CC-202, recargos, becas).
4. **Techos.** 5 MB, tipo por bytes (JPEG/PNG/PDF), 3 pendientes por alumno (serializado con un candado por alumno), mismo archivo deduplicado por SHA-256 mientras no esté rechazado, limitador por principal en el borde (no el limitador global del gateway).

## Cómo (finanzas)

- **Datos** (`migrations/finance/0032_payment_vouchers.sql`): `payment_vouchers` + `payment_voucher_events` (historial append-only), ambas `ENABLE + FORCE RLS` con `tenant_isolation`. CHECK de forma por estado. `uq_payment_vouchers_deposit`: un depósito, a lo más un comprobante; la columna `deposit_id` única por fila: un comprobante, a lo más un depósito.
- **Archivo**: mismo bucket privado que los egresos (`EXPENSE_RECEIPTS_BUCKET`, prefijo `vouchers/<id>/<aleatorio>`). Sube antes de la fila y se borra si la fila falla. `ReceiptFile` se extrajo de `expense.go` para que lo usen los dos.
- **Cobro por comprobante** (`RegisterDeposit.voucher_id`): la llave de idempotencia se **deriva** del comprobante y el cliente no puede mandar otra. Dentro de la transacción: cargos del alumno `FOR UPDATE` → fila del comprobante `FOR UPDATE` (mismo orden en Void y Correct, sin deadlocks) → ajeno/inexistente `NotFound`, aplicado `VOUCHER_ALREADY_APPLIED` (con folio), rechazado `VOUCHER_REJECTED` → depósito → `applied`. Un segundo intento con la misma llave es replay.
- **Cobro a mano** (sin `voucher_id`): los pendientes del alumno menos `declined_voucher_ids`. 0 nada; 1 se vincula (`auto_linked`) y vuelve en `linked_voucher`; 2 o más `VOUCHER_CHOICE_REQUIRED` y no se cobra nada. Nunca otro alumno, nunca por importe ni nombre. Finanzas no lee flags: el vínculo automático ocurre aunque el módulo esté apagado, y eso es lo que evita el doble pago cuando se vuelva a encender.
- **Anular / corregir**: `VoidDeposit` regresa el comprobante a `pending` (`released_by_void`); `CorrectDeposit` lo mueve al depósito corregido (`moved_by_correction`). `deposit_id` siempre apunta a dinero vivo.
- Los códigos viajan como `X-Error-Code` (`VOUCHER_ALREADY_APPLIED`, `VOUCHER_REJECTED`, `VOUCHER_CHOICE_REQUIRED`, `VOUCHER_LIMIT`).

## Desviación del diseño: la llave cuenta las liberaciones

El diseño fijaba la llave en `voucher:<id>`. Eso rompe la anulación: el depósito anulado **conserva** su llave (es la fila que explica el hueco del día), así que el comprobante liberado nunca podría volver a aplicarse — cada intento sería el replay del dinero anulado. Por eso `payment_vouchers.release_count` cuenta las anulaciones y la llave es `voucher:<id>` la primera vez y `voucher:<id>:<n>` después. El usecase lee el comprobante para derivarla; dentro de la transacción, si la llave ya no es la vigente (alguien anuló en medio) la respuesta es un 409 "ábrelo de nuevo", y el replay de un depósito anulado por comprobante también se rechaza en vez de contestarse.

## Consecuencias

- Doble pago cubierto por llave derivada + `FOR UPDATE` + dos índices únicos + 409 en ambigüedad. Residual: la encargada declina un vínculo correcto y luego aplica el comprobante; lo cubre la guardia humana del diseño (§4 D, web), no el servidor.
- Un vínculo automático equivocado (efectivo de otra cosa con una transferencia pendiente) deja el comprobante aplicado al pago incorrecto; se ve en `linked_voucher` del recibo y se arregla con corrección o anulación.
- `PreviewDeposit` no recibe `voucher_id`: prefijar el importe es del cliente, con los datos de la cola que ya leyó.
- Pendiente fuera de finanzas: gateway (rutas, `ownsChild`, flag `module_payment_vouchers`, limitador por principal), web, móvil (picker nativo ⇒ build nuevo), lifecycle de 90 días del prefijo `vouchers/` para rechazados, aviso de privacidad LFPDPPP (bloquea producción, no dev).
