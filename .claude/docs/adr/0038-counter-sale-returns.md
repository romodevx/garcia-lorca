# ADR 0038: Devolución de una venta de mostrador de uniformes

- Status: Accepted
- Date: 2026-10-19
- Context: QA de horario nocturno (C2-015). Una venta de mostrador no se podía deshacer: ni pantalla ni API. `sold_at_counter` nacía terminal (ADR 0007), la venta no es un depósito (es un cargo `uniforms:<entrega>` con pagos sueltos, `deposit_id NULL`) y nada anulaba un pago suelto. PLAN-m12 dejaba las devoluciones fuera de alcance.

## Decisión (del dueño, 19-oct)

1. **Se devuelve la venta completa.** Un cambio de talla es devolver y vender de nuevo.
2. **El dinero:**
   - **El mismo día de la venta** (día del colegio, ADR 0035), se anulan los pagos y se cancela el cargo. El corte del día queda como si no se hubiera vendido, pero la lista la venta como cancelada, con su motivo.
   - **Otro día** no se toca el corte de la venta, que puede estar cerrado. El reembolso es un **egreso** del día en que se devuelve, en la categoría «Devoluciones de uniformes» y por el método que se elija (efectivo, tarjeta o transferencia).
3. **Inventario:** cada prenda vuelve a la venta, salvo las que se marquen dañadas. Esas quedan como merma.
4. **Permiso:** el mismo que vender (`uniforms:write` + `finance:write`). El motivo es obligatorio.

## Cómo

- **uniforms:**
  - `sold_at_counter → returned` es la única transición que sale de una venta de mostrador, y solo la pide una persona (`ReturnAtCounter`); ningún barrido llega ahí.
  - Las líneas pasan de `sold` a `returned`. La etiqueta del pedido es `returned` cuando todas sus líneas volvieron.
  - Migración 0011: los CHECK de estados y el tipo de movimiento `return`, más la tabla `uniform_returns` (motivo, modo, método, estado del reembolso, egreso, unidades dañadas, quién, cuándo) con RLS.
  - Cada unidad devuelta deja un movimiento `return` (+n). Una dañada deja además un `adjust` (−n, «Merma: …»), y el inventario solo sube por lo que se puede vender.
- **finance:**
  - RPC `VoidSalePayments`: bajo el candado del cargo, solo para un cargo `uniforms:…` y solo si sus pagos son del día. **Mueve** los pagos a `voided_sale_payments` (migración 0030) en lugar de marcarlos. Así ningún lector de `payments` (saldo, corte, ingresos, comprobantes) tiene que aprender a ignorar una columna.
  - Cancela el cargo con «Devolución: <motivo>», `cancelled_at` y `cancelled_by`.
  - `DailyCashUp` devuelve `cancelled_sales`. `DayChangedSince` detecta la anulación, así que un corte cerrado se marca «modificado después del cierre».
- **Orden e idempotencia:**
  - El mismo día, primero se anula en finanzas. Si finanzas rechaza, las prendas siguen vendidas. La anulación es idempotente: repetirla regresa lo que ya anuló.
  - Después va la transacción de uniformes, que bloquea la entrega. Un segundo clic encuentra la venta devuelta y no regresa las prendas dos veces.
  - Otro día, el egreso se registra al final. Si falla, la devolución queda con el reembolso `pending` y la entrega marcada «requiere atención»; volver a pulsar «Devolver» lo termina.
- **gateway:** `POST /api/v1/uniforms/orders/{id}/return`. El comprobante de una venta devuelta sale como CANCELADO con «Devolución: <motivo>».

## Consecuencias

- **Nota de crédito:** el egreso de otro día devuelve el total de la venta por el método elegido. Si la venta se pagó en parte con nota de crédito, el personal decide cuánto regresa en dinero. Hoy el sistema no separa esa parte; cuando pase, habrá que capturarlo a mano.
- **Venta sin cobro en finanzas** (sin alumno, o marcada antes de crear el cargo): la devolución regresa las prendas y queda con modo `none`. No hay dinero que mover en el sistema.
- No hay devolución parcial ni cambio de talla en un solo paso. Si el colegio los pide, se agregan aparte.
