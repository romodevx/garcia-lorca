# PLAN — Command Palette (buscador con superpoderes)

> Cross-cutting UX infra (NO un módulo de negocio): centro de **navegación + ejecución**
> con el mínimo de clics. Memoria: [[command-palette]] · [[ux-minimum-clicks]].
> Estado: **PLAN — pendiente de checkpoint humano.**

## Objetivo
Cmd+K abre un palette que entiende lo que el usuario quiere y lo **lleva directo** o lo
**ejecuta**: `crear alumno` → form; `Juan Pérez` → perfil; `tomar asistencia` → asistencia
de hoy; `pago Juan` → cobrar. Reemplaza el buscador cosmético del TopBar.

## Decisiones de arquitectura (revisadas — `/plan-eng-review` 2026-06-17; detalle en [[command-palette]])
- **Shell sobre `cmdk`** (headless estándar: teclado/foco/ARIA/fuzzy). No hand-roll.
- **Dos capas:** (A) comandos (módulos+crear+rápidas) con el **registro ensamblado server-side
  (RSC) por usuario+flags+rol** — el fuzzy corre en cliente sobre lo ya autorizado (no se filtra
  el mapa de acciones en el bundle); (B) registros vía un **índice de búsqueda propio** (Postgres
  FTS/`pg_trgm`) alimentado por **eventos** — **NO federación por-servicio, los módulos no exponen
  `Search` RPC**.
- **Contrato `ModuleCommands`:** estático (nav/create/quick) **firme**; `records` **PROVISIONAL**
  hasta que students/M03 lo valide y se extraiga. No se congela a ciegas.
- **Matching:** título i18n + alias chico + fuzzy con **folding de acentos**; sin listas grandes
  de sinónimos. **i18n** ([[i18n-bilingual]]) es+en. **Flags** (FLAGS.md): OFF → no se envía el comando.
- **Seguridad:** acciones **navegan a form prellenado, nunca mutan**; búsqueda de registros
  filtrada por IAM+RLS server-side (sin fuga por conteo/timing); rate-limit; historial guarda
  solo IDs de comando, nunca datos de registros. **Iconos/colores** del dashboard.

## PM — alcance MoSCoW
**Must (MVP del palette — infra cross-cutting, construible YA, sin depender de módulos):**
- Shell del palette: Cmd+K, navegación por teclado (↑↓/Enter/Esc), resultados con iconos+colores,
  agrupados, máx ~8–10, mobile responsive, a11y (roles/aria, focus trap).
- Registro estático + **el contrato `ModuleCommands`**.
- Capa A: módulos + acciones de crear + acciones rápidas; fuzzy local; keywords i18n es+en; flag-aware.
- Historial de recientes (localStorage).
- Sustituye el buscador del TopBar.
- *Entrega el valor central (navegar + ejecutar, min clics) aun antes de que existan registros reales.*

**Should:**
- Capa B vía **índice de búsqueda propio** (Postgres FTS/`pg_trgm`), empezando por **estudiantes**
  (una sola fuente, M03), filtrado server-side por IAM+RLS. Quick-actions de registro ("Juan → adeudos…").

**Could:**
- Más tipos al índice conforme aterrizan módulos (cada uno emite eventos: finanzas, asistencia…).
- Parsing de query ("Juan deuda", "$128,300", "adeudo enero") e info-queries ("total estudiantes",
  "ingresos junio"). Comandos contextuales (el palette sabe en qué pantalla estás).

**Won't (ahora):** búsqueda semántica/NL con IA, voz. (Futuro.)

## Eng-Manager — secuencia, dependencias y estimación
- **No es un módulo vertical**; es infra como i18n. La **Capa A** depende solo del frontend +
  i18n (ya en master) → **construible ahora**, idealmente junto con/después del spine (M01→M03).
- La **Capa B** depende del **gateway (M02)** + un **índice de búsqueda propio** alimentado por
  eventos. La parte **estática** del contrato se fija ahora; la de **`records` se EXTRAE** de
  students/M03 (no se congela a ciegas → evita el churn que la voz externa marcó como el mayor riesgo).

| Fase | Entregable | Size | IED | Depende de |
|------|-----------|------|-----|-----------|
| CP-1 | Shell `cmdk` + registro server-assembled + Capa A estática (Must) | **M** | 4–7 ⚠️ | i18n (listo), `cmdk` |
| CP-2 | Índice de búsqueda + records de **students** (una fuente) → extraer contrato (Should) | **M** | 3–5 | M02, M03 |
| CP-n | Cada módulo emite eventos al índice + registra comandos (Could, recurrente) | **S** c/u | 1–2 | el módulo respectivo |

> Soft top en CP-1: a11y (cmdk lo baja), fuzzy con folding de acentos, binding i18n, y el
> **ensamblado server-side del registro** (por flags/rol). CP-n se **absorbe en el DoD de cada módulo**.

## DoD del Command Palette (se añade al DoD de cada módulo)
Un módulo NO está terminado hasta que registró en el palette: su `nav`, sus `create`, sus `quick`
(que **navegan a forms prellenados, nunca mutan**), y (si tiene registros) **emite sus eventos al
índice** + define su `records.toResult`. Tests: alias es+en (con acentos plegados) resuelve el
comando; flag OFF → el comando no se ensambla server-side; búsqueda de registros aislada por RLS
(un rol no ve lo que no le toca, ni por conteo/timing).

---

## Catálogo de búsquedas por módulo (semilla — del usuario; crece con cada módulo)
Fuente de módulos/iconos/colores: `apps/web/app/_design/data.ts`.

**1. Estudiantes** (👤 forest, `module_students`) — nav: `estudiantes`. crear: `crear alumno`,
`nuevo estudiante`, `agregar alumno`. registros: `Juan Pérez`→perfil, `4to A`→grupo, `maria@…`→email,
recientes. quick de registro: estado, calificaciones.

**2. Finanzas y Adeudos** (📦 orange, `module_finance`) — nav: `finanzas`, `adeudos`. crear: `crear pago`,
`nuevo adeudo`, `crear cobranza`, `generar recibo`. registros: `Juan deuda`, `$128,300`, `adeudo enero`, `morosos`.

**3. Asistencia QR** (lima, `module_attendance`) — nav: `asistencia`, `QR`. rápidas: `tomar asistencia`→hoy,
`asistencia 4to A`, `marcar ausencia`, `reporte asistencia`. registros: `Juan ausente`, `asistencia junio`, `presentes hoy`.

**4. Calificaciones** (📋 yellow, `module_grades`) — nav: `calificaciones`, `notas`. crear: `crear evaluación`,
`cargar notas`, `nueva calificación`, `bloque nuevo`. registros: `Juan matemáticas`, `4to A promedio`, `pendientes`, `reprobados`.

**5. Avisos** (📢 orange, `module_comms`) — nav: `avisos`, `notificaciones`. crear: `crear aviso`,
`enviar comunicado`, `notificar a padres`, `avisar grupo`. registros: `aviso festival`, `no leídos`, `avisos junio`.

**6. Becas** (🎓 purple, `module_scholarships`) — nav: `becas`. crear: `crear beca`, `asignar beca`,
`nueva solicitud`. registros: `Juan beca`, `becas activas`, `becas julio`.

**7. Documentos** (📄 forest, `module_docs`) — nav: `documentos`. crear: `crear documento`,
`generar constancia`, `kardex`, `certificado`. registros: `constancia Juan`, `kardex 4to A`, `pendientes`.

**8. Uniformes** (👕 purple, `module_uniforms`) — nav: `uniformes`. crear: `nuevo pedido uniforme`,
`registrar talla`, `crear solicitud`. registros: `Juan uniforme`, `pedidos pendientes`, `uniformes junio`.

**9. Facturación** (yellow, `module_billing`, **OFF/post-MVP**) — nav: `facturación`, `facturas`. crear:
`crear factura`, `generar CFDI`, `facturar pago`. registros: `factura Juan`, `facturas junio`, `CFDIs pendientes`.
*(Sus comandos solo aparecen cuando el flag se encienda; hoy reservado.)*

**10. Reportes** (yellow, `module_reports`) — nav: `reportes`. rápidas: `reporte asistencia/finanzas/calificaciones`,
`indicadores`, `estadísticas`. info: `total estudiantes`, `ingresos junio`, `asistencia promedio`.

**11. Configuración** (⚙️ neutral, `module_config`) — nav: `configuración`, `ajustes`. acciones: `permisos`,
`usuarios`, `crear usuario`, `roles`, `backup`, `centro`.

## Tests (objetivo del palette — se afinan en CP-1/CP-2)
```
[CP-1] registro/matching
  ├── alias es+en resuelve el comando (crear alumno / new student)   [TEST]
  ├── folding de acentos (maria→María, nino→niño)                    [TEST]
  ├── flag OFF → el comando NO se ensambla server-side               [TEST]
  ├── teclado: ↑↓/Enter/Esc (vía cmdk)                               [TEST]
  ├── historial guarda solo IDs de comando (no datos de registro)    [TEST]
  └── estados vacío / sin-resultados / error                         [TEST]
[CP-2] índice de registros
  ├── students aparece por nombre/grupo/email                        [TEST]
  ├── RLS: rol A no ve registros de rol B (ni por conteo/timing)     [TEST → integración]
  ├── acción de registro NAVEGA a form prellenado (no muta)          [TEST]
  └── rate-limit de búsqueda                                         [TEST]
```

## NOT in scope (deferidos, con razón)
- **Federación por-servicio / `Search` RPC por servicio** — descartado a favor del índice propio.
- **Mutaciones desde el palette** — siempre navega a form; nunca ejecuta.
- **Parsing de query e info-queries** ("Juan deuda", "ingresos junio") — Could, post-CP-2.
- **Búsqueda semántica/NL/voz** — futuro.
- **Congelar `records` del contrato ahora** — se extrae de M03.

## GSTACK REVIEW REPORT

| Review | Trigger | Why | Runs | Status | Findings |
|--------|---------|-----|------|--------|----------|
| CEO Review | `/plan-ceo-review` | Scope & strategy | 0 | — | — |
| Codex Review | `/codex review` | Independent 2nd opinion | 0 | — | — |
| Eng Review | `/plan-eng-review` | Architecture & tests (required) | 1 | issues_open | 5 arch + outside-voice; folded or surfaced |
| Design Review | `/plan-design-review` | UI/UX gaps | 0 | — | — |
| DX Review | `/plan-devex-review` | Developer experience gaps | 0 | — | — |

- **CODEX:** not installed — outside voice ran via Claude subagent (independent context).
- **CROSS-MODEL:** convergencia fuerte en (1) no congelar el contrato → records PROVISIONAL extraído
  de M03; (2) no federación → **índice propio** (la voz externa lo marcó como el mayor riesgo; el
  usuario lo eligió como dirección); (3) shell sobre `cmdk`; (4) acciones navegan, nunca mutan.
  Folds adicionales (seguridad/correctitud): registro ensamblado server-side por flag/rol (no leak de
  bundle), historial sin datos de registro, folding de acentos, matching por título+alias (no
  sinónimos), rate-limit, core agnóstico de plataforma, keying canónico por flag, estados vacío/error.
- **VERDICT:** ENG reviewed — listo para implementar **CP-1** (Capa A estática). CP-2 espera M03.
  No flags se encienden; es infra cross-cutting (sin gate de módulo).

NO UNRESOLVED DECISIONS
