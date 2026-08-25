# ADR 0009 — `ChargeSettled`: un evento para "el dinero ya entró"

- **Estado:** aceptado (2026-08-24)
- **Contexto del módulo:** M12 Uniforms · rebanada 12a

## Contexto

El módulo de uniformes retiene mercancía apartada y solo puede soltarla cuando el cargo está
**saldado**. La primera idea del plan fue agregar `charge_remaining_cents` y `charge_status` al
evento `PaymentRegistered` que ya existe, y que el consumidor decidiera.

Eso tiene tres problemas:

1. **Rehace la aritmética del libro mayor en el consumidor.** Sumar pagos fuera de finanzas es
   duplicar la única cuenta que importa, en un servicio que no es dueño de ella.
2. **Depende del orden de entrega.** No hay `enable_message_ordering` en `pubsub.tf`, así que dos
   pagos sobre el mismo cargo pueden llegar invertidos. Un parcial rezagado podría revertir un
   estado ya cerrado.
3. **Obliga a suscribirse a todos los pagos del colegio** — colegiaturas incluidas — para
   descartar el 95%.

Además, `charge_status` como string duplica en el cable un estado que es **derivado**, donde puede
quedar rancio, e invita a tratar el evento como fuente de verdad sobre dinero.

## Decisión

Finanzas publica un evento nuevo, `ChargeSettled`, **exactamente cuando un cargo cruza a pagado** —
no en cada cobro del camino:

```
ChargeSettled { charge_id, student_id, tenant_id, external_ref, amount_cents, settled_at }
```

- Un **pago parcial simplemente no lo emite**. El consumidor no tiene nada que decidir.
- `external_ref` viaja en el evento (ADR 0008), así que el consumidor resuelve su propia fila
  **sin llamar de vuelta a finanzas**.
- Viaja por el mismo topic `payment-events` y se distingue por el atributo `type`, igual que el
  resto. Un consumidor que solo quiere liquidaciones filtra por ese atributo.
- La condición se evalúa sobre el **estado derivado del cargo** después del pago, no sobre el monto
  cobrado — el monto por sí solo no dice nada de lo que se debía.

`PaymentRegistered` no cambia: sigue anunciando cada cobro, que es lo que comms necesita para el
recibo.

## Consecuencias

- El consumidor que libera mercancía tiene una señal binaria y sin ambigüedad de orden.
- La publicación es best-effort, como `PaymentRegistered`: el dinero ya está en el banco y perder
  una notificación es estrictamente mejor que rechazar un cobro recibido. La red de seguridad es la
  reconciliación del barrido de uniformes, que consulta finanzas para los apartados listos cuyo
  cargo ya esté saldado.
- Cualquier módulo futuro que dependa de "ya pagaron" usa este evento y no rehace la suma.

## Relacionado

- `.claude/docs/PLAN-m12-uniforms.md` §4, §11.3 y hallazgo B7e.
- ADR 0003 (event backbone) — topic por dominio, tipo en el atributo.
