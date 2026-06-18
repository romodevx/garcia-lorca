# PLAN — Command Palette (buscador con superpoderes)

> Cross-cutting UX infra (NO un módulo de negocio): centro de **navegación + ejecución**
> con el mínimo de clics. Memoria: [[command-palette]] · [[ux-minimum-clicks]].
> Estado: **PLAN — pendiente de checkpoint humano.**

## Objetivo
Cmd+K abre un palette que entiende lo que el usuario quiere y lo **lleva directo** o lo
**ejecuta**: `crear alumno` → form; `Juan Pérez` → perfil; `tomar asistencia` → asistencia
de hoy; `pago Juan` → cobrar. Reemplaza el buscador cosmético del TopBar.

## Decisiones de arquitectura (resumen; detalle en la memoria)
- **Dos capas:** (A) comandos estáticos client-side instantáneos (módulos + crear + rápidas),
  fuzzy local; (B) registros server-backed vía gateway `/search` (debounced, permission/RLS-scoped).
- **Contrato `ModuleCommands`** que cada módulo aporta (nav + create + quick + records?) y un
  RPC `Search` por servicio dueño de registros. Es el mecanismo que mantiene el palette
  completo conforme crecen los módulos.
- **i18n** ([[i18n-bilingual]]): títulos + **sinónimos de búsqueda en es+en**. **Flags**
  (FLAGS.md): módulo OFF → 0 comandos (server autoritativo). **Iconos/colores** del dashboard.

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
- Capa B vía gateway `/search`, empezando por **estudiantes** (primer slice vertical, M03),
  scoped por permisos/RLS. Quick-actions de registro ("Juan → adeudos/calificaciones/documentos").

**Could:**
- Federación a más tipos conforme aterrizan módulos (finanzas, asistencia, calificaciones…).
- Parsing de query ("Juan deuda", "$128,300", "adeudo enero") e info-queries ("total estudiantes",
  "ingresos junio"). Comandos contextuales (el palette sabe en qué pantalla estás).

**Won't (ahora):** búsqueda semántica/NL con IA, voz. (Futuro.)

## Eng-Manager — secuencia, dependencias y estimación
- **No es un módulo vertical**; es infra como i18n. La **Capa A** depende solo del frontend +
  i18n (ya en master) → **construible ahora**, idealmente junto con/después del spine (M01→M03).
- La **Capa B** depende del **gateway (M02)** + que cada servicio dueño exponga `Search`. Aterriza
  incremental: el **contrato se define ahora** y cada módulo añade su `records` + RPC `Search` al
  construirse → **cero retrabajo**.

| Fase | Entregable | Size | IED | Depende de |
|------|-----------|------|-----|-----------|
| CP-1 | Shell + registro + contrato + Capa A (Must) | **M** | 4–6 ⚠️ | i18n (listo), patrón de slices |
| CP-2 | Gateway `/search` (skeleton federador) + records de **students** (Should) | **M** | 3–5 | M02, M03 |
| CP-n | Wiring de records por módulo (Could, recurrente) | **S** c/u | 1–2 | el módulo respectivo |

> Soft top en CP-1 por el pulido de accesibilidad y el motor fuzzy + el binding de keywords i18n.
> CP-n se **absorbe dentro del build de cada módulo** (parte de su DoD), no es trabajo suelto.

## DoD del Command Palette (se añade al DoD de cada módulo)
Un módulo NO está terminado hasta que registró en el palette: su `nav`, sus `create`, sus `quick`
relevantes, y (si tiene registros) su `records` + el RPC `Search` scoped por permisos. Tests:
keyword es+en resuelve el comando; flag OFF oculta y el server responde NotFound.

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

## GSTACK REVIEW REPORT
_(Pendiente: este PLAN aún no pasó `/plan-eng-review`. Correr antes de implementar CP-1.)_
