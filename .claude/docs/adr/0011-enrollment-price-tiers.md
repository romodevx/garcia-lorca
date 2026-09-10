# ADR 0011 — Inscripción por conceptos y tramos: precio derivado hasta el primer peso

- **Estado:** aceptado (2026-09-09)
- **Contexto del módulo:** M05 Finance / M15 Colegiaturas · plan M18
- **Decide:** dueño del producto (checkpoint de `/plan`, decisiones D1–D4)

## Contexto

El folleto de costos del colegio no cobra la inscripción como un monto. La cobra como
**tres conceptos**, y el precio de cada uno depende **del mes en que la familia lo cubre**:

| Concepto | Ene/Feb | Marzo | Abril a Sept |
|---|---:|---:|---:|
| Inscripción | $2,200 | $3,300 | $4,400 |
| Gastos adm y seguro | $4,700 | $4,700 | $4,700 |
| Cuota Asociación Padres de Familia | $850 | $850 | $850 |
| **Total** | **$7,750** | **$8,850** | **$9,950** |

La escalera es el incentivo entero: inscribirse tarde cuesta el doble. Y no se detiene al
emitirse el cargo — el dueño lo dijo sin rodeos: *"si no paga cada mes le cargas la
colegiatura actual más la inscripción según corresponda el mes"*. Una inscripción impaga
en marzo vale $3,300 aunque se haya generado en enero.

El sistema sabe guardar un número: `TuitionPlan.enrollmentCents`, un `int64`.

`Fee` (ADR del plan M17, `migrations/finance/0013`) no resuelve esto. Reparte **un** monto
entre varios meses del año — "la cuota de julio, mitad en septiembre y mitad en octubre".
Aquí hace falta lo contrario: **un** monto que cambia según cuándo se paga. Forzar una
idea dentro de la otra dejaría las dos sin poder explicarse.

El obstáculo real está en `charges`. `domain/charge.go` documenta que el monto es una
foto tomada al emitirse, y que editar el catálogo **nunca** reescribe un cargo ya emitido.
Es lo que permite auditar: el cargo del año pasado sigue cuadrando contra la tabla del año
pasado. La escalera del folleto pide, de frente, que un cargo emitido cambie de precio.

## Decisión

### 1 · La inscripción es una lista de conceptos, cada uno con tramos

`TuitionPlan` gana `EnrollmentConcepts []EnrollmentConcept`, donde un concepto es un
nombre y una escalera de precios:

```go
type PriceTier struct {
    EffectiveFrom time.Time
    AmountCents   int64
}

type EnrollmentConcept struct {
    Name  string
    Tiers []PriceTier // ordenados por EffectiveFrom, al menos uno
}
```

`AmountOn(t)` devuelve **el último tramo cuya fecha ya llegó**. Antes del primero, el
primero — un plan no cobra cero por haberse consultado temprano.

**"Vigente desde" y no un conjunto de meses** (decisión D4). "De abril a septiembre" son
tres renglones y no seis casillas, y nadie tiene que validar que no quede un mes huérfano
ni repetido: la escalera cubre el eje completo por construcción.

**Un plan sin conceptos sigue significando lo que significaba**: un renglón "Inscripción"
por `enrollmentCents`. Es la misma disciplina que `enrollment_due_on` en 0013 — lo ya
configurado sigue cobrando exactamente lo que cobra hoy.

### 2 · El precio se deriva al leer y se congela cuando entra dinero

El dueño eligió **re-preciar el renglón abierto** (D1): el estado de cuenta dice
"Inscripción $3,300" y cambia solo, en vez de acumular un renglón de ajuste. Y el tramo lo
fija **la fecha de consulta o de pago** (D2), no la de emisión.

Eso se implementa con el patrón que este código **ya usa para el recargo**:

> *"Derived, never stored. […] The ledger materialises it as one real charge at the moment
> money is applied to it, so rows exist where money moved."* — `domain/collection.go`

Aplicado a la inscripción, son tres caminos y **una sola aritmética**:

- **Al leer** (`GetAccount`, `ExpectedToday`, `ListCharges`): mientras el cargo tenga
  `paid_cents = 0`, vale `AmountOn(fecha de consulta)`.
- **Al cobrar** (`depositPlanner.plan`): el cargo se re-precia y se persiste **antes** de
  asignar el dinero. Es el carril por el que ya viaja `discounted`, el descuento por
  pronto pago; no se abre uno nuevo.
- **En la corrida** (`RunBilling` → `RepriceUnpaid`, gemelo de `RescheduleUnpaid`): escribe
  el tramo vigente en los cargos de inscripción impagos, para que la base concuerde con lo
  que la app muestra y ningún reporte mienta.

El invariante de `charge.go` no se rompe: **un cargo que ha recibido dinero es inmutable**.
Lo que cambia es un cargo que nadie ha pagado, que es una cotización vigente y no un hecho
contable.

### 3 · El primer peso congela

Un abono parcial fija el tramo. Una familia que abona $1,000 en enero contra la
inscripción queda debiendo $1,200, no $3,400, aunque termine de pagar en abril.

Es la única regla que deja los pagos ya aplicados cuadrando: re-preciar un cargo con
dinero encima significaría que el recibo que la familia tiene en la mano contradice el
estado de cuenta. Y de las lecturas posibles es la generosa, que es la que un colegio
defiende frente a un padre en el mostrador.

### 4 · Identidad del cargo

`external_ref` pasa a ser por concepto:

```
enrollment:<cycle>:<student>:<slug-del-concepto>
```

Los planes sin conceptos conservan `enrollment:<cycle>:<student>`. La unicidad por tenant
que ADR 0008 estableció es lo que mantiene idempotente la corrida, y sigue intacta: tres
conceptos son tres referencias distintas, y re-correr la generación no escribe ninguna
segunda vez.

### 5 · Persistencia: `jsonb` en `tuition_plans`

`enrollment_concepts jsonb NOT NULL DEFAULT '[]'`, con check de que sea un arreglo. El
mismo criterio que `fees` (0013) y `schedule` (0010), por las mismas razones: la lista es
pequeña, pertenece por completo a su plan, se lee entera y se escribe entera. Nadie va a
preguntar "qué niveles cobran cuota de asociación".

## Consecuencias

- El folleto se puede capturar tal como está impreso, y el sistema cobra lo que dice.
- El padre ve **un** renglón por concepto, con el precio de hoy. No hay que explicarle un
  "ajuste por inscripción tardía" que él nunca pidió.
- **La escalera funciona como incentivo también dentro del sistema**: la deuda sube sola
  al pasar el mes, que es exactamente lo que el colegio quiere que pase.
- `BillingResult` gana `EnrollmentRepriced`. Una corrida que mueve cuatrocientos precios en
  silencio es una que nadie puede auditar — la misma razón por la que ya reporta
  `EnrollmentMoved`.
- El monto vigente se calcula en **una** función de dominio (`AmountOn`), consumida por los
  tres caminos. El frontend la duplica sólo para **previsualizar** antes de guardar, con el
  mismo trato consciente que ya recibe `feeShares`: la pantalla promete, el servidor manda.
- La migración es inerte sobre datos reales: el default `'[]'` deja todo plan existente
  cobrando lo que cobra hoy.

## Alternativas descartadas

- **Renglón de ajuste aparte** ("Inscripción $2,200" + "Ajuste inscripción marzo $1,100").
  Era la recomendación técnica: cargos estrictamente inmutables, pagos parciales sin
  ningún borde, y el estado de cuenta explica **por qué** subió sin que nadie pregunte. El
  dueño lo descartó por la lectura del padre — dos renglones para una cosa que el folleto
  presenta como una. Decisión de producto, tomada con el costo a la vista.
- **Meterlo en `Fee`.** Reutiliza estructura existente y cero migración, pero `Fee`
  significa "un monto repartido entre meses". Un tipo que significara las dos cosas no
  significaría ninguna, y las dos viven en el mismo `TuitionPlan`.
- **Un `TuitionPlan` por tramo**, con vigencias. Modela el tiempo de forma uniforme, pero
  multiplica por tres las filas de la tabla de colegiaturas para expresar una escalera que
  sólo afecta a la inscripción, y obliga a elegir plan en cada lectura de colegiatura, que
  no cambia.
- **Congelar el tramo al emitir el cargo.** Es lo que el invariante actual quiere, y anula
  el incentivo entero: quien se inscribe en enero y no paga hasta septiembre pagaría
  $2,200. El colegio dejaría de cobrar la diferencia que el folleto anuncia.

## Relacionado

- `.claude/docs/PLAN-m18-inscripcion-por-tramos.md` — plan completo, decisiones D1–D4 y riesgos.
- ADR 0008 (`external_ref`) — de ahí viene la identidad por referencia que hace idempotente
  la corrida con tres conceptos.
- ADR 0002 (payment knot) — el re-precio al cobrar ocurre dentro de esa transacción, bajo
  el mismo row lock.
- `migrations/finance/0013_fees_and_enrollment_due.sql` — precedente de `jsonb` para una
  lista propia del plan, y de un default que deja intacto lo ya configurado.
