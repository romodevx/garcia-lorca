<!-- /autoplan restore point: /Users/andresromo/.gstack/projects/romodevx-garcia-lorca/dev-autoplan-restore-20260831-204739.md -->

# PLAN M16 — Automatización financiera y configuración escolar

Rama: `dev` · Origen: petición del dueño tras revisar el sistema con la escuela.
Alcance UI: **sí**. Alcance DX: no (el usuario es personal escolar, no desarrolladores).

---

## 0. La petición, tal como llegó

Siete bloques. Los resumo sin reinterpretarlos; la revisión decide qué es real.

### 1 · Configuración general
- **Recargos y pronto pago:** recargo diario editable (hoy 10 pesos fijos), fecha límite
  sin recargo, descuento por pronto pago (ej. 5% en los primeros 5 días hábiles), aplicado
  automáticamente según la fecha de pago.
- **Calendario de feriados y vacaciones:** módulo donde el personal carga los días; el
  sistema los respeta al calcular plazos; si un vencimiento cae en feriado se recorre al
  siguiente día hábil conservando el beneficio o el costo. El mismo calendario sirve para
  actividades escolares.
- **Cuotas por nivel:** distribución flexible de las colegiaturas (ej. 11 meses partidos,
  mitad en diciembre y mitad en marzo), configurable por nivel y por período.

### 2 · Pagos y colegiaturas
- **Conceptos automáticos:** eliminar la creación manual por alumno. El sistema genera
  inscripción del siguiente ciclo, 11 colegiaturas del ciclo actual y otros cargos.
- **Registro de pago con detalle exacto:** modal con búsqueda de alumno, fecha exacta del
  pago, tipo de tarjeta (débito/crédito), y tres campos: **Monto Esperado** (no editable),
  **Monto Capturado** (editable) y **Diferencia** (calculada). Botón "Aplicar Pronto Pago"
  manual para el padre puntual que falló un día.
- **Deuda total y aplicación automática:** deuda anual calculada; los pagos se aplican
  solos a los meses correspondientes; el excedente pasa al siguiente mes
  (paga $6,000 → sep–feb pagados; paga $1,500 → $1,000 a sep y $500 a oct).
- **Descuentos maternales** configurables.

### 3 · Estudiantes
- **Multicontacto:** varios teléfonos y correos por alumno.
- **Descarga en Excel:** listado con los filtros aplicados, conteo por filtro en vivo y
  cantidad de alumnos por salón.

### 4 · Comunicaciones
- Avisos dirigidos a un alumno, un grupo, varios grupos o un nivel completo.

### 5 · Inventario
- Módulo versátil para uniformes, libros y cualquier artículo que venda el colegio, con
  alta de piezas al momento de la compra.

### 6 · Finanzas
- **Ingresos:** vista de ingresos totales por concepto (colegiaturas, libros, uniformes,
  inscripciones, otros).
- **Registro de pagos** como interfaz aparte: buscar alumno, capturar solo monto y fecha,
  y que el sistema distribuya internamente sin intervención manual.

### 7 · Flujo automático completo
Al registrar la fecha exacta: validar contra feriados → aplicar descuento o recargo →
calcular esperado → comparar con capturado → generar diferencia → aplicar a conceptos →
actualizar deuda → generar estado de cuenta.

---

## 1. Verdad de terreno — qué de esto YA existe

Leído en el código, no supuesto. Esto cambia el tamaño del trabajo de forma sustancial.

| Lo que pide la petición | Estado real | Dónde |
|---|---|---|
| Colegiatura mensual por nivel | ✅ **existe** | `domain.TuitionPlan.monthlyCents` |
| Inscripción del ciclo | ✅ **existe** | `TuitionPlan.enrollmentCents` |
| N colegiaturas por ciclo | ✅ **existe** | `TuitionPlan.months`, `firstBilledMonth` |
| Día de vencimiento | ✅ **existe** | `TuitionPlan.dueDay` |
| Recargo por atraso | ⚠️ **existe pero MENSUAL** | `TuitionPlan.lateFeeCents`, `accrueLateFees` |
| Generación automática de cargos | ✅ **existe** | `FinanceService.RunBilling` |
| Cargo de inscripción al inscribirse | ✅ **existe** (por evento) | `StudentEnrolled` → finance |
| Registro de pago idempotente | ✅ **existe** | `RegisterPayment` + `0002_payment_idempotency` |
| Inventario con piezas y tallas | ✅ **existe** | `uniforms`, 4 migraciones, ADR 0007 |
| Aritmética de días hábiles | ✅ **existe** (en uniforms) | `PickupWindowBusinessDays` |
| Notificaciones a familias | ✅ **existe** (por alumno) | `comms`, guardianes vía `students` |
| **Recargo DIARIO configurable** | ❌ falta | el actual es un monto fijo por mes vencido |
| **Descuento por pronto pago** | ❌ falta | no existe el concepto |
| **Calendario de feriados** | ❌ falta | nada en el repo lo modela |
| **Colegiatura partida por período** | ❌ falta | `months` es uniforme |
| **Pago sin elegir concepto** | ❌ falta | `RegisterPayment(chargeID, …)` exige el cargo |
| **Multicontacto por alumno** | ❌ falta | `students.guardian_name` es UN texto |
| **Export a Excel** | ❌ falta | — |
| **Aviso a grupo / nivel** | ❌ falta | `comms` resuelve por alumno |
| **Inventario genérico (libros)** | ⚠️ parcial | el modelo es de prendas y tallas |
| **Vista de ingresos por concepto** | ❌ falta | hay cargos y pagos, no el agregado |
| **Descuento maternal** | ❌ falta | — |

**Lectura:** el 40% de lo pedido ya está construido. Lo que falta se divide en tres clases
muy distintas: campos nuevos (baratos), un calendario nuevo (medio), y **un cambio en el
modelo de pago** (caro, y es el corazón de la petición).

---

## 2. El cambio que no es un feature

`RegisterPayment(ctx, chargeID, amountCents, …)` ata un pago a **un cargo**. La petición
pide lo contrario: capturar un monto contra el **alumno** y que el sistema lo reparta.

```
HOY                                    LO QUE SE PIDE
pago ──► cargo (1:1)                   pago ──► alumno
                                              │
                                              ├─► sep  $1,000  (liquidado)
                                              └─► oct  $  500  (parcial)
```

Eso toca el nudo de pagos que ya está probado: idempotencia, `ChargeSettled`,
`external_ref`, y la UNIQUE de cargos por ciclo. No es un endpoint nuevo; es reabrir la
pieza más delicada del módulo de finanzas.

---

## 3. Preguntas que el plan NO responde y la revisión debe resolver

1. ¿El recargo diario **reemplaza** al mensual o convive? Hay cargos ya emitidos con la
   regla mensual.
2. ¿El pronto pago es porcentaje o monto fijo? La petición dice ambas cosas
   ("ej: 5%" y "cantidad fija configurable").
3. Si un pago se reparte entre meses, ¿qué pasa con el recargo ya emitido de un mes que
   se liquida tarde?
4. ¿"Descuento maternal" es por hermano, por madre trabajadora, u otra cosa? El término
   no aparece en ninguna parte del sistema.
5. ¿El estado de cuenta automático es un PDF, una pantalla, o un correo?

---
---

# REVISIÓN /autoplan — FASE 1: CEO (estrategia y alcance)

Modo: **SELECTIVE EXPANSION**. Voces: Claude subagente ✅ · Codex ❌ `[binary not found]` → **[subagent-only]**.

## Auditoría previa

- `git log`: el repo viene de desplegar la demo a GCP. El último trabajo de producto fue
  M14/M15 (Docentes y Colegiaturas) — **exactamente el terreno que esta petición amplía**.
- **No existe `TODOS.md`.** Todo lo que esta revisión difiera necesita dónde vivir.
- `MODULES.md` ya tiene planeado **M08 Scholarships** ("becas y descuentos", prioridad
  *Should*) con 7 archivos Go de andamio y flag registrada en OFF.
- La demo está desplegada y **vacía** (0 alumnos, 0 usuarios) por decisión del dueño.

### Chequeo retrospectivo

M15 (`.claude/docs/PLAN-m15-colegiaturas.md`) construyó hace días el tabulador por
(ciclo, nivel), la corrida mensual y el recargo. **Esta petición pide reabrir esas tres
piezas.** Un módulo que se reabre a los pocos días de terminarse no es mala suerte: es
señal de que se especificó sin el usuario enfrente. Vale la pena decirlo porque cambia
cómo conviene planear lo que sigue.

---

## 0A. Desafío de premisas

| # | Premisa de la petición | Veredicto |
|---|---|---|
| P1 | *"Eliminar la creación manual de conceptos por alumno"* | ❌ **Ya no existe.** `RunBilling` genera los cargos del padrón desde M15. Lo que el director vio manual es la pantalla de **conceptos** (catálogo), que es otra cosa |
| P2 | *"Recargo actualmente 10 pesos, debe ser editable"* | ⚠️ **Ya es editable**: `TuitionPlan.lateFeeCents` por (ciclo, nivel). Lo que NO es, es **diario** |
| P3 | *"Descuento por pronto pago (ej: 5%) - cantidad fija configurable"* | ❌ **Autocontradictorio**: 5% es un porcentaje, "cantidad fija" es un monto. Son reglas distintas y dan números distintos |
| P4 | *"El sistema debe calcular la deuda total anual"* | ✅ Válida, y es barato: los cargos ya existen, falta el agregado |
| P5 | *"Si hay excedente, aplicar al siguiente mes"* | ✅ Válida, pero **es el cambio más caro de toda la lista** (§2) |
| P6 | *"Módulo versátil para uniformes, libros y cualquier artículo"* | ⚠️ Subestima el costo: el modelo de `uniforms` es prendas×tallas con máquina de estados de 7 estados. Un libro no tiene talla |
| P7 | *"Descuentos maternales"* | ❌ **Término no definido.** No aparece en el sistema ni en el glosario del proyecto. ¿Hermanos? ¿Madre trabajadora? ¿Madres solteras? |
| P8 | *"Mantiene el proceso manual actual de captura del estado de cuenta del director"* | ⚠️ Revela algo que el plan no dice: **el director hoy lleva las cuentas en otro lado** y captura a mano. Eso es el problema real |

### Las tres preguntas

1. **¿Es el problema correcto?** Parcialmente. La petición está escrita como lista de
   funciones, pero P8 delata el problema de fondo: **el director lleva su contabilidad
   fuera del sistema y transcribe**. Si el sistema no le gana a su método actual, no lo
   va a usar aunque tenga las 20 funciones.
2. **¿Cuál es el resultado real?** Que la escuela firme y **deje su método actual**.
   Todo lo que no acerque a eso es adorno.
3. **¿Y si no hacemos nada?** El director sigue transcribiendo. Dolor real, y él ya lo
   nombró — por eso pidió el módulo de registro de pagos por separado.

---

## 0B. Mapa de código existente

| Sub-problema | ¿Existe? | Dónde |
|---|---|---|
| Colegiatura, inscripción, meses, día de pago | ✅ completo | `domain.TuitionPlan` |
| Generación automática de cargos | ✅ completo | `usecase.RunBilling` |
| Recargo por mora | ⚠️ mensual, no diario | `accrueLateFees` |
| **Descuento sobre un cargo** | ✅ **completo** | `charge.go:45` `discount`, `Effective() = amount - discount` |
| Pago idempotente | ✅ completo | `RegisterPayment` + migración 0002 |
| Aritmética de días hábiles | ✅ existe (en `uniforms`) | `PickupWindowBusinessDays` |
| Inventario con piezas | ✅ completo para prendas | `uniforms`, 4 migraciones |
| Notificación a familias | ✅ por alumno | `comms` + guardianes |
| Becas y descuentos como módulo | ⚠️ **planeado, andamio** | M08, flag OFF, 7 archivos |

**Lo que esto cambia:** el pronto pago y el descuento maternal **no necesitan modelo
nuevo** — necesitan una regla que calcule un `discount` que la entidad ya sabe aplicar.
Y el descuento maternal es una **beca**, o sea M08, no finanzas.

---

## 0C. Estado soñado

```
  HOY                        ESTA PETICIÓN               IDEAL A 12 MESES
  ──────────────────────     ─────────────────────       ──────────────────────
  El director lleva sus      El sistema calcula lo       El director no transcribe
  cuentas aparte y las       esperado y reparte los      nada: captura el pago y
  transcribe al sistema.     pagos solo.                 el sistema hace el resto.

  Cargos automáticos por     + recargo diario            Reglas de cobro como
  ciclo y nivel (M15).       + pronto pago               configuración, no como
                             + calendario de feriados    código, para una segunda
                                                          escuela con otras reglas.
  Pago atado a un cargo.     Pago atado al alumno,       Estado de cuenta que el
                             repartido solo.             padre ve sin preguntar.
```

**Delta:** avanza hacia el ideal en lo esencial (el reparto automático). **Riesgo:** si
las reglas de cobro se codifican en vez de configurarse, la segunda escuela obliga a
reescribir. La petición dice "configurable" siete veces — hay que tomárselo en serio.

---

## 0C-bis. Alternativas de implementación

```
ENFOQUE A: "Construir la lista tal como llegó, en orden"
  Esfuerzo: XL (humano ~6-8 semanas / CC ~5-7 días)
  Riesgo:   Alto
  Pros:     - El director ve exactamente lo que pidió.
            - No hay que discutir alcance con él.
  Contras:  - Reabre el nudo de pagos antes de saber si el reparto es correcto.
            - Mezcla lo que cierra la venta con lo que es adorno (Excel, maternal).
            - Siete "configurable" sin definir se vuelven siete decisiones a mitad
              de la implementación.

ENFOQUE B: "El motor de cobro primero; lo demás cuelga de él"
  Resumen: una sola pieza nueva —el CALENDARIO DE COBRO— que sabe de días hábiles,
           feriados, vencimientos, recargos y descuentos. Todo lo demás (pronto pago,
           recargo diario, monto esperado, estado de cuenta) son lecturas de ella.
           Luego el reparto de pagos. Luego lo cosmético.
  Esfuerzo: L  (humano ~4 semanas / CC ~3-4 días)
  Riesgo:   Medio
  Pros:     - "Monto esperado" deja de ser un cálculo disperso y pasa a tener un dueño.
            - El calendario de feriados sirve a finanzas Y a actividades escolares,
              como el propio director pidió.
            - Cada función posterior es pequeña porque la difícil ya está.
  Contras:  - La primera semana no produce nada visible para el director.

ENFOQUE C: "Solo lo que el director toca a diario"
  Resumen: registro de pagos con reparto + estado de cuenta. Nada más.
  Esfuerzo: M  (humano ~2 semanas / CC ~1-2 días)
  Riesgo:   Bajo
  Pros:     - Ataca el dolor que él mismo nombró (P8): dejar de transcribir.
            - Se puede enseñar en dos semanas y validar antes de seguir.
  Contras:  - Deja fuera el calendario, y sin él el "monto esperado" es aproximado.
            - El director puede leerlo como que no se le hizo caso.
```

**RECOMENDACIÓN: ENFOQUE B**, con la primera entrega recortada a lo de C.
El calendario de cobro es la pieza de la que cuelgan seis de las peticiones; construirla
primero hace que las demás sean pequeñas. Pero la primera cosa que el director debe VER
es el registro de pagos con reparto, porque es su dolor declarado.

**Decisión (P1 completitud + P2 radio de impacto):** ENFOQUE B. **Marcada TASTE DECISION**
— A y C son defendibles y dependen de cuándo es la próxima junta con la escuela, que es
información que yo no tengo.

---

## 0D. Alcance: qué entra, qué se difiere

Aplicando P2 (radio de impacto + <1 día CC → aprobar) y P3 (pragmático):

| # | Petición | Decisión | Razón |
|---|---|---|---|
| 1 | Calendario de feriados y días hábiles | **ACEPTAR** | Es la base de seis peticiones. Y el director ya dijo que lo quiere también para actividades |
| 2 | Recargo diario configurable | **ACEPTAR** | Cuelga del calendario; sin él es una resta de fechas equivocada |
| 3 | Pronto pago | **ACEPTAR** | Mismo motor. Usa el `discount` que ya existe |
| 4 | Monto esperado / capturado / diferencia | **ACEPTAR** | Es la pantalla que el director usa a diario |
| 5 | Reparto automático del pago entre meses | **ACEPTAR** | Es el corazón de la petición |
| 6 | Deuda total y meses pagados visibles | **ACEPTAR** | Sale gratis del reparto |
| 7 | Colegiatura partida por período | **DIFERIR** | Solo la usa el caso "mitad en diciembre". Cuelga del calendario; hacerlo después es barato |
| 8 | Multicontacto por alumno | **ACEPTAR** | 1 migración + 1 pantalla. Radio de impacto chico |
| 9 | Export a Excel + conteos por filtro | **ACEPTAR** el conteo · **DIFERIR** el Excel | El conteo son horas; el Excel bien hecho (filtros, formato) es un día y nadie se queda sin trabajar por no tenerlo |
| 10 | Avisos por grupo / nivel | **ACEPTAR** | `comms` ya resuelve destinatarios; falta la consulta por grupo |
| 11 | Inventario genérico (libros) | **DIFERIR** | El modelo actual es prendas×tallas. Generalizarlo es un rediseño, no un campo |
| 12 | Vista de ingresos por concepto | **ACEPTAR** | Una consulta agregada sobre datos que ya existen |
| 13 | Descuento maternal | **DIFERIR a M08** | Es una **beca**, y M08 ya está planeado para eso. Meterlo en finanzas duplica el concepto |
| 14 | Estado de cuenta automático | **ACEPTAR** la pantalla · **DIFERIR** el PDF/correo | La pantalla sale del reparto; el documento es otro trabajo |

**Aceptadas: 10 · Diferidas: 4.** Ninguna se rechaza — todas son razonables, cuatro
simplemente no van en la primera entrega.

---

## 0E. Interrogatorio temporal

| Momento | Lo que hay que decidir AHORA |
|---|---|
| **Hora 1** | ¿El recargo diario **sustituye** al mensual de M15, o conviven? Hay cargos emitidos con la regla vieja. **Recomendación: sustituye**, y se migran los pendientes |
| **Horas 2-3** | El pronto pago: **porcentaje o monto fijo** (P3). Y sobre qué base — ¿solo colegiatura o también inscripción? |
| **Horas 4-5** | La sorpresa: si un pago se reparte y liquida tarde un mes que **ya tenía recargo emitido**, ¿el recargo se conserva, se perdona, o se recalcula? Sin esta regla el reparto produce saldos que nadie sabe explicar |
| **Hora 6+** | Se va a desear haber tenido: un **estado de cuenta** que muestre el porqué de cada peso, no solo el saldo. Si el director no puede explicarle a un padre de dónde salió un número, no usa el sistema |

---

## 0F. Confirmación de modo

**SELECTIVE EXPANSION**, **ENFOQUE B**, 10 peticiones aceptadas y 4 diferidas.

---

## Sección 1 — Arquitectura

### Grafo, antes y después

```
HOY                                    CON M16
───────────────────────                ─────────────────────────────────
config  (ciclos, niveles,              config  ── + CALENDARIO ESCOLAR ◄── pieza nueva
         grupos, materias)                        (feriados, vacaciones,
   │                                               días hábiles)
   ▼                                        │
finance ── TuitionPlan                      ▼
        ── RunBilling ──► charges     finance ── TuitionPlan (+ regla de recargo)
        ── RegisterPayment(chargeID)          ── RunBilling  ──► charges
   │                                          ── BillingCalendar ◄── consulta config
   ▼                                          ── RegisterPayment(studentID) ── NUEVO
PaymentRegistered ──► comms, students         └──► reparto ──► N charges
                                          │
                                          ▼
                                    PaymentRegistered (sin cambio de contrato)
```

**Acoplamiento nuevo:** `finance → config` para leer el calendario. Ya existe esa
dirección (`CONFIG_BASE_URL` en `finance_env`), así que no se agrega un borde nuevo al
grafo — se usa uno que ya está.

**Dónde vive el calendario:** en `config`, no en `finance`. El director pidió el mismo
calendario para **actividades escolares**, y actividades no es finanzas. Ponerlo en
finanzas obligaría a comms y a config a pedirle fechas al módulo de dinero.

### Hallazgo 1.1 — 🔴 El reparto choca con `uq_charges_concept_per_cycle`

`migrations/finance/0001_init.sql:67` tenía `UNIQUE (tenant_id, student_id, cycle_id,
concept_id)` — un alumno solo puede deber un concepto **una vez por ciclo**.
`0003_charge_external_ref.sql` la reemplazó por dos índices: uno que conserva esa regla
para los cargos propios (`external_ref = ''`) y otro sobre `(tenant_id, external_ref)`
para los que vienen con referencia.

**Consecuencia para esta petición:** las 11 colegiaturas solo pueden coexistir porque
cada una lleva su `external_ref` (mes). Entonces **el reparto de un pago debe ordenar por
esa referencia**, no por fecha de creación ni por id. Y la "colegiatura partida"
(mitad diciembre, mitad marzo) es un `external_ref` más, no un modelo nuevo — lo cual la
abarata mucho y justifica haberla diferido sin drama.

### Hallazgo 1.2 — 🔴 El reparto necesita ser una transacción, no un bucle

Repartir $6,000 entre seis meses son **seis escrituras de cargo + un pago**. Si el
proceso muere a la mitad, el alumno queda con tres meses pagados y $3,000 sin registrar
en ningún lado. La idempotencia actual protege **un** pago contra **un** cargo
(`0002_payment_idempotency`); no protege un reparto parcial.

**Decisión (P1):** el reparto es una sola transacción con la misma clave de idempotencia
que hoy. Reintentar con la misma clave devuelve el mismo resultado, no un segundo reparto.

### Las cuatro rutas del flujo nuevo

```
  FELIZ:  captura $6,000 el 3 de septiembre
          → calendario dice: día hábil, dentro de los 5 primeros → pronto pago
          → esperado = 6 × colegiatura − descuento
          → reparte sep..feb, marca pagados
          → PaymentRegistered  ✅

  NULA:   el alumno no tiene cargos emitidos todavía (la corrida no ha pasado)
          → hoy: el reparto no tiene a qué aplicar
          → 🔴 GAP: ¿se rechaza, o se crea un saldo a favor?

  VACÍA:  captura $0  → 🟡 debe rechazarse en el borde, no crear un pago de cero

  ERROR:  el calendario no responde (config dormido en scale-to-zero)
          → 🔴 GAP: ¿se bloquea la captura, o se registra sin descuento?
          → Un pago que se registra con el monto esperado EQUIVOCADO es peor que
            uno que no se registra
```

### Escalado y puntos de falla

- **Qué truena primero:** el reparto sobre un alumno con muchos cargos abiertos es
  O(n) escrituras. Con 11 meses + inscripción + uniformes, n≈15. Irrelevante.
- **SPOF nuevo:** `config` pasa a estar en el camino de **registrar un pago**. Hoy no lo
  está. Si config no responde, la caja de la escuela se detiene. Ver ruta ERROR.

### Rollback

| Qué | Cómo se revierte |
|---|---|
| Regla de recargo mal configurada | Es configuración: se corrige y se vuelve a correr |
| Un reparto equivocado | ⚠️ **No hay reverso de pago hoy.** Ver Hallazgo 2.1 |
| Calendario con un feriado de más | Se edita el día; los cálculos futuros lo toman |

---

## Sección 2 — Mapa de errores y rescates

| Ruta | Qué puede salir mal | Clase |
|---|---|---|
| `RegisterStudentPayment` (nuevo) | el alumno no tiene cargos abiertos | `KindInvalid` |
| | el monto excede la deuda total | (sobrepago — ver 2.2) |
| | `config` no responde al pedir el calendario | timeout |
| | el reparto falla a mitad | (transacción) |
| `BillingCalendar.ExpectedAmount` | fecha de pago futura | `KindInvalid` |
| | no hay plan de colegiatura para ese nivel | `KindNotFound` |
| Alta de feriado | día duplicado | `KindConflict` |
| | rango invertido (fin antes que inicio) | `KindInvalid` |

| Clase | ¿Rescatado? | Acción | Qué ve el usuario |
|---|---|---|---|
| alumno sin cargos | **N ← GAP** | — | debe decir "corre la facturación del ciclo primero" |
| sobrepago | **N ← GAP CRÍTICO** | — | ver 2.2 |
| calendario caído | **N ← GAP CRÍTICO** | — | ver 2.3 |
| reparto parcial | **N ← GAP** | — | se cierra con la transacción de 1.2 |
| día duplicado | Y (constraint) | rechaza | mensaje en el formulario |

### Hallazgo 2.1 — 🔴 No existe reverso de pago, y el reparto lo vuelve urgente

Hoy un pago mal capturado se corrige... no se corrige. No hay `VoidPayment`. Con el modelo
actual el daño es un cargo mal saldado; **con reparto automático, un pago equivocado
mueve seis meses a la vez.** La cajera se va a equivocar — eso no es hipótesis, es
operación de mostrador.

**Decisión (P1 completitud):** `VoidPayment` entra en el alcance. Reversa por asiento
(un pago negativo que deshace el reparto), nunca un `DELETE`, para que el estado de cuenta
siga explicando de dónde salió cada peso.

### Hallazgo 2.2 — 🔴 ¿Qué pasa si pagan de más?

La petición dice *"si hay excedente, aplicar al siguiente mes"* — pero no dice qué pasa
cuando ya no hay siguiente mes. Un padre que liquida el año entero y da $500 de más.

**Decisión (P1):** el excedente se queda como **saldo a favor** del alumno, visible en el
estado de cuenta, y se consume solo en la siguiente corrida. Rechazar el pago sería peor:
el dinero ya entró a la caja.

### Hallazgo 2.3 — 🔴 Si el calendario no responde, la captura se bloquea

El caso realista: `config` lleva 15 minutos sin tráfico, está dormido, y la cajera captura
un pago. El arranque en frío tarda segundos; un timeout, más.

**Decisión (P1 + seguridad del dato):** si el calendario no responde, **la captura se
bloquea con un mensaje claro**, no se registra con el esperado sin descuento. Un pago con
el monto esperado equivocado contamina el estado de cuenta y nadie lo detecta hasta que
un padre reclama.

---

## Sección 3 — Seguridad

| # | Amenaza | Prob. | Impacto | ¿Mitigada? |
|---|---|---|---|---|
| S1 | La cajera aplica "Pronto Pago" a discreción y perdona dinero | **Alta** | Media | ❌ **GAP** — el botón manual que pidió el director es una puerta de descuento sin control |
| S2 | Reparto sobre un alumno de otro tenant | Baja | Alta | ✅ RLS ya cubre `charges` y `payments` |
| S3 | Export a Excel del padrón completo | Media | **Alta** | ⚠️ 400 nombres de menores en un archivo que sale del sistema. Diferido, pero cuando entre necesita permiso propio y bitácora |
| S4 | Aviso masivo a un nivel entero por error | Media | Media | ❌ **GAP** — mandar a "Secundaria" son ~100 familias; no hay confirmación |
| S5 | Multicontacto usado para exfiltrar datos de familias | Baja | Alta | ✅ mismo modelo de permisos que `students` |

### Hallazgo 3.1 — 🟡 El botón "Aplicar Pronto Pago" necesita dueño

El director lo pidió con buena razón (*"si el padre siempre paga puntual pero falla un
día"*). Pero un botón que perdona dinero, disponible para quien cobra, sin registro de
quién lo apretó, es un agujero de caja.

**Decisión (P1, y es barato):** el botón se queda —el caso de uso es real— pero (a) exige
`finance:write`, (b) queda asentado **quién** lo aplicó y **cuándo** en el estado de
cuenta. No es desconfianza: es que la persona que cobra pueda demostrar que no se quedó
con la diferencia.

### Hallazgo 3.2 — 🟡 Un aviso a un nivel no se puede deshacer

**Decisión (P3 pragmático):** confirmación explícita con el conteo de destinatarios
("Vas a enviar a 103 familias de Secundaria"). No es un permiso nuevo, es una pantalla.

---

## Sección 4 — Casos borde

| Interacción | Caso borde | ¿Cubierto? |
|---|---|---|
| Capturar pago | fecha del pago **anterior** al inicio del ciclo | ❌ debe rechazarse |
| Capturar pago | fecha **futura** (dedazo: 2027 en vez de 2026) | ❌ **muy probable**, debe rechazarse |
| Capturar pago | el alumno ya liquidó el año | ❌ → saldo a favor (2.2) |
| Capturar pago | doble clic en Guardar | ✅ la idempotencia ya existe |
| Pronto pago | el día 5 cae en sábado | ✅ es exactamente para lo que sirve el calendario |
| Pronto pago | el día 5 es feriado **y** sábado | ❌ recorrer una vez o dos, hay que definirlo |
| Recargo diario | el mes tiene 3 feriados seguidos | ❌ ¿se cobran o no esos días? **La petición no lo dice y cambia el número** |
| Reparto | hay un recargo emitido en un mes que se liquida tarde | ❌ **GAP** — pregunta abierta #3 del plan |
| Calendario | se carga un feriado **retroactivo** | ❌ ¿recalcula recargos ya emitidos? |
| Avisos | grupo sin alumnos | 🟡 debe decirlo, no fallar |

### Hallazgo 4.1 — 🔴 "Recargo diario" no está definido para días no hábiles

Si el recargo son 10 pesos diarios y hay un puente de 3 días, ¿son 30 pesos o 0?
El director dijo que el calendario debe respetarse "manteniendo el beneficio/costo",
lo cual sugiere que **sí se cobran**, pero es ambiguo y cambia lo que el padre paga.

**Es la pregunta con más impacto en pesos de toda la petición y no puedo decidirla yo.**
→ va al gate final.

---

## Sección 5 — Calidad

- **DRY:** la aritmética de días hábiles **ya existe** en `uniforms`
  (`PickupWindowBusinessDays`). Duplicarla en finanzas sería el clásico error de dos
  implementaciones que se desincronizan. **Decisión (P4):** se extrae a `pkg/` y ambos
  la usan. Es el único refactor que este plan justifica.
- **Nombres:** "descuento maternal" no significa nada en el dominio. Sea lo que sea, se
  nombra por lo que hace (`beca_hermanos`, `beca_madre_trabajadora`).
- **Sobre-ingeniería:** el riesgo está en el calendario. Un calendario de cobro completo
  (reglas por nivel, por período, por concepto) es un motor de reglas. **Se acota a lo
  que la escuela pidió**, no a lo que se podría configurar.
- **Sub-ingeniería:** el reparto escrito "obvio" (un bucle que resta) pierde la
  transacción y la idempotencia. Ver 1.2.

---

## Sección 6 — Pruebas

```
  RUTAS NUEVAS                                   TIPO         ¿EXISTE?
  ───────────────────────────────────────────    ──────────   ────────
  BillingCalendar: día hábil siguiente           Unit tabla   NO 🔴
  BillingCalendar: feriado + fin de semana       Unit tabla   NO 🔴
  BillingCalendar: monto esperado con pronto pago Unit tabla  NO 🔴
  BillingCalendar: recargo diario N días         Unit tabla   NO 🔴
  Reparto: exacto (6 meses justos)               Unit         NO 🔴
  Reparto: parcial ($1,500 → sep + medio oct)    Unit         NO 🔴
  Reparto: excedente sin meses → saldo a favor   Unit         NO 🔴
  Reparto: idempotente (misma clave, un reparto) Integration  NO 🔴
  Reparto: transacción (falla a la mitad)        Integration  NO 🔴
  VoidPayment deshace el reparto                 Integration  NO 🔴
  Avisos: destinatarios por grupo y por nivel    Unit         NO 🔴
  Multicontacto: RLS por tenant                  Integration  NO 🔴
```

**La prueba que da confianza:** una tabla que toma (fecha de pago, calendario, plan) y
compara el monto esperado contra números calculados a mano por el director. Si esa tabla
pasa, el sistema cobra lo que la escuela cobra. Todo lo demás es plomería.

**Riesgo de intermitencia:** cualquier prueba de calendario que use `time.Now()` es una
bomba de tiempo — literalmente falla en enero. El reloj se inyecta, como ya hace
`TuitionPlan` (`time.Now` es un parámetro, no una llamada).

---

## Sección 7 — Rendimiento

- **Reparto:** O(cargos abiertos del alumno) ≈ 15. Irrelevante.
- **Vista de ingresos:** agregación sobre `payments` de todo el ciclo. Con 400 alumnos ×
  13 conceptos ≈ 5,200 filas. Irrelevante hoy; **necesita índice** por (tenant, ciclo,
  concepto) para no degradar cuando sean 5 años de historia.
- **Conteo por filtro en vivo:** si dispara una consulta por cada tecla, son cientos de
  llamadas. **Decisión (P3):** debounce en el front, y el conteo viaja en la misma
  respuesta que la lista, no en una consulta aparte.
- **Calendario en el camino del pago:** es la ruta que más importa. Se cachea en memoria
  del servicio de finanzas por ciclo — un calendario escolar cambia dos veces al año.

---

## Sección 8 — Observabilidad

| Necesidad | Decisión |
|---|---|
| Bitácora de quién aplicó un pronto pago manual | **P1** — es dinero perdonado (3.1) |
| Bitácora de repartos | **P1** — para explicarle a un padre por qué su pago fue a marzo |
| Métrica: pagos capturados por día | Útil; sale de datos que ya existen |
| Alerta: reparto fallido | **P1** — un reparto que falla deja dinero sin aplicar |
| Estado de cuenta que explique cada peso | **P1** — es lo que hace que el director confíe |

---

## Sección 9 — Despliegue

- **Migraciones:** calendario (tabla nueva), multicontacto (tabla nueva), regla de recargo
  (columnas en `tuition_plans`), saldo a favor. Todas **aditivas** — compatibles hacia
  atrás.
- **La migración que sí tiene riesgo:** cambiar el recargo de mensual a diario con cargos
  **ya emitidos** bajo la regla vieja. **Decisión (P1):** los cargos existentes conservan
  su regla; la nueva aplica desde el ciclo siguiente. Recalcular el pasado le cambiaría
  el saldo a familias que ya pagaron.
- **Flag:** todo detrás de `module_finance`, que ya existe. El calendario, ¿bajo
  `module_config`? Sí — vive ahí.
- **Orden:** calendario → recargo/pronto pago → reparto → pantallas. Cada paso es
  desplegable solo.

---

## Sección 10 — Trayectoria

- **Deuda que introduce:** las reglas de cobro de ESTA escuela quedan en el modelo. Si
  llega una segunda con reglas distintas (recargo porcentual, no fijo), hay que
  generalizar. **Aceptable**: no hay segunda escuela todavía, y adivinar sus reglas es
  peor que adaptarse cuando exista.
- **Reversibilidad: 4/5.** Todo aditivo salvo el cambio de regla de recargo, que se acota
  por ciclo.
- **La pregunta del año:** ¿por qué el calendario vive en `config` y no en `finance`?
  Porque el director lo pidió también para actividades escolares. **Eso hay que escribirlo
  en el código**, o alguien lo "arregla" moviéndolo a finanzas.
- **Qué habilita:** el estado de cuenta que el padre ve solo (app familiar, 9c/9d), y el
  CFDI de M11 — que necesita exactamente esto: qué se cobró, cuándo se pagó, con qué
  descuento.

---

## Sección 11 — Diseño y UX

Alcance UI detectado. La pantalla central es el **modal de registro de pago**, y su
diseño decide si el director deja su método actual.

```
  ┌─ Registrar pago ──────────────────────────────┐
  │  Alumno   [buscar…            ]                │
  │  Fecha    [03/09/2026]  ← manda el cálculo     │
  │  Método   ( )efectivo (•)tarjeta               │
  │           └─ ( )débito (•)crédito              │
  │                                                │
  │  Esperado     $ 5,700   ← pronto pago aplicado │
  │  Capturado    [$ 5,700]                        │
  │  Diferencia   $     0   ← verde en 0           │
  │                                                │
  │  [Aplicar pronto pago]      [Guardar]          │
  │                                                │
  │  Se aplicará a:  sep ✓  oct ✓ … ← ANTES de     │
  │                                    guardar     │
  └────────────────────────────────────────────────┘
```

### Hallazgo 11.1 — 🔴 El reparto debe verse ANTES de guardar

La petición describe el reparto como consecuencia. Pero si la cajera no ve a qué meses va
el dinero **antes** de confirmar, el primer error se descubre cuando el padre reclama —
y ya hay seis meses movidos.

**Decisión (P1):** el modal muestra el reparto propuesto antes del guardado. Convierte un
error caro en un clic de corrección.

| Estado | Cubierto |
|---|---|
| Cargando (calendario) | ❌ debe bloquear Guardar, no dejar capturar a ciegas |
| Vacío (alumno sin cargos) | ❌ mensaje que diga qué hacer |
| Error (config caído) | ❌ ver 2.3 |
| Éxito | ✅ |
| Parcial (excedente) | ❌ debe decir "quedan $500 a favor" |

**Recomendación:** correr `/plan-design-review` sobre esta pantalla antes de construirla.
Es la que el director va a usar cincuenta veces al día.

---

## Paso 0.5 — Voces duales

**CODEX:** `[codex-unavailable: binary not found]` en ambas fases → **[subagent-only]**.

Las dos voces (CEO y diseño) encontraron cosas que yo no vi, incluidas **cuatro
correcciones a este mismo plan** y **dos bugs vivos en producción**. Verifiqué cada
afirmación de peso contra el árbol antes de aceptarla.

### 🔴🔴 C1 — Me equivoqué: la aritmética de días hábiles NO existe

Escribí que existía en `uniforms` (`PickupWindowBusinessDays`). Verificado:

```
$ grep -rniE "business.?day|día hábil|holiday|weekend" services/ pkg/ --include="*.go"
  (sin resultados fuera del propio nombre de la variable)

$ grep -n "PickupWindowBusinessDays" services/uniforms/internal/domain/settings.go
99: func (s Settings) PickupWindowBusinessDays() int { return s.pickupWindowBusinessDays }
```

**Es un getter de un entero de configuración. No calcula nada.** No hay una sola función
en el repo que sepa qué es un día hábil.

Ese error importa más que ninguno: **acredité como hecho el primitivo del que dependen
los bloques 1, 2, 6 y 7 de la petición.** El calendario no es "una pieza más" — es la
única pieza, y parte de cero.

Y hay una señal que lo confirma: `uniforms` documenta el hueco en
`migrations/uniforms/0003_settings.sql:7-10` — *"the school calendar is deliberately NOT
here… it lives in the config service"* — y `config` tampoco lo tiene. La costura se
diseñó y nunca se llenó.

### 🔴🔴 C2 — "Descuentos maternales" no es un descuento: es un NIVEL que el sistema rechaza

El mejor hallazgo de toda la revisión, y yo lo leí como "beca" (P7 de mis premisas).
En México **maternal es el nivel anterior a kínder**. Y el sistema lo prohíbe:

```sql
-- migrations/students/0001_init.sql:17
level text NOT NULL CHECK (level IN ('kinder','primaria','secundaria','preparatoria'))
```

```
-- apps/web/app/_screens/students/StudentsScreen.tsx:94
"un nivel llamado 'Maternal' no tiene dónde poner uno"
```

**Ese comentario lo escribió alguien que ya sabía que la escuela tiene Maternal.**
O sea: **la escuela no puede dar de alta a sus alumnos de maternal en el sistema.**

Eso es más grave que las 20 peticiones juntas, y venía disfrazado de "descuento".
Si es el nivel, entonces el "descuento maternal" es **cero código**: un renglón de
`tuition_plans` para ese nivel, que ya funciona.

### 🔴 C3 — La fecha de pago no existe en el contrato

`RegisterPaymentRequest` (`finance.proto:296-307`) tiene `charge_id`, `amount_cents`,
`method`, `reference`, `idempotency_key`. **No tiene fecha.** Y
`usecase/finance.go:311` estampa `s.now()`.

Yo puse *"Registro de pago idempotente ✅ existe"* y con eso escondí que **la fecha
exacta —el dato del que dependen TODAS las reglas de cobro que pide el director— no se
puede enviar hoy.** La cajera que se pone al corriente el lunes estampa lunes.

La columna existe en la base (`0001_init.sql:93`) y es inalcanzable.

### 🔴🔴 C4 — DOS BUGS VIVOS en la pantalla de cobro, hoy, en producción

**`apps/web/app/_screens/finance/FinanceScreen.tsx:76`:**
```ts
registerPayment(API_URL, { …, idempotencyKey: "" })
```
Y el propio proto documenta qué significa: *"Empty ⇒ no deduplication (every call posts
a new payment)"*. **Doble clic = doble cobro.** Alcanzable hoy.

Lo peor: `PaymentPanel.tsx` **sí** genera la clave correctamente (`newAttemptKey`). O sea
la protección existe y la pantalla que se usa no la usa.

**`FinanceScreen.tsx:87`:** `onClick={onClose}` en el fondo del modal, sin guardia de
cambios. Un clic fuera **borra un pago a medio capturar**.

Mi Sección 4 decía *"doble clic en Guardar ✅ la idempotencia ya existe"*. Existe en el
backend y en un panel que nadie usa. **Falso donde importa.**

### 🔴 C5 — El tope de 500 filas rompe la facturación con el padrón real

`repository.go:32` — `maxListLimit = 500`, aplicado a `ListOverdue` (`:606`), que
`accrueLateFees` consume (`billing.go:280`).

**400 alumnos × 11 colegiaturas ≈ 4,400 cargos.** Pasando 500 vencidos, la corrida
mensual **deja de acumular recargos en las deudas más nuevas, en silencio**. La escuela
sub-factura y nadie se entera. El mismo tope está en `ListCharges`, así que la "deuda
total" que el director pidió **no va a cuadrar** y lo va a descubrir la primera semana.

**Es un bug de dinero, vivo, y el recargo diario lo empeora.**

### 🔴 C6 — El reparto no cabe en el modelo de pagos actual

`payments.charge_id` es `NOT NULL` con FK (`0001_init.sql:88`), y
`uq_payments_idempotency` es `UNIQUE (tenant_id, idempotency_key)`. **No se pueden
escribir seis renglones de pago para un depósito con una sola clave.**

Además `derivePaidStatus` **veta el sobrepago** (`finance.go:337-339`) y `GetAccount`
pisa los saldos en cero (`:401`). O sea: el pago de $6,000 del ejemplo del director
**hoy ni siquiera se puede guardar**, mucho menos repartir.

Yo lo llamé "reabrir una pieza". Es **un agregado nuevo**: `deposits` +
`payment_allocations`.

Y un acoplamiento que no vi: `students` marca al alumno como inscrito escuchando
`ChargeSettled` filtrado por el prefijo `"enrollment:"`
(`students/internal/adapters/pubsub/consumer.go:63`). **Un repartidor que liquide la
inscripción sin emitir ese evento rompe el estado de admisión en silencio.**

### 🔴 C7 — Los avisos no existen ni siquiera para un alumno

Dije *"comms notifica por alumno; falta grupo y nivel"*. Verificado: `CommsService` tiene
`Health`, `RegisterDevice`, `ListNotifications`, `MarkNotificationRead`. **No hay RPC de
envío.** El contrato lo dice: *"Sending is a side effect of consuming an event, never an
RPC."*

No es un filtro que falta: es una ruta de escritura nueva completa. **Se dimensiona como
módulo, no como campo.**

### 🟡 C8 — Multicontacto: ya hay DOS modelos, no cero

Dije que faltaba porque `students.guardian_name` es un texto. Pero
`migrations/students/0003_guardians.sql` ya tiene **`guardian_students`**: muchos a
muchos, con RLS y `relationship IN ('mother','father','guardian','other')`.

Hay un nombre de tutor en texto libre **y** un grafo de tutores real. El director quiere
una tercera cosa (teléfonos y correos), que ninguno guarda.
**Decisión:** columnas de contacto en `guardian_students`, y retirar `guardian_name`.
**No** crear `student_contacts` — sería el tercer modelo.

### 🟡 C9 — Cité texto que no está en mi propia transcripción

Mi premisa P3 dice que la petición contiene *"cantidad fija configurable"*. Mi §0, que
prometí transcribir sin reinterpretar, dice solo *"ej. 5%"*. **Razoné sobre texto que no
registré.** La contradicción que denuncié puede ser mía.

### 🟡 C10 — El recargo diario NO es un campo barato

Lo clasifiqué entre los "campos nuevos (baratos)". `accrueLateFees` emite **un cargo por
mes vencido**. Diario emitiría ~30 renglones por mes por alumno → 400 × 30 = **12,000
cargos mensuales**, contra un tope de lectura de 500.

**Un recargo diario tiene que ser un monto DERIVADO** sobre el cargo padre, calculado de
`(vencimiento, hoy, calendario, política)`. Eso cambia qué *es* un recargo. Es el segundo
ítem más caro de la lista, no un campo.

### 🔴 D1 — La pantalla no está diseñada para quien la usa 50 veces al día

La voz de diseño la calificó y el promedio es duro:

| Dimensión | Puntaje |
|---|---|
| Jerarquía de información | 5/10 |
| Estados de error | 3/10 |
| Confianza y auditoría | 4/10 |
| Velocidad de uso repetido | **3/10** |
| Intención móvil | **2/10** |

Lo esencial: **"Esperado / Capturado / Diferencia" es un triple de contador, no de
cajera.** Ella no compara números: recibe efectivo y necesita saber *cuánto debe esta
familia hoy*. Y con el reparto, el ejemplo del propio director ($6,000) hace que
"Diferencia" muestre **−$5,000**, que parece un error, no un adelanto.

**Los dos campos no pueden coexistir con el reparto.** Es una contradicción del diseño,
no un detalle.

Además: `PayDrawer` **no es un formulario** (es un árbol de `div`s) — sin Enter para
guardar, sin Escape, sin trampa de foco. Cincuenta veces al día con un padre esperando,
el ratón es el cuello de botella.

### 🔴 D2 — Falta el corte de caja, y es lo que de verdad da confianza

No aparece en la petición ni en mi revisión. Es **el único artefacto** que le permite a
la cajera demostrar que no se quedó con una diferencia, y al director cuadrar contra el
banco. Sin él, el director sigue con su libreta.

Tampoco hay **folio** (cero resultados en el repo para recibo/folio/imprimir), así que
una cajera y un padre no pueden referirse a la misma transacción por teléfono.

### ⚠️ D3 — La voz de diseño DISCREPA de mi Hallazgo 2.3, y tiene razón

Yo decidí: si el calendario no responde, **bloquear la captura**.

Ella responde: *"hay un padre parado con efectivo. Negarse a registrar dinero que ya
entró al cajón es peor que una asignación aproximada — garantiza que la cajera lo anote
en papel, que es justo el fallo que el proyecto existe para eliminar."*

**Acepto la corrección.** Se registra siempre el pago; se difiere solo la **asignación**.
Queda como `pendiente de aplicación`, con folio y recibo, y aparece en una cola que el
director resuelve. **El dinero nunca se bloquea; solo espera la aritmética.**

---

## TABLA DE CONSENSO — VOCES DUALES

```
═══════════════════════════════════════════════════════════════════════
  Dimensión                          CEO      Diseño   Codex  Consenso
  ─────────────────────────────────  ───────  ───────  ─────  ─────────
  1. ¿Premisas válidas?              NO       —        N/A    ⚠️ 4 correcciones al plan
  2. ¿Es el problema correcto?       PARCIAL  NO       N/A    ⚠️ AMBAS: la venta, no la lista
  3. ¿Alcance bien calibrado?        NO       NO       N/A    ⚠️ AMBAS: cortar más
  4. ¿Estimación realista?           NO       —        N/A    ⚠️ 45% de la lista, 15% del esfuerzo
  5. ¿Riesgos cubiertos?             NO       NO       N/A    ⚠️ AMBAS: 2 bugs vivos de dinero
  6. ¿Trayectoria sana?              NO       —        N/A    ⚠️ tenant fijo al arranque
═══════════════════════════════════════════════════════════════════════
Modo: [subagent-only] — sin cobertura cross-model.
```

### Temas que aparecieron en AMBAS voces de forma independiente

**Señal de alta confianza — dos revisores que no se leyeron llegaron a lo mismo:**

1. **El tope de 500 filas es un bug de dinero, vivo.** (CEO §4.1 · diseño lo implica en
   "no va a cuadrar")
2. **La auditoría del descuento manual no es opcional.** El CEO encontró el
   `TODO(finance): persist discount reason/audit`; diseño exige que se **muestre** en
   pantalla o la propiedad es invisible.
3. **La lista no es el problema; la firma sí.** El CEO: *"ocho semanas construyendo contra
   la política de una escuela, entregado a un cliente sin contrato, produce una
   consultoría, no un producto"*. Diseño: *"el Excel es un síntoma, no un requisito"*.
4. **Hay que preguntarle tres cosas al director antes de escribir código.**

---

# FASE 3 — Ingeniería

Voz independiente sobre el plan **ya corregido**. 20 hallazgos; verifiqué los tres que
cambian decisiones. Los tres resultaron ciertos, y **uno mata un diseño que yo propuse**.

### 🔴🔴 E1 — El recargo DERIVADO no se puede cobrar. Mi decisión C10 estaba mal.

`usecase/finance.go:333-341`:
```go
if newPaidCents > charge.Effective().Int64() {
    return "", glerrors.New(glerrors.KindInvalid, "payment exceeds the remaining balance")
}
```

Propuse que el recargo diario fuera un **monto derivado** sobre el cargo padre, para no
emitir 12,000 renglones al mes. Pero `Effective() = amount − discount` **no incluye un
derivado**, así que una familia que paga colegiatura **más** recargo es rechazada como
sobrepago. **El recargo derivado no tiene dónde recibir el dinero.**

Y se lleva cuatro cosas más por delante: el cargo se marca `paid` con el recargo sin
pagar → dispara `ChargeSettled` → **`students` inscribe a un alumno que no terminó de
pagar**; `GetAccount` excluye todos los recargos de la "deuda total"; `ListOverdue` saca
de la cola de cobranza a quien solo debe recargo; y el cajón de pago pre-llena un monto
corto.

**Corrección (acepto la de la voz):** derivar **solo para mostrar**, y **materializar en
el momento en que el dinero se aplica**. El recargo se calcula para la pantalla; cuando
la cajera cobra, se inserta un cargo real de `late_fee` por el monto acumulado, con
`external_ref = latefee:<chargeID>:<fecha>`, justo antes de aplicarle el pago.

Así los cuatro sitios de arriba siguen funcionando sin tocarse, el libro mayor sigue
explicable, y **solo existen renglones donde hubo dinero** — que era el problema que yo
quería evitar. Es mejor que las dos propuestas anteriores.

### 🔴🔴 E2 — Un depósito genera SEIS notificaciones al padre

`comms/service.go:397-405` indexa su ledger por `PaymentRegistered:<payment_id>` y
manda *"Recibimos tu pago de $X"*. Seis asignaciones = seis ids distintos = **seis push,
cada uno por una fracción de lo que el padre entregó**. El ledger no puede deduplicarlos
porque son eventos genuinamente distintos.

**Decisión:** `PaymentRegistered` se re-ancla al **depósito** (un evento, monto total,
sin `charge_id`), y `ChargeSettled` lleva el reparto por cargo. Es un cambio de proto, y
**hoy es barato**: `charge_id` solo lo consume comms, que lo ignora. Después de M11/CFDI
sería caro.

### 🔴🔴 E3 — Reusar `late_fee_cents` multiplica el recargo por 30

La columna está documentada como *"the fixed surcharge added once per completed overdue
month"*, y la demo siembra `lateFeeCents: 15000` ($150/mes). Reinterpretarla como tarifa
diaria convierte $150/mes en **$150/día ≈ $4,500/mes**, sobre 400 alumnos.

**Decisión:** nunca reinterpretar. Columnas nuevas `late_fee_daily_cents` +
`late_fee_mode CHECK (mode IN ('monthly','daily')) DEFAULT 'monthly'`. Los renglones
existentes conservan `monthly`. Así mi "los cargos existentes conservan su regla" pasa de
ser una convención en prosa a **una restricción en los datos**.

### 🔴 E4 — `guardian_students` no puede recibir los contactos: está vacía y no puede llenarse

Verificado: `guardian_user_id uuid NOT NULL` — exige un **usuario de iam con rol
guardian**. La demo siembra **cero** vínculos (solo `guardianName` como texto).

Mi decisión C8 —"columnas de contacto en `guardian_students`"— era correcta en evitar un
tercer modelo y **equivocada en cuál sobrevive**: no hay nada sobre lo que hacer backfill,
y fabricar un usuario de iam por tutor crea una credencial de acceso que la escuela nunca
pidió.

**Corrección:** `guardian_user_id` pasa a **nullable** (un contacto que todavía no es un
login), se agregan `phone`/`email`, y se hace backfill de un renglón por alumno desde
`students.guardian_name`. Así la ruta de auth familiar (`guardian_user_id IS NOT NULL`) y
la lista de contactos **son la misma tabla**, que es lo que yo quería y no logré.

### 🔴 E5 — `ChargeSettled` es un disparador de máquina de estados, no un aviso

`students/consumer.go:54-77` lo consume para mover al alumno pending→accepted→enrolled.
Hoy lo emite `publishChargeSettledIfClosed` desde **un** cargo. Con un depósito que cierra
seis, si el emisor se queda donde está, dispara una vez o ninguna. Y si sale de la
transacción, una caída entre commit y publish deja **un alumno pagado y en trámite para
siempre** — no hay reconciliador.

**Decisión:** un `ChargeSettled` por cargo que cruce a `paid`, y los eventos salen por una
**tabla de outbox transaccional** en la misma transacción. Publicar en modo "mejor
esfuerzo" era defendible para un cargo; no lo es para seis.

### 🔴 E6 — Sin timeout, la decisión D3 es indeployable

`pkg/gcpauth/gcpauth.go:30-41` devuelve un cliente **sin `Timeout`**. El único límite es
el `writeTimeout = 30s` del servidor. Con `config` en `min_instances = 0`, el Guardar de
la cajera **gira hasta 30 segundos** y devuelve un 500 sin mensaje útil.

O sea: la corrección que acepté de la voz de diseño (registrar el dinero y diferir la
aritmética) **no se puede implementar**, porque el código no puede ni distinguir que fue
un timeout. **Decisión:** `context.WithTimeout` de 2-3s específico para la lectura del
calendario, con error distinguible.

### 🟡 E7 — El caché por ciclo no es seguro, y yo mismo nombré al invalidador

Escribí en la Sección 7 "se cachea por ciclo, un calendario cambia dos veces al año", y en
la Sección 4 dejé abierto el caso de **cargar un feriado retroactivo**. Se contradicen.

Con `min_instances=0` y N instancias, no hay señal de invalidación: la escuela agrega un
feriado y finanzas sigue cotizando el monto viejo hasta que la instancia muera. Dos
instancias pueden discrepar a la vez.

**Decisión:** TTL en minutos, no en ciclos, y **estampar la versión del calendario en el
depósito** — así el estado de cuenta puede explicar con qué reglas se calculó un número.
Eso es lo que hace verdadera la promesa de "explicar cada peso", y es casi gratis.

### 🟡 E8 — Dos depósitos simultáneos del mismo alumno se van a bloquear entre sí

Dos cajeras, mismo alumno. Hoy solo hay **un** `FOR UPDATE`, así que el orden nunca
importó. **Decisión:** bloquear los cargos abiertos del alumno en una sola sentencia
ordenada por `due_date, id`.

Y de paso corrige mi Hallazgo 1.1: propuse ordenar el reparto **por `external_ref`**.
Ordenar por texto pone `latefee:` **antes** que `tuition:` — el depósito se consumiría al
revés. `due_date, id` da el orden correcto gratis.

### 🟡 E9 — El saldo a favor es invisible en la pantalla que debe explicarlo

`GetAccount` pisa cada cargo en cero y `ListPayments` hace `JOIN charges ON
payments.charge_id`. Un depósito sin asignar **no aparece en ninguna de las dos**: el
dinero está en la caja y ausente de la pantalla. Justo el saldo a favor de mi Hallazgo 2.2
y el "pendiente de aplicación" de D3.

**Decisión:** `AccountStatement` gana `UnallocatedCents`, y la lista de pagos sale de
`deposits`, no del join con cargos.

### 🟡 E10 — Retirar `guardian_name` son SEIS despliegues, no uno

Es campo de proto (`students.proto:116,130`), está en el dominio, **es obligatorio en el
formulario web** y lo leen tres pantallas. El orden correcto: (1) columnas aditivas,
(2) backfill idempotente, (3) RPC nuevos con doble escritura, (4) el front lee del
origen nuevo, (5) `reserved 8` en el proto — nunca borrar un número de campo,
(6) drop de la columna, en su propia migración.

**Los pasos 2 y 6 en el mismo release es como se pierden 400 nombres de tutor.**

## TABLA DE CONSENSO — INGENIERÍA

```
═══════════════════════════════════════════════════════════════════
  Dimensión                        Claude   Codex   Consenso
  ───────────────────────────────  ───────  ──────  ──────────────
  1. ¿Arquitectura sólida?         NO       N/A     ⚠️ el recargo derivado no cobra
  2. ¿Cobertura de pruebas?        NO       N/A     ⚠️ 6 rutas no son unit-testeables
  3. ¿Riesgos de datos cubiertos?  NO       N/A     ⚠️ reinterpretar columna = ×30
  4. ¿Contratos de evento sanos?   NO       N/A     ⚠️ 6 push por un depósito
  5. ¿Rutas de error manejadas?    NO       N/A     ⚠️ sin timeout, D3 no se puede hacer
  6. ¿Migración segura?            NO       N/A     ⚠️ guardian_students está vacía
═══════════════════════════════════════════════════════════════════
Modo: [subagent-only] en las tres fases. Sin cobertura cross-model.
```

---

## RESPUESTAS DEL DUEÑO — gate cerrado (2026-08-31)

### D1 · "Sí tiene nivel maternal, falta incluirlo"

**Confirmado el bloqueador disfrazado.** La escuela tiene maternal y el sistema no lo
admite. Esto sube al primer lugar de la lista: **no es una petición, es que una parte del
padrón no cabe en el sistema.**

Y no es un `CHECK` de una línea. "Qué es un nivel" vive en **tres lugares incompatibles**:

```
config.levels          TABLA libre        ← aquí sí cabe maternal hoy
students CHECK (…)     enum codificado    ← lo rechaza
uniforms CHECK (…) ×2  enum codificado    ← lo rechaza
+ 2 enums de proto, 2 clientes del gateway, 5 archivos del front
```

**Alcance real: ~11 archivos, 2 servicios, 2 protos.** Un día de trabajo, no una hora.

**Y el "descuento maternal" desaparece como feature**: una vez que el nivel existe, es un
renglón de `tuition_plans` con su propia colegiatura — que ya funciona. **Cero código.**

### D2 · "El pronto pago es una cantidad fija"

Resuelta la contradicción (mi premisa P3, y la duda C9 sobre si yo la había inventado).
**Es un monto fijo, no un porcentaje.** Simplifica el modelo: una columna de centavos, sin
base de cálculo que discutir ni redondeo que definir.

### D3 · "Si paga 6000 no dice nada; captura, marca los meses pagados y baja el total anual"

**Esta respuesta elimina el campo "Diferencia" del diseño**, y con él la contradicción que
la voz de diseño marcó como crítica.

Lo que el director describe no es una conciliación — es un **abono a cuenta**:

```
LO QUE PEDÍA LA ESPECIFICACIÓN        LO QUE EL DIRECTOR QUIERE
┌────────────────────────────┐        ┌────────────────────────────┐
│ Esperado    $ 5,700        │        │  Debe al año   $ 13,000    │
│ Capturado   [$ 6,000]      │        │  Abona         [$  6,000]  │
│ Diferencia  -$   300  ← ?? │        │  ─────────────────────────  │
└────────────────────────────┘        │  sep oct nov dic ene feb ✓ │
                                       │  Queda         $  7,000    │
                                       └────────────────────────────┘
```

**Consecuencias:**
- Se cae `Esperado / Capturado / Diferencia` como triple. Queda **un** número que importa
  (lo que debe) y **un** campo (lo que abona).
- El "monto esperado" no desaparece: sirve para **una** cosa, decidir si aplica pronto
  pago o recargo según la fecha. Pero es un cálculo interno, no una casilla que la cajera
  concilia.
- La pantalla se vuelve más simple que la que se pidió. La voz de diseño puntuó
  3/10 en velocidad de uso repetido; esto es justo lo que la sube.

---

## LISTA DE TAREAS — orden de ejecución

### Bloque 0 · Bugs vivos de dinero (antes de cualquier feature)

| # | Tarea | Dónde | Por qué ahora |
|---|---|---|---|
| T1 | Clave de idempotencia en el cajón de pago | `FinanceScreen.tsx:76` | **Doble cobro alcanzable hoy** |
| T2 | Guardia de cambios en el fondo del modal | `FinanceScreen.tsx:87` | Un clic borra un pago capturado |
| T3 | Paginación en `ListOverdue` y `ListCharges` | `repository.go:32,379,606` | **La escuela se sub-factura** pasando 500 vencidos |

### Bloque 1 · El bloqueador que el director nombró sin saberlo

| # | Tarea | Alcance |
|---|---|---|
| T4 | **Nivel maternal**: migraciones, 2 protos, 2 clientes, 5 pantallas | ~11 archivos, 2 servicios |
| T5 | Colegiatura de maternal | 1 renglón de `tuition_plans` — **cero código** |

### Bloque 2 · El calendario, del que cuelga todo lo demás

| # | Tarea | Notas |
|---|---|---|
| T6 | `paid_at` en `RegisterPaymentRequest` + caso de uso + formulario | 1 día. **Sin esto ninguna regla de cobro funciona** |
| T7 | Calendario escolar en `config` + `pkg/schoolcal` | Parte de cero (E1/C1) |
| T8 | Timeout de 2-3s en la lectura del calendario | Sin él, D3 es indeployable (E6) |
| T9 | Caché con TTL en minutos + versión estampada en el pago | No por ciclo (E7) |

### Bloque 3 · Cobro

| # | Tarea | Notas |
|---|---|---|
| T10 | `late_fee_daily_cents` + `late_fee_mode` — **columnas nuevas** | Reinterpretar la vieja = ×30 (E3) |
| T11 | Pronto pago como **monto fijo** + columna de motivo/auditoría | D2. El `TODO(finance)` de auditoría es prerrequisito, no seguimiento |
| T12 | Recargo: derivar para mostrar, **materializar al aplicar** | Mi diseño derivado no cobraba (E1) |

### Bloque 4 · El abono a cuenta

| # | Tarea | Notas |
|---|---|---|
| T13 | Agregado `deposits` + `payment_allocations` con RLS | `payments.charge_id` es NOT NULL (C6) |
| T14 | Idempotencia sobre `deposits`, no sobre asignaciones | El lock actual desaparece (E-1.3) |
| T15 | Bloqueo determinista por `due_date, id` | Ordenar por `external_ref` reparte al revés (E8) |
| T16 | `PaymentRegistered` re-anclado al depósito | Hoy serían **6 push por un pago** (E2) |
| T17 | Outbox transaccional para `ChargeSettled` | Un fallo deja al alumno "en trámite" para siempre (E5) |
| T18 | `UnallocatedCents` en el estado de cuenta | El saldo a favor hoy es invisible (E9) |
| T19 | `VoidPayment` por asiento, nunca `DELETE` | Un error mueve seis meses (2.1) |

### Bloque 5 · Pantalla

| # | Tarea |
|---|---|
| T20 | Modal: **un** número (debe) + **un** campo (abona) + vista previa del reparto |
| T21 | `<form>` real: Enter guarda, Esc cierra, foco atrapado |
| T22 | Búsqueda de alumno como punto de entrada |
| T23 | Folio + recibo imprimible |
| T24 | **Corte de caja diario** — el artefacto que hace que el director suelte la libreta |

### Bloque 6 · Lo demás, ya sin prisa

T25 multicontacto (`guardian_user_id` nullable + backfill, 6 despliegues) ·
T26 avisos por grupo/nivel (ruta de escritura nueva) · T27 ingresos por concepto ·
T28 export genérico "exportar esta vista".

**Diferidas:** inventario genérico, calendario de actividades, PDF del estado de cuenta,
colegiatura partida por período.

**26 tareas. Estimado con CC: ~4-6 días de trabajo real**, con la firma pedida al terminar
el Bloque 2.

---

## GSTACK REVIEW REPORT

| Review | Trigger | Why | Runs | Status | Findings |
|--------|---------|-----|------|--------|----------|
| CEO Review | `/plan-ceo-review` | Scope & strategy | 1 | issues_open | 14 proposals, 10 accepted, 4 deferred; 6 critical gaps |
| Design Review | `/plan-design-review` | UI/UX gaps | 1 | issues_open | 3/10 speed, 2/10 mobile; 2 live bugs found |
| Eng Review | `/plan-eng-review` | Architecture & tests | 1 | issues_open | 20 issues, 6 critical gaps |
| Codex Review | `/codex review` | Cross-model | 0 | unavailable | binary not installed |

**CROSS-MODEL:** not obtained — Codex absent in all three phases. Confidence rests on
verifying each finding against the tree, not on two models agreeing.

**VERDICT:** NOT ready to build. Three live bugs must land first (empty idempotency key,
500-row cap, backdrop discard). One disguised blocker outranks the whole list: the school
likely cannot enroll its Maternal pupils. Three questions go to the director before any code.

**RESOLVED AT THE GATE (2026-08-31):** D1 → the school HAS a Maternal level and it is
missing from the system, confirming the disguised blocker · D2 → pronto pago is a FIXED
amount · D3 → no Diferencia field; the payment is a credit against the annual debt, which
removes the design contradiction outright.

NO UNRESOLVED DECISIONS

---

## LAS REGLAS DE COBRO, EN PALABRAS DEL DUEÑO (2026-09-01)

### R1 · El recargo corre TODOS los días

> *"si el recargo sí sábados domingos y feriados"*

Recargo = tarifa diaria × **días naturales** desde el vencimiento. `CalendarDaysBetween`,
no `WorkingDaysBetween`.

**Consecuencia que conviene tener presente:** el calendario escolar **no interviene en el
recargo**. Sirve solo para la ventana de pronto pago ("los primeros N días hábiles"), que
sí se cuenta en días hábiles. Fue buena decisión que `pkg/schoolcal` respondiera las dos
preguntas: la respuesta del dueño usa una y la otra sigue haciendo falta.

### R2 · El pronto pago se aplica por cada mes que se paga

> *"si paga 6 meses se le aplica durante los 6 meses la cuota de pronto pago"*

Es un monto fijo **por mes liquidado**, no por transacción. Pagar 6 meses puntual
= 6 × descuento. Encaja sin fricción con el `discount` que ya existe en `charge.go`:
cada cargo mensual lleva el suyo.

### R3 · El recargo se conserva y sigue creciendo hasta que se pague

> *"se conserva. Si el padre después del día 10 paga cada mes se le genera diario el
> recargo hasta que pague. Si paga el siguiente día 10 debe pagar el mes anterior más los
> 10 pesos diarios que se generaron más el mes actual. Si no, le sigue generando recargos"*

La regla completa, con el ejemplo del dueño:

```
día 10 sep    vence la colegiatura de septiembre
día 11 sep    empieza a correr el recargo: $10 por día natural
...
día 10 oct    el padre paga. Debe:
                colegiatura septiembre     $3,200
                recargo 30 días × $10      $  300
                colegiatura octubre        $3,200   (vence hoy, sin recargo)
                ─────────────────────────────────
                                           $6,700

              si NO paga, el recargo de septiembre sigue corriendo
              y el de octubre empieza el día 11
```

**Lo que esto fija en el diseño:**
- El recargo se acumula **por cargo mensual**, no por alumno. Cada mes tiene el suyo.
- Deja de correr cuando **ese** mes queda liquidado, no cuando el alumno paga algo.
- Nunca se perdona por sí solo: perdonarlo es un acto explícito, que es justo lo que hace
  el botón de "aplicar pronto pago" y por eso necesita quedar asentado.

**Esto confirma el diseño al que llegó la revisión de ingeniería:** derivar el recargo para
mostrarlo (crece todos los días, sin escribir 30 renglones al mes) y **materializarlo como
cargo real en el momento en que se le aplica dinero**. Los renglones existen solo donde
hubo movimiento.
