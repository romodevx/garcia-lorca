# ADR 0037: Cancelar un cargo y descontarlo exigen motivo

- Status: Accepted
- Date: 2026-10-17
- Context: QA de ciclo completo (CC-203, PR #143). Finanzas ganó pantalla para cancelar un cargo y darle un descuento. `POST /api/v1/finance/charges/{id}/cancel` no llevaba cuerpo, así que un cargo desaparecía sin razón registrada; la columna `charges.cancellation_reason` (finance/0027) sólo la llenaba la baja. El descuento aceptaba un motivo vacío.

## Decision

- **Cambio de contrato:** `POST /charges/{id}/cancel` ahora **exige** el cuerpo `{"reason": "…"}`. Sin él, o con espacios, el gateway responde 400 y no llama a finanzas. `CancelChargeRequest.reason` (proto, campo 2) y `FinanceService.CancelCharge(ctx, id, reason)` también lo exigen (trim, máximo 500 caracteres, `KindInvalid`). `SaveCharge` lo guarda en `cancellation_reason`.
- `POST /charges/{id}/discount` con `reason` vacío o en blanco también es 400 en el gateway.
- La anulación de un depósito ya exigía motivo (sin cambio).

## Consequences

- Es un cambio incompatible para quien llamara a cancel sin cuerpo. El único cliente era `apps/web` (`cancelCharge`, que no se usaba hasta este PR), y la web y el gateway se despliegan juntos, así que no hace falta subir de versión.
- El RPC `CancelChargeIfUnpaid`, el que usan otros módulos, sigue sin pedir motivo: lleva el suyo (p. ej. "baja").
