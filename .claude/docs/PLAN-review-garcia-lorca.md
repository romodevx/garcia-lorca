# Plan — Review García Lorca (18 tareas, 5 bloques)

> Estado: **Aprobado 2026-09-30 — Fase 2 en ejecución.** Respuestas en la §7
> (corrección: calificaciones solo enteros).
> Fecha: 2026-09-30 · base: `dev` @ `b2840159`.

## 0. Lo que el código ya tiene (cambia el tamaño de varias tareas)

| Tarea | Ya existe | Falta |
|---|---|---|
| 3 Apellidos | `students.last_name` es UN campo ("paterno y materno"); orden en web por `lastName firstName` | Formateador único; ~25 lugares arman `${first} ${last}`; backend ordena por `id` |
| 6 Notas | `student_notes` (append-only + anular) y `NotesPanel` en la edición | Edición de nota, modal desde el listado, panel en Registrar Pago |
| 7 Estado de cuenta | `/finance/accounts/{id}` + `AccountPanel`; la app arma su propio `toLedger` | Una sola fuente del libro mayor (backend) + PDF |
| 8 Nota de crédito | `deposits` con UN método (cash/card/transfer) | Desglose por método (`deposit_tenders`) + `credit_note` |
| 9 Editar pago | `VoidDeposit` existe (sin pantalla) | Corrección con historial + aviso al padre |
| 10 Egresos | Corte diario (`DailyCashUp`) e ingresos por concepto | Egresos, balance, cierre, reportes PDF/Excel |
| 11 Cumpleaños | `students.birth_date` ya existe | Job diario (no hay scheduler en comms), plantilla |
| 12 Retardos | `late` ya existe en BD, captura web, QR y app | Reportes de asistencia (no existen), revisar app y avisos |
| 13 Asignaturas | **Ya son por grado** (`subjects.grade_level_id`) y la asignación ya limita grupos al grado | Pantalla de alta masiva por nivel/grado; verificación; **sin migración de datos** |
| 14/15 Periodos | `terms` por ciclo+nivel mapeados a meses; calificación **mensual** (`monthly_grades` numeric(5,2)); promedios redondeados half-up a 1 decimal en backend, web y app | Periodos por grado con fechas, captura por periodo, promedio truncado |
| 16 Horarios | Nada | Módulo nuevo |
| 17 Actividades | Nada ("actividades" hoy = `school_events`) | Módulo nuevo con cobro |
| 18 Becas | `services/scholarships` es un esqueleto vacío; finance tiene descuentos por cargo | Módulo nuevo en el cálculo de cobros |

## 1. Decisiones de arquitectura (propuestas; ADR por cada una en su PR)

- **A1 — Becas y actividades viven en `finance`** (ADR 0022). El requisito central es "todo
  cálculo de cobro automático y consistente". Hoy recargo y pronto pago se calculan al leer
  (`domain/collection.go`, `expectedFrom`). Poner la beca en otro servicio obliga a una RPC
  en cada lectura de adeudo y abre dos verdades. El esqueleto `services/scholarships` se
  retira (sin datos, sin tráfico). Actividades: su catálogo, horario descriptivo e inscripciones
  van también en finance porque su única regla de negocio es el cobro.
- **A2 — Horarios en `config`** (ADR 0023). Dueño de asignaturas, grupos y asignaciones; las
  restricciones de choque son locales (EXCLUDE de Postgres con `btree_gist`), no RPC.
- **A3 — Periodos de evaluación en `config`, calificaciones en `grades`**. `grades` consulta
  los periodos por RPC igual que hoy consulta `term_months`.
- **A4 — Libro mayor del alumno en backend** (`finance.GetStatement`): la app de padres y el
  panel consumen el mismo cálculo. Se borra `toLedger` de la app.
- **A5 — Componentes compartidos web** en `apps/web/app/_design/students/`:
  `formatStudentName`/`compareStudents` (T3), `StudentFilters` + `useStudentFilter` (T4),
  `DualPanelPicker` (T17, reusado en T18), `StudentNotesPanel` (T6, 3 lugares).
- **Flags nuevos (default OFF):** `module_expenses` (T10), `module_schedule` (T16),
  `module_activities` (T17), `module_scholarships` (ya registrado; se activa con T18).
- **Permisos nuevos** (migraciones iam, owner por defecto, asignables por el director):
  `users:delete` (owner-only), `payments:edit`, `expenses:read|write`, `schedule:read|write`,
  `activities:read|write`, `scholarships:read|write`.

## 2. Bloque A — Correcciones y base

### T1 · `fix/1-inscripcion-concepto-duplicado`
- **Hipótesis por orden** (el listado no duplica: `ListCharges` no tiene JOIN, keys por id):
  - H1: cargo viejo `enrollment:<ciclo>:<alumno>` (plan de monto único) + cargo nuevo
    `...:inscripcin` tras convertir el plan a conceptos (`tuition-math.ts:233`). Nadie cancela el viejo.
  - H2: renombrar/corregir acento de un concepto cambia su slug (`Slugify` quita no-ASCII:
    "Inscripción" → `inscripcin`) ⇒ referencia nueva ⇒ cargo nuevo; el viejo sigue.
  - H3: cargos $0 residuales del bug R1 anterior a `0d192f79`.
  - H4: cargo manual "Inscripción" sin referencia junto al generado.
- **Paso 1:** reproducir con los datos de dev (SELECT de solo lectura, pregunta Q1) y con un test.
- **Fix de causa raíz (si H1/H2):** la referencia del cargo pasa a usar el **id estable del
  concepto**, no el slug del nombre; el run reconoce las referencias heredadas (monto único y
  slug viejo) como "ya emitido" para ese concepto. Migración que agrega `concept_key` estable
  a cada concepto en el JSONB (sin tocar cargos).
- **No se borra ningún cargo**: el PR incluye la consulta y la lista de duplicados existentes.
- **Revisión cruzada** en el PR: colegiaturas (`tuition:<ciclo>:<alumno>:<YYYY-MM>` — cambiar
  inicio de ciclo re-emite meses; dos pagos en un mes se descartan en silencio) y cuotas
  (`fee:…:<slug>` — mismo problema de renombre). Se corrige lo mismo en `fee:`; lo de
  colegiatura se reporta y se corrige si es la misma clase de defecto.

### T2 · `feature/2-eliminar-usuarios`
- Migración iam: `users.status` agrega `deleted`; columnas `deleted_at`, `deleted_by`.
- `DeleteUser`: baja lógica + revoca refresh tokens (ADR 0005) + revoca invitaciones +
  **deshabilita la cuenta en Identity Platform** (`provisioner` → `Disabled: true`).
- Guardas en el caso de uso (y probadas): no a sí mismo; no el último usuario activo con rol
  owner; permiso `users:delete` owner-only (validado en gateway y en iam).
- Listados excluyen `deleted`. Historial intacto (todas las referencias son UUID + nombre snapshot).
- Web: botón papelera en `UsersPanel`, modal de confirmación (irreversible desde la UI ⇒ diálogo, no undo).

### T3 · `feature/3-apellido-primero`
- `formatStudentName(s)` → "Apellidos Nombre(s)" y `compareStudents` (collation `es`, sin acentos).
  Equivalente Go en `pkg/` para gateway, recibos PDF, boletas y snapshots (`grades/studentsclient/names.go`).
- Backend: `ListStudents` ordena por `last_name, first_name` (índice nuevo).
- Formularios: apellidos antes que nombre (QuickAdd y edición). Ver **Q2** (uno o dos campos de apellido).
- El PR lista cada pantalla tocada (~25 identificadas: alumnos, pagos, finanzas, uniformes,
  asistencia, calificaciones, avisos, recibos, boleta, app de padres).

### T4 · `feature/4-filtros-estudiantes`
- `StudentFilters`: chips Nivel → Grado → Grupo encadenados (solo muestra hijos del elegido,
  con contadores), búsqueda por apellido/nombre con `fold` existente (sin mayúsculas ni acentos,
  mientras se escribe, debounce 150 ms), "Seleccionar todos (N)" sobre lo filtrado.
- Estado de filtros en la URL (volver atrás conserva la vista). 1°B preescolar = 3 clics.

## 3. Bloque B — Pagos y finanzas

### T5 · `feature/5-rango-fechas-concepto`
- Cada concepto/cuota tiene `starts_on` (desde cuándo se puede cobrar/aparece) y `due_on`
  (fecha límite). Validación inicio ≤ límite en dominio y UI.
- **Propuesta de efecto al cambiar fechas:**
  - Cargos **pagados**: nunca se tocan (incluye recargos ya cobrados).
  - Cargos **pendientes o con abono**: se actualiza su `due_date` (mismo patrón que
    `RescheduleUnpaid`). Como recargo y pronto pago se calculan al leer, se re-evalúan solos:
    si la fecha se amplía, el recargo pendiente desaparece; si se acorta, aparece al vencer.
  - Cargos aún no emitidos: se emiten con las fechas nuevas.
  - Un cargo con `starts_on` futuro no aparece en la app de padres hasta esa fecha.

### T6 · `feature/6-notas-alumno`
- Migración students: `student_notes` agrega `updated_at`, `updated_by`, `updated_by_name` +
  tabla `student_note_revisions` (texto anterior, quién, cuándo). Una sola fuente.
- `StudentNotesPanel` compartido en: modal desde el icono del listado, edición del alumno y
  Registrar Pago (panel izquierdo si hay notas; si no, botón "Agregar nota" que amplía el modal).

### T7 · `feature/7-estado-de-cuenta`
- `finance.GetStatement(student)`: movimientos (cargos, pagos por método, recargos, descuentos,
  becas, notas de crédito, correcciones) con fecha, concepto, monto y **saldo acumulado**; totales.
- Icono en el listado → cajón con el estado de cuenta; botón PDF (gateway, mismo motor de recibos).
- La app de padres pasa a consumir el mismo endpoint (se retira su `toLedger`).

### T8 · `feature/8-nota-credito-pago-combinado`
- Migración finance: `deposit_tenders(deposit_id, method, amount_cents, reference)`; método
  nuevo `credit_note`. Los depósitos existentes se reflejan como un tender (backfill 1:1, reversible).
- UI de cobro (colegiatura, conceptos, inventario): lista de métodos; "Nota de crédito" pide
  monto; "+ Agregar método"; barra en vivo: Total · Cubierto · Restante.
- **Propuesta pagos parciales:** se mantienen los abonos que ya existen. El cajero fija
  "Monto a pagar" (≤ adeudo; por defecto el total). La regla estricta es
  **suma de métodos = monto a pagar** (no se registra si no cuadra). Así no se rompe el abono.
- Ticket/PDF: desglose por método. Finanzas y corte: nota de crédito **fuera** del ingreso de
  efectivo, en renglón propio. No se lleva saldo a favor (decisión del colegio).
- Uniformes usa el mismo desglose.

### T9 · `feature/9-editar-pagos`
- Corrección = operación atómica: el depósito original queda **sustituido** (no se borra) y se
  crea el corregido con `corrects_deposit_id`; tabla `deposit_corrections` (antes/después en JSON,
  usuario, fecha, motivo obligatorio).
- Se re-aplica con la **fecha de pago corregida**, así recargos, pronto pago y becas se
  recalculan con la misma lógica de siempre.
- Permiso `payments:edit` (propuesta: solo director por defecto; asignable).
- Evento `PaymentCorrected` → comms → push "Tu pago fue actualizado: $1,000.00".
- Reimpresión del comprobante corregido ("Corrige a P-2026-000123").

### T10 · `feature/10-egresos-cortes`
- Tablas finance: `expense_categories` (configurables), `expenses` (fecha, categoría, monto,
  método, descripción, adjunto), `expense_events` (historial). Cancelar = estado, nunca DELETE.
- Adjunto: mismo almacenamiento que la foto del alumno (se verifica en la tarea).
- Finanzas: ingresos, egresos y balance por método de pago.
- **Propuesta corte de caja:** sí se captura el **efectivo contado**; el cierre guarda la
  foto del día (esperado, contado, diferencia, quién cerró). Si luego cambia un movimiento de
  ese día (p. ej. T9), el corte muestra "modificado después del cierre" y el director puede reabrirlo.
- Reportes diario/semanal/mensual/anual → PDF y Excel (`.xlsx` generado en el gateway).
- Flag `module_expenses`; permisos `expenses:*`.

## 4. Bloque C — Alumnos y asistencias

### T11 · `feature/11-cumpleanos`
- `birth_date` ya existe; se verifica que esté en alta rápida (hoy solo en edición) y se agrega.
- Cloud Scheduler diario (07:00 America/Mexico_City) → `CommsService/RunBirthdays` (mismo patrón
  OIDC que el cobro mensual). Idempotencia con `processed_events`: `birthday:<alumno>:<año>`.
- Plantilla editable por el colegio (`{nombre}`), tabla en comms, con un texto por defecto.
- **29 de febrero:** en años no bisiestos se felicita el **28 de febrero** (sigue en su mes).

### T12 · `feature/12-retardos`
- Retardo ya se captura. Se completa: ajuste manual después del QR (verificar/permitir
  presente→retardo), aviso al padre con texto "retardo", vista en la app con color propio.
- **Reporte de asistencia nuevo** (no existe): por grupo y rango de fechas, conteos
  presente/ausente/retardo/justificada por alumno; PDF/Excel con el mismo motor de T10.

## 5. Bloque D — Estructura académica

### T13 · `feature/13-asignaturas-por-grado`
- Ya es por grado. Se agrega: alta de asignaturas eligiendo nivel → uno o varios grados (crea
  una por grado), vista agrupada por nivel/grado, restricción de grupo al grado en BD
  (hoy solo en código) y pruebas. **No hace falta migrar datos** — se confirma con un conteo.

### T14 · `feature/14-periodos-por-grado`
- Tabla config `evaluation_periods(cycle, grade_level_id, ordinal, name, starts_on, ends_on)`
  con `EXCLUDE USING gist (grade_level_id WITH =, cycle WITH =, daterange(...) WITH &&)`
  ⇒ no se enciman ni por carrera.
- Pantalla por nivel con sus grados; "Copiar a…" otros grados (y "aplicar a todo el nivel").
- Migración de datos: por cada `term` + `term_months` existente se crea el periodo equivalente
  en cada grado del nivel (fechas = primer día del primer mes al último del último mes). No borra nada.

### T15 · `feature/15-calificacion-por-periodo`
- Tabla grades `period_grades(enrollment, subject, period, value numeric(5,2) CHECK entero, captured_by…)`.
  `numeric` exacto; el dominio deja de convertir a centésimas enteras.
- Captura: **solo enteros** (corrección del dueño, §7); un valor con decimales se rechaza en
  UI y backend.
- **Promedios:** cálculo exacto con decimales arbitrarios y **truncado a 2 decimales**
  (8.666… → 8.66; 8.5 → 8.5). Se reemplazan `averageHalfUp` (web), `gradeAverage.ts` (app) y
  `domain/average.go` por una sola regla probada con la misma tabla de casos en Go y TS.
- Se retira la captura mensual (UI y rutas); boleta y app muestran columnas por periodo.
- **Si hay datos en `monthly_grades`: me detengo y pregunto** (ver Q8 para dejar la respuesta lista).

### T16 · `feature/16-horarios`
- Tablas config: `schedule_blocks(cycle, group, subject, teacher, weekday 1–5, starts_at, ends_at)`
  con dos EXCLUDE (profesor/día/rango y grupo/día/rango); `schedule_overrides(date, block,
  kind: substitute|swap|cancel, teacher, reason, by)`; `schedule_events` (historial).
- Bloque solo con asignaturas/asignaciones de T13 (profesor + grupo + asignatura válidos).
- Vistas: semana L–V por nivel/grado/grupo o por profesor; "¿Qué pasa ahora?" (día+hora →
  tarjetas por grupo en rejilla, sin encimados). Cambio por fecha no toca la base.
- Sugerencia de suplentes: profesores sin bloque ni suplencia en ese rango ese día; primero los
  que imparten esa asignatura o ese grado.
- Horario base: se edita libre antes del inicio del ciclo; ya iniciado, los cambios a la base
  también se registran con motivo.
- Flag `module_schedule`; permisos `schedule:read|write`.

## 6. Bloque E — Módulos con cobros

### T17 · `feature/17-actividades`
- Finance: `activities(name, schedule JSONB [{día, inicio, fin}], periodicity, billing_starts_on,
  bill_day, due_day, late_fee_cents, active)`, `activity_price_options(activity, days_per_week |
  null=cuota fija, amount_cents)`, `activity_enrollments(activity, student, option, starts_on, ends_on)`.
- **Periodicidad propuesta:** configurable por actividad: `monthly` (default), `weekly`,
  `per_cycle`. El run de cobro pasa a correr **diario** (idempotente, ya lo es) para cubrir semanal.
- Cargo por periodo con referencia `activity:<actividad>:<alumno>:<periodo>`; aparece en app
  y adeudos al llegar `bill_day`; varias actividades suman.
- **Recargo propuesto:** opcional por actividad (monto fijo, default $0 = sin recargo) y usa
  el mismo cálculo al leer de colegiaturas.
- Alta a mitad de periodo: se cobra el periodo en curso si la inscripción es antes o el día
  límite; si no, desde el siguiente (sin prorrateo).
- Pantalla de inscripción: `DualPanelPicker` (izq. `StudentFilters`; der. inscritos; arrastrar
  varios y flechas → / ←), apellidos primero. Al inscribir se elige el esquema por alumno
  (por defecto el del lote).
- Flag `module_activities`.

### T18 · `feature/18-becas`
- Finance: `scholarships(name, kind: percent|fixed_off|fixed_price, value, targets JSONB
  [enrollment|tuition|concept:<id>|activity:<id>], keep_deadline, active)`,
  `scholarship_students(scholarship, student, from, to)`.
- Aplicación **al calcular** (mismo lugar que recargo/pronto pago), así Registrar Pago, app de
  padres, estado de cuenta y comprobantes salen del mismo cálculo; descuento desglosado.
- **Propuestas:**
  - **Orden:** precio base → descuento de nivel → **beca** → pronto pago (si aplica) →
    si paga **después de la fecha límite de la beca**: se cobra sin beca **y** con el recargo normal.
  - **Varias becas en el mismo concepto:** no se acumulan; se aplica la **más favorable a la
    familia** y la pantalla de asignación avisa del empalme.
  - **Cargos ya generados:** como se calcula al leer, asignar una beca aplica a todos los
    cargos **pendientes**; quitarla los regresa a precio completo. Pagados: nunca cambian.
    Abonados: la beca aplica al saldo restante.
- Selección dinámica de conceptos y actividades existentes; `DualPanelPicker` para alumnos.
- Flag `module_scholarships`; se retira el esqueleto de servicio.

## 7. Preguntas (en el chat del 2026-09-30, con respuesta por defecto)

**Aprobado 2026-09-30.** Todas las respuestas por defecto, con UNA corrección del dueño:

- **Calificaciones: se capturan SOLO ENTEROS** (sin decimales). Sustituye Q9 y la línea de
  T15 "hasta 3 decimales". Lo que sí lleva decimales son los **promedios** que calcula el
  sistema: exactos y truncados a 2 decimales. El valor se sigue guardando en `numeric`
  (entero validado), sin punto flotante.

Respuestas vigentes:
1. Sí, consultas de solo lectura a la base de dev (GCP) para T1.
2. Apellido paterno y materno en dos campos; sin transformar datos (lo actual queda en paterno).
3. Invitar un correo eliminado reactiva la misma cuenta con su historial.
4. Eliminar usuarios: solo owner.
5. Nota de crédito: referencia opcional en texto libre.
6. Editar pagos: owner por defecto, permiso `payments:edit` asignable.
7. Egresos/reportes: owner por defecto, permisos asignables.
8. `monthly_grades` con datos: se conservan intactas como historial de solo lectura, no se convierten (aun así se avisa antes de ejecutar).
9. ~~3 decimales~~ → **solo enteros** (corrección del dueño).
10. El cierre de periodo sigue emitiendo boletas.
11. Suplente: cualquier profesor libre, primero los de esa asignatura/grado; rejilla 7:00–15:00, paso de 5 min, configurable.
12. Fecha límite de la beca = fecha límite de cada cargo.
13. Beca sobre el precio con descuento de nivel.
14. Módulos nuevos se encienden en dev al pasar QA y compuertas.
15. CI sin cuota: verificación local equivalente + GitGuardian + merge `--admin`, anotado en el PR.
16. Push/PR/merge a `dev` autorizados por tarea; `master` no se toca.
17. Build de EAS de la app de padres lo lanza el dueño al final.

## 8. Mecánica de ejecución (Fase 2)
- Por tarea: rama desde `dev` → TDD (test rojo primero) → `/code-review`, `/verify`,
  `/security-review` hasta limpio → `/qa` diff-aware en navegador visible → PR a `dev` →
  esperar CI + GitGuardian → squash merge → borrar rama.
- Dependencias: T3→T4→T17→T18; T13→T16; T14→T15; T8→T9→T10; T7 antes de T18 (desglose de beca).
- App de padres: los cambios llegan a las familias con un nuevo build/OTA de EAS (fuera del merge).

## 9. T19 (añadida 2026-10-02) · Ausencias de profesores en horarios

Aprobada por el dueño con las respuestas por defecto: ausencias de día completo con opción de rango
horario; las registran owner y `schedule:write` (el profesor no); tipo y nota visibles solo para
`schedule:write` (los demás ven "Ausente"); sin aviso a padres; sin comprobante adjunto. Sin asistencia
del profesor. Efectos: clases del ausente "Sin cubrir" en calendario y "Ahora", vista "Clases por
cubrir", sugerencias y suplencias excluyen ausentes, aviso al guardar con N clases afectadas; además
validar días inhábiles al registrar cambios de un día (pendiente de T16).
