# PLAN M18 — Inscripción por conceptos y tramos de precio

Rama: `feature/inscripcion-por-tramos` (de `dev`) · Origen: petición del dueño con el
folleto **Costos ciclo 2026/2027** en mano.
Alcance UI: **sí** (configuración + estado de cuenta web + app del padre). Alcance DX: no.

Módulo: **M05 Finance / M15 Colegiaturas** (extensión). **No nace un flag nuevo**: viaja
dentro de `module_finance`, que ya está registrado.

---

## 0. La petición, tal como llegó

El folleto del colegio dice, para Preescolar:

| Concepto | Ene/Feb | Marzo | Abril a Sept |
|---|---:|---:|---:|
| Inscripción | $2,200 | $3,300 | $4,400 |
| Gastos adm y seguro | $4,700 | $4,700 | $4,700 |
| Cuota Asociación Padres de Familia | $850 | $850 | $850 |
| **Total a pagar** | **$7,750** | **$8,850** | **$9,950** |

De ahí salen cinco exigencias:

1. La inscripción no es un monto: es **varios conceptos**, cada uno con su nombre.
2. Cada concepto tiene **montos distintos según la fecha** en que se cubre.
3. **Alumno nuevo a mitad del ciclo en curso:** al registrarlo se le cargan esos
   conceptos **más la colegiatura**, como primer pago.
4. **Alumno ya inscrito, ciclo siguiente:** al crear 2027-2028 y llegar enero se le
   carga inscripción + colegiatura de enero. **Si no paga, el mes siguiente debe la
   colegiatura corriente más la inscripción al precio del mes que corresponda.**
5. El padre debe ver **en la app, al día de consulta, el saldo real** — ya con recargos y
   pronto pago, que es como funciona hoy en ventanilla.

---

## 1. Decisiones del dueño (tomadas en el checkpoint, 9-sep-2026)

| # | Decisión | Elegida |
|---|---|---|
| D1 | La inscripción impaga que sube de tramo | **Re-preciar el renglón abierto.** Un solo renglón que dice "Inscripción $3,300" y cambia solo. (Descartado: renglón de ajuste aparte.) |
| D2 | Qué fecha fija el tramo | **La fecha de consulta/pago.** Mientras no esté pagada vale lo que valga hoy; al pagarse queda congelada. |
| D3 | Alumno nuevo a mitad de ciclo | **Sólo colegiaturas desde su mes de ingreso.** |
| D4 | Captura de los tramos | **"Vigente desde" + monto.** El tramo aplicable es el último cuya fecha ya pasó. |

---

## 2. Lo que ya existe y no hay que volver a construir

| Pieza | Dónde | Sirve para |
|---|---|---|
| `TuitionPlan` por (ciclo, nivel) | `services/finance/internal/domain/tuition.go` | Colegiatura, calendario de meses, día de vencimiento, política de cobranza |
| `CollectionPolicy` | `domain/collection.go` | Recargo diario y pronto pago, **derivados, nunca almacenados** |
| `ExpectedToday` | `usecase/deposit.go:240` | Lo que se debe **al día de consulta**, con recargo devengado y pronto pago |
| `depositPlanner.plan` | `usecase/deposit.go:389` | Ya sabe **modificar un cargo y persistirlo** cuando entra dinero (`discounted`) |
| `RescheduleUnpaid` | `ports` + repo | Precedente de **corregir cargos impagos** desde la corrida |
| `fees jsonb` en `tuition_plans` | `migrations/finance/0013` | Precedente de lista pequeña, propia del plan, leída y escrita entera |
| `CollectDrawer` | `apps/web/.../CollectDrawer.tsx` | Registrar pago ya totaliza por alumno con `ExpectedToday` |

---

## 3. Los huecos reales (verificados en código)

**H1 · La inscripción es un solo número.** `TuitionPlan.enrollmentCents` es un `int64`.
No hay lista de conceptos ni tramos.
`Fee` **no sirve** para esto: reparte *un* monto entre varios meses del año; aquí se
necesita *un* monto que **cambia según cuándo se paga**. Son cosas distintas y meterlo en
`Fee` haría ilegibles las dos.

**H2 · El cargo es una foto inmutable.** `charge.go` documenta que `amount` es un
snapshot y que editar el catálogo nunca reescribe un cargo emitido. D1 pide lo contrario
para este caso. Se resuelve sin romper el invariante — ver §4.2.

**H3 · La corrida factura el año completo a todo inscrito activo.**
`RunBilling` (`billing.go:224`) recorre `plan.Installments(cycle.StartsOn)` para cada
enrolamiento activo, sin mirar cuándo entró el alumno. Un alta en marzo hoy quedaría
debiendo septiembre. **Es un defecto preexistente que D3 destapa.**

**H4 · `EnrollmentRef` no trae la fecha de alta.** `ports.EnrollmentRef` tiene
`ID/StudentID/GroupID/CycleID/Status`. El campo `enrolled_at` **ya existe en el proto de
students** (`students.proto:233`), así que es mapeo, no contrato nuevo.

**H5 · El estado de cuenta del padre NO es en tiempo real.**
`GetAccount` (`finance.go:498`) es `Σ max(0, efectivo − pagado)`. No pasa por la lógica
de `ExpectedToday`. El mostrador ve el número correcto y la app del papá no. Es
exactamente lo que el dueño pidió y hoy no se cumple.

---

## 4. Diseño

### 4.1 Dominio: `EnrollmentConcept` + `PriceTier`

```go
// PriceTier is what a concept costs from a date onward.
type PriceTier struct {
    EffectiveFrom time.Time
    AmountCents   int64
}

// EnrollmentConcept is one line of the inscription: a name the family reads and
// what it costs depending on when it is settled.
type EnrollmentConcept struct {
    Name  string
    Tiers []PriceTier // ordered by EffectiveFrom, at least one
}

// AmountOn is what the concept costs as of a date: the last tier whose
// EffectiveFrom has arrived. Before the first tier, the first tier's amount.
func (c EnrollmentConcept) AmountOn(t time.Time) int64
```

`TuitionPlanSpec` gana `EnrollmentConcepts []EnrollmentConcept`.
**Un plan sin conceptos sigue significando lo que significaba** (`enrollmentCents`, un
renglón "Inscripción"), igual que se hizo con `EnrollmentDueOn` en 0013. Nada de lo ya
configurado se rompe.

Validación (misma severidad que `Fee.validate`): nombre requerido y único dentro del
plan; al menos un tramo; tramos **estrictamente crecientes** por fecha; montos > 0; cotas
`maxEnrollmentConcepts = 12`, `maxTiersPerConcept = 12`.

### 4.2 Re-precio sin romper el invariante (D1 + D2)

La forma segura es la que **este código ya usa para el recargo**:

> *"Derived, never stored. […] The ledger materialises it as one real charge at the
> moment money is applied to it, so rows exist where money moved."* — `collection.go`

Se aplica igual a la inscripción:

- **Se deriva al leer.** `GetAccount`, `ExpectedToday` y `ListCharges` piden el monto del
  tramo vigente a la fecha de consulta mientras el cargo tenga `paid_cents == 0`.
- **Se congela al cobrar.** `depositPlanner.plan` re-precia el cargo y lo persiste antes
  de asignar el dinero — el mismo carril por donde ya viaja `discounted`.
- **Se materializa en la corrida.** `RunBilling` escribe el tramo vigente en los cargos de
  inscripción impagos (`RepriceUnpaid`, gemelo de `RescheduleUnpaid`), para que la base
  concuerde con lo que la app muestra y ningún reporte mienta.

Una sola función de dominio (`AmountOn`) sirve a los tres caminos. No hay dos aritméticas.

**Regla de congelación: el primer peso congela.** Un abono parcial en enero fija el
concepto en $2,200 aunque termine de pagarse en abril. Es la lectura generosa para la
familia y la única que deja los pagos ya aplicados cuadrando. **Esta regla es
consecuencia de D1 y hay que confirmarla en la revisión del ADR** — es el único borde
donde "re-preciar el renglón" y "los pagos ya hechos no se reescriben" se tocan.

### 4.3 Identidad del cargo

`external_ref` pasa a ser por concepto:
`enrollment:<cycle>:<student>:<slug-del-concepto>`.
El `enrollment:<cycle>:<student>` de hoy se conserva para los planes sin conceptos. La
unicidad por tenant (ADR 0008) es lo que mantiene la corrida idempotente, y sigue intacta.

### 4.4 Alta a media marcha (D3)

`TuitionPlan.InstallmentsFrom(cycleStart, joinedOn)` devuelve sólo las mensualidades cuyo
mes es igual o posterior al de ingreso. `joinedOn` anterior al ciclo ⇒ año completo, que
es el comportamiento de hoy y el de todos los alumnos que ya están.

Los conceptos de inscripción del alumno nuevo se cotizan a **su fecha de alta**, y esa
fecha es también la que arranca la regla de re-precio si no paga.

---

## 5. Ejecución

Orden fijo del repo: contrato → datos → backend (TDD) → frontend → QA → gates → docs.
**TDD estricto: ninguna línea de producción sin un test que falle primero.**

### Etapa 0 — Contrato y decisión (0.5 d)

| # | Tarea | Entregable |
|---|---|---|
| T1 | **ADR 0011 — Inscripción por tramos: precio derivado hasta el primer peso** | Documenta D1–D4, la regla de congelación, y por qué no choca con ADR 0008 ni con el invariante de `charge.go` |
| T2 | Proto: `PriceTier`, `EnrollmentConcept`; campos en `TuitionPlan` y `UpsertTuitionPlanRequest`; `GetAccountResponse` gana saldo al día, recargo devengado y pronto pago | `buf lint` + `buf breaking` en verde (todo es campo nuevo ⇒ no rompe) |

### Etapa 1 — Dominio, TDD puro sin I/O (1.5 d)

| # | Tarea |
|---|---|
| T3 | `domain/enrollment_concept.go` + tabla de casos: tramos ordenados, fecha anterior al primer tramo, fecha exacta de corte, nombre duplicado, tramo sin monto, cotas |
| T4 | `TuitionPlanSpec.EnrollmentConcepts` + validación + getter; test de que un plan **sin** conceptos sigue cobrando `enrollmentCents` |
| T5 | `TuitionPlan.EnrollmentChargesOn(asOf)` → los renglones con su monto vigente. Caso del folleto como test: Preescolar al 15-ene = 7,750; al 3-mar = 8,850; al 20-abr = 9,950 |
| T6 | `TuitionPlan.InstallmentsFrom(cycleStart, joinedOn)`: alta en septiembre = año completo; alta en marzo = marzo..junio; alta antes del ciclo = año completo |

### Etapa 2 — Persistencia (1 d)

| # | Tarea |
|---|---|
| T7 | Migración `0015_enrollment_concepts.sql`: `enrollment_concepts jsonb NOT NULL DEFAULT '[]'` + check de array, **mismo criterio que `fees` en 0013** (lista pequeña, propia del plan, leída y escrita entera). RLS heredada de `tuition_plans` |
| T8 | `models.go` + `mapper.go` + test de integración (testcontainers): ida y vuelta con tres conceptos y tres tramos, y un plan viejo que sigue leyéndose |
| T9 | `RepriceUnpaid` en `ports` + repo: escribe el monto vigente **sólo** en cargos de inscripción con `paid_cents = 0`. Test de aislamiento RLS incluido |

### Etapa 3 — Corrida y cobro (2.5 d)

| # | Tarea |
|---|---|
| T10 | `EnrollmentRef.EnrolledAt` + mapeo en `adapters/studentsclient` (el campo ya viaja en el proto de students) |
| T11 | `RunBilling`: emite un cargo por concepto de inscripción, cotizado a `asOf`; usa `InstallmentsFrom` con la fecha de alta; llama `RepriceUnpaid`. `BillingResult` gana `EnrollmentRepriced` — una corrida que mueve 400 precios en silencio es una que nadie puede auditar |
| T12 | `EnsureEnrollmentCharge` (consumidor de `StudentEnrolled`): mismos conceptos, cotizados a la fecha de alta, más la colegiatura del mes de ingreso — el "primer pago" del alumno nuevo |
| T13 | `depositPlanner.plan`: re-precia y persiste el cargo de inscripción impago antes de asignar. Test de que un abono parcial **congela** el tramo |

### Etapa 4 — Lectura en tiempo real (1 d)

| # | Tarea |
|---|---|
| T14 | `GetAccount` devuelve saldo al día: monto vigente por tramo + recargo devengado − pronto pago. Reutiliza la aritmética de `ExpectedToday`, no la reimplementa |
| T15 | `ExpectedToday` cotiza los conceptos de inscripción al `when` recibido |
| T16 | Gateway: pasar los campos nuevos en `/finance/accounts/{id}` y en `/api/v1/family/students/{id}/account` |

### Etapa 5 — Frontend (2.5 d)

| # | Tarea |
|---|---|
| T17 | `TuitionPanel`: editor de conceptos de inscripción con tramos "vigente desde + monto"; **previsualización del total por tramo antes de guardar**, igual que ya hace `monthlyTotals` con las cuotas. Test de componente primero (Vitest/RTL) |
| T18 | `AccountPanel` (web) muestra el saldo al día y **por qué** cambió el monto de la inscripción |
| T19 | App del padre (Expo): estado de cuenta con el saldo al día de consulta |

### Etapa 6 — QA y compuertas (1.5 d)

| # | Tarea |
|---|---|
| T20 | E2E del folleto: configurar Preescolar 2026/2027 → alta en enero → consultar en marzo → ver $8,850 → pagar → verificar congelado |
| T21 | E2E de alta a media marcha: alumno nuevo en marzo no debe septiembre |
| T22 | Suites con flag ON y OFF; contract tests contra el OpenAPI; cobertura ≥ 80% en la lógica nueva |
| T23 | Compuertas **security** y **tech-lead**; actualizar `MODULES.md`, `FLAGS.md` y la referencia de API generada |

---

## 6. Estimación

| Etapa | Días |
|---|---:|
| 0 · Contrato y decisión | 0.5 |
| 1 · Dominio | 1.5 |
| 2 · Persistencia | 1.0 |
| 3 · Corrida y cobro | 2.5 |
| 4 · Lectura en tiempo real | 1.0 |
| 5 · Frontend | 2.5 |
| 6 · QA y compuertas | 1.5 |
| **Total** | **10.5 días hábiles** (≈ 2 semanas y media, un ingeniero) |

Paralelizable a partir de la Etapa 2: el frontend (T17) puede arrancar contra el contrato
congelado de T2 mientras el backend hace las Etapas 3 y 4. Con dos carriles, ≈ 7 días.

Las Etapas 0–3 son secuenciales de verdad: comparten `tuition.go` y `billing.go`.

---

## 7. Riesgos

| Riesgo | Qué pasa | Mitigación |
|---|---|---|
| **El re-precio y el abono parcial** | Una familia abona $1,000 en enero y paga el resto en abril. ¿Debe $1,200 o $3,400? | Regla explícita en el ADR: **el primer peso congela** ⇒ debe $1,200. Confirmar con el dueño al revisar el ADR |
| **H3 es un defecto vivo** | Si hoy se da de alta un alumno a media marcha, ya se le está cargando el año completo | T11 lo corrige. Revisar si hay altas así en `dev` antes de desplegar |
| **Dos aritméticas del tramo** (Go y TypeScript) | La pantalla promete un total y el servidor cobra otro | El front sólo **previsualiza**; el servidor manda. Mismo trato que `feeShares`, que ya vive duplicada a conciencia |
| **`dev` es la demo en vivo** | Un cambio a medias en el cobro se ve en la demo diaria | Rama propia hasta que las dos compuertas pasen; `dev` sólo recibe el módulo completo |
| **Migración sobre datos reales** | `dev` en GCP tiene planes ya configurados | El default `'[]'` deja todo plan existente cobrando exactamente lo que cobra hoy |
| **El día del colegio es el día UTC** | El colegio está en UTC-6 y el servicio trata el día UTC como el día escolar (`resolveAsOf` trunca `now().UTC()`). A las 18:30 hora local ya es el día siguiente para el sistema: una familia cotizada el 28-feb a esa hora paga el precio de marzo | **Preexistente y transversal** — ya afecta vencimientos, recargo diario y ventana de pronto pago. Corregirlo sólo en la escalera la dejaría 6 h desfasada del recargo en el mismo estado de cuenta, que es peor que un sistema desfasado de forma consistente. Va en un solo lugar, donde `now()` se convierte en fecha, contra una zona horaria del colegio que el servicio todavía no tiene. **Carry-forward, fuera de M18** |

---

## 8. Lo que este plan NO hace

- No toca becas (M08) ni su interacción con los tramos.
- No cambia la regla de recargo ni la de pronto pago: se reutilizan tal cual.
- No genera un flag nuevo. Viaja dentro de `module_finance`.
- No implementa notificación al padre cuando su inscripción sube de tramo. Vale la pena y
  es otro trabajo (comms, M10).
