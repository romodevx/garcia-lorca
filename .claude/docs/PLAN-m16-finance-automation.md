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
