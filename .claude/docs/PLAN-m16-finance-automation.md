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
