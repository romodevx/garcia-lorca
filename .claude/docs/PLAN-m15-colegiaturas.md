# M15 — Colegiaturas, inscripción y adeudo mensual

> M13 ya es Reports en `MODULES.md`. La salida de Docentes de Configuración es **M14**.

> Plan. Nada de esto está construido. Requiere visto bueno antes de ejecutar.
> Convenciones del repo: TDD estricto, contrato antes que handlers, flag por módulo.

## 1. Qué pidió el dueño

Cada nivel educativo (kínder, primaria, secundaria, preparatoria) tiene **su propio
costo mensual de colegiatura** y **su propio costo de inscripción**. Hay una **fecha
límite de pago cada mes**. Mes a mes se le carga a cada alumno su colegiatura, y si
no paga, **se le va formando un adeudo**. Esta configuración vive en **Configuración**.

Aparte: **Profesores** y **Asignar docentes** salen de Configuración y se vuelven
**módulos propios**, como Estudiantes.

## 2. Decisiones del dueño

| # | Decisión | Elegido |
|---|---|---|
| D1 | ¿Cuántas colegiaturas por ciclo? | **Configurable por nivel** — cada nivel define su número de mensualidades y su mes de inicio |
| D2 | ¿Qué pasa si no paga a tiempo? | **Recargo fijo por mes vencido** |
| D3 | ¿Cuándo se genera el cargo de inscripción? | **Al inscribir al alumno en un grupo** |

## 3. Decisiones técnicas (mías, con su porqué)

**T1 · El precio se guarda por (ciclo, nivel), no por nivel a secas.**
Un colegio sube la colegiatura cada año. Si el precio colgara solo del nivel,
subirlo reescribiría el pasado: los cargos del ciclo anterior dejarían de cuadrar
con su propio tabulador y no habría manera de auditar qué se cobró y por qué.

**T2 · El tabulador vive en `finance`, aunque la pantalla viva en Configuración.**
Es dinero: es la política que genera cargos. Si viviera en `config`, config
tendría que saber de dinero y finanzas tendría que pedirle precios por RPC en
cada corrida. La pantalla sí va en Configuración —es donde el dueño la busca— y
llama al gateway, que llama a finanzas. Igual que Uniformes escribe en finanzas
sin ser finanzas.

**T3 · El recargo es un cargo aparte, no una mutación del cargo vencido.**
Inflar el `amount_cents` del cargo original destruiría el rastro: nadie podría
decir después cuánto era colegiatura y cuánto castigo. Cada recargo es su propia
línea en el estado de cuenta, con su propio concepto.

**T4 · `charge_concepts` gana una columna `kind`.**
Hoy los conceptos se distinguen por nombre, y el nombre es editable. La
generación necesita saber cuál concepto ES la colegiatura. `kind` ∈
`custom | tuition | enrollment | late_fee`, con índice único parcial
`UNIQUE (tenant_id, kind) WHERE kind <> 'custom'`: el colegio puede renombrar
"Colegiatura" a "Mensualidad" sin romper nada, y no puede terminar con dos.

**T5 · La idempotencia sale de `external_ref`, que ya existe (ADR 0008).**
`uq_charges_concept_per_cycle` prohíbe cobrar el mismo concepto dos veces en un
ciclo — correcto para inscripción, imposible para 10 colegiaturas. Los cargos
generados llevan ref y caen bajo `uq_charges_external_ref`:

| Cargo | `external_ref` |
|---|---|
| Colegiatura | `tuition:<cycleId>:<studentId>:<YYYY-MM>` |
| Inscripción | `enrollment:<cycleId>:<studentId>` |
| Recargo | `latefee:<chargeId>:<YYYY-MM>` |

Correr la generación dos veces no cobra dos veces. Eso es lo que permite que un
scheduler la dispare sin miedo.

**T6 · `due_day` tope 28.**
Un límite el día 31 no existe en febrero y se movería solo. Se rechaza en el
borde, no se "ajusta" en silencio.

**T7 · La inscripción se dispara con el evento, y se respalda con la corrida.**
`StudentEnrolled` ya se publica, pero *best-effort*: hoy un fallo de publish no
tumba la inscripción del alumno — y no debe tumbarla. Para que ese hueco no se
vuelva un alumno sin adeudo, la corrida mensual también rellena inscripciones
faltantes. El evento da inmediatez; la corrida da la garantía.

**T8 · Docentes: módulo nuevo en el front, mismo servicio atrás.**
Sacarlo de Configuración es una decisión de producto sobre navegación. Mover las
tablas a un servicio propio sería una migración de datos sin beneficio hoy.

## 4. Modelo de datos

`migrations/finance/0004_tuition_plans.sql`

```sql
CREATE TABLE tuition_plans (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          uuid NOT NULL,
    cycle_id           uuid NOT NULL,   -- config.cycles  (validado por RPC, sin FK)
    level_id           uuid NOT NULL,   -- config.levels  (validado por RPC, sin FK)
    enrollment_cents   bigint   NOT NULL CHECK (enrollment_cents >= 0),
    monthly_cents      bigint   NOT NULL CHECK (monthly_cents    >= 0),
    months             smallint NOT NULL CHECK (months BETWEEN 1 AND 12),
    first_billed_month smallint NOT NULL CHECK (first_billed_month BETWEEN 1 AND 12),
    due_day            smallint NOT NULL CHECK (due_day BETWEEN 1 AND 28),  -- T6
    late_fee_cents     bigint   NOT NULL DEFAULT 0 CHECK (late_fee_cents >= 0),
    active             boolean  NOT NULL DEFAULT true,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, cycle_id, level_id)
);
-- RLS ENABLE + FORCE, policy por current_setting('app.current_tenant').
```

`migrations/finance/0005_concept_kind.sql` — la columna `kind` de T4.

`months` + `first_billed_month` en vez de una lista de meses: es lo más chico que
expresa "10 mensualidades desde septiembre" y de ahí se derivan las diez exactas
(9,10,11,12,1,2,3,4,5,6). Una lista permitiría estados que ningún colegio quiere.

## 5. Contrato (primero, y linteado)

`proto/garcialorca/finance/v1/finance.proto`:

- `UpsertTuitionPlan(cycle_id, level_id, …)` — crear o actualizar el tabulador de un nivel
- `ListTuitionPlans(cycle_id)` — el tabulador completo del ciclo
- `RunBilling(cycle_id, as_of_date)` → `{tuition_created, enrollment_backfilled, late_fees_created, skipped}`

`docs/api/openapi.yaml` gana las rutas equivalentes. El job de CI ya valida el
contrato, así que esto se lintea antes de escribir un handler.

## 6. La corrida (`RunBilling`) — el corazón

Idempotente, misma respuesta la llame un botón o Cloud Scheduler:

1. Lee los tabuladores del ciclo.
2. Lee de config los grupos del ciclo → arma el mapa `grupo → nivel`.
3. Lee de students las inscripciones **activas** del ciclo → `alumno + grupo` → nivel → tabulador.
4. Para cada mes cuyo límite ya llegó: genera la colegiatura (`ON CONFLICT DO NOTHING`).
5. Rellena las inscripciones que falten (T7).
6. Para cada cargo vencido y no pagado: un recargo por **cada mes completo** de atraso.

El paso 6 se apoya en meses completos, no en días, porque "recargo fijo por mes
vencido" es una regla mensual: correrla el día 3 o el 27 debe dar lo mismo.

## 7. Frontend

**Configuración → nueva pestaña "Colegiaturas".** Una fila por nivel del ciclo
activo: inscripción, mensualidad, número de mensualidades, mes de inicio, día
límite, recargo. Un botón "Generar cargos del mes" con el resumen de lo que hizo
(cuántos creó, cuántos ya existían) — que la corrida sea idempotente es
justamente lo que hace seguro ese botón.

**Docentes: módulo propio.** Tile nuevo en el dashboard, flag `module_teachers`,
pantalla `_screens/teachers/` que reusa `TeachersPanel` y `AssignmentsPanel` tal
cual están. Se quitan de `Catalog.tsx`. Sin cambio de backend.

## 8. Flags

| Flag | Default | Qué apaga |
|---|---|---|
| ~~`module_tuition`~~ → **`module_finance`** | OFF | **Desviación del plan, decidida al construir.** El flag propio no tenía lector: para un colegio el tabulador, la corrida y el estado de cuenta son una sola cosa, y un módulo de finanzas con colegiaturas apagadas responde sobre cargos que nadie puede explicar. Las rutas viven en `/api/v1/finance/...` detrás de `module_finance` + `finance:read`/`finance:write`. |
| `module_teachers` | OFF | El módulo Docentes. Configuración deja de mostrarlos igual. |

Recordatorio ya aprendido: los flags viven en `ENABLED_MODULES` (env), no en la
base — apagar uno es un deploy, y **no detiene un scheduler ya programado**. El
Cloud Scheduler de la Etapa E se apaga en Terraform, no con el flag.

## 9. Riesgos y lo que NO incluye

- **Becas y descuentos** (`scholarships`) no se cruzan aquí. Un alumno becado
  recibirá su colegiatura completa hasta que se conecten. Es el siguiente paso
  natural y no está en este plan.
- **Sin prorrateo**: un alumno que entra en enero recibe las colegiaturas desde
  enero, no las de septiembre a diciembre. Es lo que se quiere, pero conviene
  decirlo.
- **Sin condonación masiva.** Cancelar un cargo ya existe (`CancelChargeIfUnpaid`).
- **Sin facturación CFDI** — `billing` sigue fuera del MVP.
- El colegio debe capturar el tabulador **antes** de la primera corrida. Un nivel
  sin tabulador se reporta en `skipped`, no se cobra en cero.

## 10. Entrega

| Etapa | Qué | Días-persona |
|---|---|---|
| A | Tabulador: schema + RLS, contrato, CRUD, rutas, pestaña en Configuración | 5 |
| B | `RunBilling`: colegiaturas, recargos, respaldo de inscripciones, idempotencia | 5 |
| C | Inscripción automática: consumidor de `StudentEnrolled` en finanzas | 3 |
| D | **M14** · Docentes como módulo propio | 2 |
| E | Cloud Scheduler → `RunBilling` con OIDC, en Terraform | 1 |
| | **Total** | **16** |

D es independiente y chico: puede ir primero para dar señal temprana.
A → B → C son secuenciales. E solo tiene sentido con B terminado.
