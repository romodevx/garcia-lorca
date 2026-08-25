# ADR 0008 — Cargos con referencia externa (`external_ref`)

- **Estado:** aceptado (2026-08-24)
- **Contexto del módulo:** M12 Uniforms · rebanada 12a
- **Decide:** dueño del producto (gate de premisas de `/autoplan`, decisión D5)

## Contexto

`migrations/finance/0001_init.sql` declara en `charges`:

```sql
UNIQUE (tenant_id, student_id, cycle_id, concept_id)
```

La restricción existe por una buena razón: `CreateChargesBulk` genera la colegiatura de todo el
padrón en una sola sentencia con `ON CONFLICT DO NOTHING`, de modo que **re-correr una generación
no duplica nada**. La regla se lee como *"un alumno debe un concepto una sola vez por ciclo"*.

Esa regla es correcta para la colegiatura y **falsa para cualquier cosa que una familia compre
más de una vez en el ciclo escolar**. Un pedido de uniforme partido en dos entregas son dos
cargos; un suéter comprado otra vez en enero es un tercero. Con la restricción total, todos
fallaban con `KindConflict`.

Quitar la restricción no es opción. `CreateChargesBulk` usaba
`clause.OnConflict{DoNothing: true}` **sin target**, así que el arbitrador se infería del índice
que hubiera. Sin la restricción, esa cláusula **deja de deduplicar en silencio** y la siguiente
corrida de `GenerateCharges` vuelve a facturar al colegio entero, sin error y sin aviso.

## Decisión

1. `charges` gana la columna `external_ref text NOT NULL DEFAULT ''`.
2. La restricción total se sustituye por **dos índices únicos parciales**:
   - `WHERE external_ref = ''` sobre `(tenant_id, student_id, cycle_id, concept_id)` — los cargos
     propios del colegio conservan la regla de "uno por ciclo", **sin cambio alguno**.
   - `WHERE external_ref <> ''` sobre `(tenant_id, external_ref)` — los cargos causados por otro
     módulo se identifican por su referencia, así que pueden repetir el concepto libremente, y una
     creación reintentada sigue deduplicándose.
3. `CreateChargesBulk` **nombra su arbitrador explícitamente**, con la misma condición parcial.
4. `idempotency_key` y `external_ref` son **el mismo campo**. No se inventan dos mecanismos para
   la misma idea.

Es la forma que `migrations/finance/0002_payment_idempotency.sql` ya usó para la idempotencia de
pagos: columna con default vacío, índice parcial que excluye el vacío. No es territorio nuevo.

## Consecuencias

- El módulo de uniformes puede cobrar por entrega y por compra repetida sin tocar más el libro mayor.
- La colegiatura conserva su idempotencia, ahora protegida por **un test de regresión explícito**
  que re-corre `GenerateCharges` y exige `created == 0`.
- Cualquier módulo futuro que necesite cobrar repetido (trámites, papelería, excursiones) usa el
  mismo mecanismo, sin otra migración del libro mayor.
- El cargo referenciado no es especial en ningún otro sentido: mismo estado de cuenta, misma caja,
  mismo corte, mismos reportes.

## Alternativas descartadas

- **Un concepto de cargo por pedido** (`"Uniformes · U-2026-000123"`). Cero riesgo de migración,
  pero llena el catálogo de conceptos con cientos de filas al año y rompe el significado de
  "concepto"; el dropdown de caja se vuelve inusable en un semestre.
- **Uniformes con su propio registro de cobro.** Cero cambios al libro mayor, pero caja opera dos
  sistemas y el uniforme no aparece en el estado de cuenta del alumno. Rompe la premisa P2 del
  plan del módulo.

## Relacionado

- `.claude/docs/PLAN-m12-uniforms.md` §4, §12 (hallazgo B1) y decisión D5.
- ADR 0002 (payment knot) — de ahí viene la postura de decidir bajo el row lock del cargo, que es
  la que usa `CancelChargeIfUnpaid`.
