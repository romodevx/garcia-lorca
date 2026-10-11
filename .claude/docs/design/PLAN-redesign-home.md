# Plan — Rediseño del Inicio (opción C) + estilo "Captions adaptado" en toda la app

Estado: **v2 aprobado por el dueño el 2026-10-11; F1 construido (PR 1)** · 2026-10-10 · rama `feature/redesign-home` (desde `dev`)
Vista previa aprobada: https://claude.ai/artifact/W1gxk8oLEgu8dCs9iusedw (dirección C + estilo Captions adaptado)
Revisiones: `reviews/PLAN-redesign-home.design-review.md` (16 hallazgos) · `reviews/PLAN-redesign-home.eng-review.md` (16 hallazgos). Cada enmienda de abajo cita su hallazgo como **[D#]** (diseño) o **[E#]** (ingeniería).

## 0. Decisiones del dueño

| # | Fecha | Decisión |
|---|---|---|
| D1 | 10-oct | Dirección **C**: módulos agrupados por área, tarjetas, accesos directos al pasar el cursor. |
| D2 | 10-oct | Estilo **Captions adaptado** (grano, barra flotante esmerilada, bordes finos, halo al pasar el cursor, íconos tintados por área, serif solo en el saludo). Sin cian, sin titulares de 48px, sin espacios de 160px. |
| D3 | 10-oct | El estilo se aplica a **toda la app de una vez**, con un solo flag. |
| D4 | 10-oct | **Arreglar el intent** primero, en su propio PR (cambio de comportamiento). |
| D5 | 10-oct | En táctil, los accesos se abren con un **botón ⋯ de 44px**; tocar la tarjeta abre el módulo. |
| D6 | 10-oct | Tablet y escritorio, todos los tamaños y zooms; teléfono fuera de alcance (§4.1). |
| D7 | 10-oct | Monitores anchos: el inicio se **acota a 1320px** centrado [D7]. |
| D8 | 10-oct | "Generar colegiaturas" **se quita** (no vive en Finanzas) [E3]. |
| D9 | 10-oct | Zoom: el inicio aguanta todos los zooms (bloqueante); los otros 10 módulos son bloqueantes hasta 200%, y de 300 a 400% se corren **en modo informe** y lo que falle va al backlog [E8]. |
| D10 | 10-oct | El peek muestra **solo accesos que llevan a algo distinto** de la tarjeta. Un módulo sin accesos así no tiene ⋯ [E3]. |

Decisiones menores tomadas con la recomendación de las revisiones (el dueño puede revertirlas): Reportes → Sistema, Facturación → Dinero, Documentos → Alumnos y familias [D8/E11] · "Asignar beca" → **"Nueva beca"** [E3] · los encabezados de los 17 módulos también pasan a ícono tintado, porque D3 es "toda la app" [D3, O3] · el resplandor lima del FAB y del botón primario se queda, por ser "lo primario" [O4] · un solo flag; apagarlo en dev es un deploy del gateway (~6 min), no un clic [E14].

## 1. Fases y PRs

| Fase | PR | Qué | Comportamiento | Flag |
|---|---|---|---|---|
| F0 | — | Línea base: capturas "antes", tests de caracterización, inventario de sombras por grep | no cambia | — |
| F1 | PR 1 | Intents: traspaso de un solo uso, 9 accesos, filtro por permiso de escritura, cobro movido a Registro de pagos | **cambia** (D4) | ninguno (arreglo de defecto) |
| F2a | PR 2 | Refactor sin cambio visual: estilos en línea → clases; `--topbar-h`; `ModuleHeaderIcon` | no cambia | — |
| F2b | PR 2 | Capa de estilo bajo `html[data-look="refresh"]` | no cambia | `module_ui_refresh` |
| F3 | PR 2 | Inicio C | no cambia (usa F1) | `module_ui_refresh` |
| F4 | PR 2 | Matriz responsive/zoom, `/qa` visible, `/design-review`, `/review-animations` | — | ON y OFF |

Con el flag OFF, la app queda **idéntica a hoy** (F2a lo demuestra con estilos calculados idénticos).

## 2. Inventario de comportamiento del inicio (contrato)

Fuente: `apps/web/app/_screens/Dashboard.tsx`, `Dashboard.test.tsx` (13), `Dashboard.regression-1.test.tsx` (4).

| # | Comportamiento hoy | En C (flag ON) |
|---|---|---|
| B1 | Saludo por hora + nombre real | Igual, en serif. El emoji 🌱 pasa a `<Icon name="sprout" aria-hidden>` [D14] |
| B2 | Fecha `es-MX` capitalizada + "· Ciclo X" solo si se conoce | Igual |
| B3 | "Nuevo aviso" solo con `comms:read` → `announcements` | **Igual**: esa pantalla ya es el formulario [E12]. Extra: enfoca el título |
| B4 | KPI Alumnos con `students:read` → `students`; excluye rechazados/bajas | Igual + estados de §3.4 |
| B5 | KPIs Por cobrar / Cobrado con `finance:read` → `finance` | Igual + estados de §3.4 |
| B6 | Sin lecturas antes de resolver permisos; el maestro no lee alumnos/ciclos/cargos | Igual |
| B7 | Módulos visibles = `isModuleVisible` | Igual; cambia solo el acomodo |
| B8 | Esqueleto mientras `permissions === undefined` | Esqueleto con forma de áreas |
| B9 | `permissionsFailed` → alerta + Reintentar | Igual |
| B10 | Cero módulos → "No tienes módulos asignados" | Igual |
| B11 | Clic en tarjeta → `onOpen(id)` | Igual |
| B12 | El maestro ve exactamente Asistencia, Calificaciones y Horarios | Igual, sin títulos de área (≤4) |

Los tests existentes siguen verdes con el flag ON y OFF; los que dependan del orden de la cuadrícula se parametrizan.

## 3. Diseño

### 3.1 Áreas y semántica

`group` es **obligatorio** en `ModuleDef` (`_design/data.ts`), así el compilador obliga a asignarlo a los módulos nuevos [D8/E11].

| Área | Color | Módulos, por uso diario |
|---|---|---|
| Alumnos y familias | `--forest` | Estudiantes, Avisos, Actividades, (Documentos) |
| Académico | `--purple` | Asistencia, Calificaciones, Horarios, Docentes |
| Dinero | `--orange` | Registro de pagos, Finanzas, Becas, Inventario, Egresos, (Facturación) |
| Sistema | neutro | Configuración, Administración, (Reportes) — sección compacta al final |

- Un área sin módulos visibles no se pinta. Con ≤4 módulos: un `h2` "Tus módulos" y sin títulos de área.
- Semántica [D11]: `h1` = saludo. Cada área es un `<section aria-labelledby>` con su `h2` y una `<ul>` de tarjetas (`<li>`). Se quitan el `h2` "Módulos" y "Todo el sistema, a la vista". Las flechas "→" llevan `aria-hidden`.
- Descripciones con verbos (copy de la vista previa, pasado por `humanizer`), con **llaves i18n ES/EN** `home.group.*` y `home.desc.*`. Test hermano `home.i18n.test.ts`: cada área y cada descripción visible tiene llave no vacía en los dos idiomas [E16].
- Los colores por área solo aplican con el flag ON; ⌘K conserva `module.color`.

### 3.2 Tarjeta y peek

```
<li class="home-card" data-group="dinero">
  <button class="home-card__main" aria-describedby="desc-x">   ← cubre la tarjeta; onOpen(id)
  <p id="desc-x">…</p>
  <button class="home-card__more" aria-expanded aria-controls="peek-x" aria-label="Accesos de Finanzas">⋯</button>
  <ul id="peek-x" class="home-card__peek" hidden> <li><button>Ver vencidos <span aria-hidden>→</span></button></li> </ul>
</li>
```

- **Solo CSS para hover, foco y peek** (`:hover`, `:focus-within`, `@media (hover:hover)`). Nada de `useState` para el hover, porque un estilo en línea no puede usar media queries [D1].
- **Ratón** (`(hover:hover) and (pointer:fine)`): el peek aparece al pasar el cursor, con 120 ms de retardo al entrar y 80 ms al salir. Se superpone **solo sobre la descripción**: el título y el ícono siguen visibles [D6].
- **Táctil o híbrido** (`(any-pointer:coarse)`): el ⋯ de 44×44 está visible. Abre el peek **dentro del flujo** (la tarjeta crece). Tocar fuera o Esc lo cierra. Solo un peek abierto a la vez.
- **Teclado** [D5]: el peek cerrado es `hidden`, fuera del Tab y del árbol de accesibilidad. Con puntero fino, el ⋯ aparece con `:focus-within`. Recorrido: Tab a la tarjeta → Tab al ⋯ → Enter abre → Tab o flechas entre accesos → Esc cierra y devuelve el foco al ⋯. Son **2 paradas por tarjeta**. Cumple WCAG 1.4.13.
- Las tarjetas sin accesos (D10) no tienen ⋯ ni peek.
- Accesos: solo los de la tabla §5, y solo si el usuario tiene el permiso `requires` [E4].
- Movimiento [D15]: una sola entrada por sección (`fade-up`, 60 ms entre secciones, sin escalonar por tarjeta). Al pasar el cursor, `translateY(-2px)`. El peek usa `transform` y `opacity` en 200 ms con `--ease`. Con `prefers-reduced-motion`: sin entrada y sin desplazamiento.

### 3.3 Capa de estilo global

**Cómo se enciende [E7]:** un `<LookAttribute/>` dentro de `AppFlagsProvider` (vive en `layout.tsx` y sobrevive a la navegación) sincroniza `html[data-look="refresh"]` con el flag. Guarda el último valor en `localStorage` (`gl-look`), y el script previo al pintado que ya aplica `gl-theme` lo aplica antes del primer pintado, sin parpadeo. Cubre también `/activar` y `/comprobantes`. Los comprobantes impresos quedan fuera con `@media print`. La piel y la variante del inicio leen el flag cada una en un solo sitio, para poder partirlo en dos flags con un cambio de una línea [E-flag].

**F2a, primero y sin cambio visual [D1, E6]:**
- `TopBar` pasa sus estilos en línea a `.topbar` en `globals.css`, y el alto se publica como `--topbar-h`. `--setup-topbar-h` pasa a ser `var(--topbar-h)`. La barra de pasos de Configuración (ON en dev) sigue pegada en su lugar.
- La tarjeta de módulo y la tarjeta KPI pasan a clases.
- `ModuleHeaderIcon`: un solo componente para los 17 cuadros de ícono de encabezado (hoy son 18 copias del mismo `div` de 56px con resplandor) [D3, E10].
- Inventario de sombras **por grep**, no a mano, en tres familias [E10]:
  - (a) halo de ícono → `--shadow-icon`;
  - (b) elevación (FAB, popover, botón primario) → `--shadow-pop` / `--shadow-fab`;
  - (c) franjas `inset` y anillos de estado → se quedan.

  Un test de grep en Vitest falla si aparece un `boxShadow` con px fuera de la familia (c).
- Test de caracterización: con OFF, `getComputedStyle` de `.topbar`, la tarjeta de módulo y la tarjeta KPI es idéntico al de hoy.

**F2b, tokens bajo `html[data-look="refresh"]`, en claro y oscuro:**

| Pieza | Hoy | Con el flag |
|---|---|---|
| Fondo | `#F5F7F5` + degradado | blanco papel + grano SVG (0.05–0.07) + lavado verde arriba |
| `--shadow-card` | sombra doble | `none` + borde `--hair` (token nuevo) [D13] |
| Hover de tarjeta | `--shadow-hover` | `--halo` (token propio, lima, desplazamiento 14px; oscuro al .10, nunca con desplazamiento 0) + borde del **color del área** al 45% [D4] |
| `--r-lg` (tarjetas) | 22px | 16px |
| Barra superior | pegada, 78px | cápsula flotante a 12px de los bordes, radio 28px, desenfoque 28px + saturación, borde `--hair` |
| Saludo | Poppins 800 | Libre Baskerville 400 (`next/font`, solo el `h1`) |
| Íconos (inicio y 17 encabezados) | cuadro sólido + resplandor | cuadro tintado al 13% + ícono del color del área |
| Anillo de foco | lima al 55% (~1.6:1) | `2px solid var(--forest)` con desplazamiento 2px (oscuro: `--lime`), ≥3:1 [D10] |
| Colores forzados | — | `@media (forced-colors: active)`: borde `CanvasText` en tarjetas [D13] |

- La regla del lima queda escrita [D4]: lima = lo primario, lo seleccionado y el halo de hover. Nada más.
- Si en el `/qa` con el proyector del colegio las tarjetas se pierden, `--hair` sube a `--border-strong` [D13].
- `DESIGN-SYSTEM.md` recibe una sección "Look refresh" con los tokens nuevos, el cuadro tintado, el anillo de foco y la regla del lima. También se corrige ahí el valor de `--r-lg` [D16].

### 3.4 Estados de los KPI [D2, D9]

La vista nueva **deja de tragarse los errores** (con OFF, B4 y B5 no cambian).

| Estado | Lo que se ve |
|---|---|
| Cargando | Etiqueta + renglón esqueleto. Nunca un "0" |
| Error | "No se pudo cargar" + **Reintentar** dentro de la tarjeta (el clic en la tarjeta sigue abriendo el módulo) |
| Sin ciclo activo | "Sin ciclo activo" + "Ir a Configuración →" (solo con `config:read`) |
| Cero con lectura exitosa | Alumnos: "Aún no hay alumnos · Nuevo alumno →" (intent `students:new` si se tiene el permiso; si no, "Ir a Estudiantes →") · Por cobrar: "Aún no hay cargos este ciclo · Ir a Finanzas →" · Cobrado: "Sin cobros todavía · Ir a Registro de pagos →" (si ve `payments`; si no, Finanzas) |
| Con valor | El número de hoy |

El destino del clic cambia **solo con el flag ON y en cero**.

## 4. Responsive y zoom

### 4.1 "Todos los zooms" sin teléfono

El zoom reduce el ancho CSS: 1280 px al 400% son 320 px. WCAG 2.2 · 1.4.10 pide reflujo a 320 px, así que el **inicio** aguanta 320 px aunque nadie lo use en teléfono. Lo que queda fuera es la navegación pensada para teléfono. Para los otros módulos aplica D9.

### 4.2 Reglas

- Ancho del inicio: `max-inline-size: 1320px`, centrado, en todo ancho (D7). Cuadrícula `repeat(auto-fit, minmax(min(100%, 232px), 1fr))`.
- La tarjeta reserva `min-height` en `rem` (título + 3 renglones de acceso), así escala con el zoom. El peek por hover tiene `max-height: 100%`, con `overflow:auto` como último recurso. El peek por ⋯ va dentro del flujo [D6].
- Barra superior: `sticky` solo con `min-height: 560px`. Más bajo pasa a estática. `html { scroll-padding-top: calc(var(--topbar-h) + 16px) }` para que la barra no tape el foco (WCAG 2.4.11) [D12].
- Por debajo de **768 px** (el patrón que ya existe, `globals.css:177`) se esconden EN, tema y correo, y la búsqueda se vuelve ícono [E6].
- `backdrop-filter` con `@supports not` → fondo opaco. El grano no usa `background-attachment: fixed`.
- Texto en `rem`/`clamp`. WCAG 1.4.4 y 1.4.12 cubiertos.
- `narrow.mobile.spec.ts` (Inventario de pie) sigue verde con el flag ON.

### 4.3 Matriz de prueba (Playwright)

El flag se fuerza con `page.route` sobre `/api/v1/features`, como `config-setup.flag.spec.ts:20-27`, en ON y OFF [E14].

| Caso | Viewport CSS | `deviceScaleFactor` |
|---|---|---|
| Tablet vertical | 768×1024, 820×1180 | 2 |
| Tablet acostada | 1024×768, 1180×820, 1366×1024 | 2 |
| Laptop / escritorio | 1280×800, 1440×900, 1920×1080, 2560×1440 | 1 |
| Zoom 150 / 200 / 300 / 400% (sobre 1280×800) | 853×533, 640×400, 427×267, 320×200 | 1.5 / 2 / 3 / 4 [E9] |
| Zoom 50 / 67% (sobre 1920) | 3840×2160, 2880×1620 | 1 |

Aserciones:
1. Sin desborde lateral: `desbordeLateral` mide `document.documentElement`. Las tablas pueden hacer scroll en su propio contenedor (excepción de WCAG).
2. Ningún título, descripción ni acceso del peek cortado, ni al 200% ni al 400%.
3. El elemento con foco nunca queda bajo la barra.
4. Accesos alcanzables con ratón (hover) y con toque (`hasTouch: true`). **Antes de afirmar**, el caso táctil comprueba `matchMedia('(any-pointer: coarse)').matches === true` [E9]. El iPad con trackpad se revisa en el `/qa` manual.
5. Contraste del anillo de foco ≥ 3:1 en claro y oscuro.
6. En 2560 y 2880, el inicio no excede 1320px.

Alcance (D9): **inicio**, todos los casos y bloqueante. **Los otros 10 módulos** de `responsive.spec.ts`, bloqueantes hasta 200%; 300–400% en modo informe (`test.info().annotations`), y lo que falle sale como tarea aparte con evidencia. **Configuración** con `module_config_setup` ON a menos de 900px, bloqueante [E6].

Las capturas "antes/después" con datos usan un proyecto propio con `E2E_SKIP_RESET=1` sobre la base sembrada (`pnpm demo:ready`). La matriz corre sobre base vacía, así que los KPI en cero quedan cubiertos [E15].

## 5. F1 — Intents (PR 1, cambio de comportamiento)

**Transporte [E1]:** Next vuelve a montar `<AppShell>` en cada cambio de ruta (`AppShell.tsx:126`), así que un intent en estado o en prop se pierde. Se generaliza el precedente que ya funciona, `schedule/coverFocus.ts` [E13]:
- `app/routing/pendingIntent.ts` expone `requestIntent(screen, intent)` y `takeIntent(screen)`, y el valor solo se toma una vez.
- `open(id, intent)` → `requestIntent` → `go`. La pantalla lo toma en su inicializador: `useState(() => takeIntent("expenses"))`.
- F5, "atrás" y los enlaces directos no reabren nada. El intent **no** va en la URL.

**Tipado [E3]:** `IntentOf = { students: "new" | "search"; finance: "overdue"; … }`, con `ActionSpec` genérico sobre `moduleId`. El compilador rechaza un intent que la pantalla no entiende.

**Permisos [E4]:** cada acceso declara `requires` (permisos de escritura). El peek y ⌘K lo omiten si el principal no los tiene. Regresión obligatoria: un rol con `expenses:read` sin `expenses:write` no ve "Registrar egreso".

**Tabla verificada contra el código (revisión de ingeniería):**

| Módulo | Acceso en el peek | Intent | Destino existente | Requiere |
|---|---|---|---|---|
| Estudiantes | Nuevo alumno | `students:new` | `QuickAdd` (`StudentsScreen.tsx:1158`) | escritura de alumnos |
| Estudiantes | Buscar alumno | `students:search` | enfocar el buscador (el filtro expone ref/`autoFocus`) | lectura |
| Actividades | Nueva actividad | `activities:new` | modo formulario (`ActivitiesScreen.tsx:82`) | `activities:write` |
| Horarios | Registrar ausencia | `schedule:absence` | `AbsenceDrawer` (`ScheduleScreen.tsx:122`) | `manage` + escritura |
| Docentes | Agregar profesor | `teachers:new` | enfocar el primer campo de `TeachersPanel.tsx:104` | escritura de docentes |
| Finanzas | Ver vencidos | `finance:overdue` | pestaña "Vencidos" (`FinanceScreen.tsx:356`) | `finance:read` |
| Becas | Nueva beca | `scholarships:new` | `ScholarshipDrawer` (`ScholarshipsScreen.tsx:125`) | `scholarships:write` |
| Inventario | Existencias | `uniforms:inventory` | pestaña inventario (`UniformsScreen.tsx:140`) | lectura de inventario |
| Egresos | Registrar egreso | `expenses:new` | `ExpenseDrawer` (`ExpensesScreen.tsx:189`) | `expenses:write` |

Sin peek (D10; la tarjeta ya lleva ahí): Avisos, Asistencia, Calificaciones, Registro de pagos, Configuración, Administración.

**Cobro [E2]:** "Cobrar" vive en `PaymentsScreen`, cuyo buscador ya tiene `autoFocus`. `create.payment` pasa de `module:finance:payment` a `module:payments` sin intent. Se borra `Finance.tsx` (huérfano, solo lo importan tests) y la sección `payment` de `useFinance` si nada más la usa.

**Tests [E5]:** se reescriben las 8 aserciones de rutas de acción de `registry.test.ts` (`:70-128`) y la de `:143`. `palette.spec.ts` gana **una** prueba e2e (proyecto `demo`, `E2E_SKIP_RESET=1`): ejecutar "Registrar egreso" abre el cajón. Regresión obligatoria: el intent sobrevive al remontaje del shell.

## 6. Orden TDD

1. **F0:** caracterización del inicio actual (OFF); capturas "antes" en los 4 estados (con datos, cargando, permisos fallidos, sin módulos); inventario de sombras por grep.
2. **F1:** `pendingIntent` (rojo → verde), un test rojo por intent, filtro `requires`, mover el cobro, borrar `Finance.tsx`, reescribir el registro, e2e de un intent. Luego `/review` y `/qa` del buscador. **PR 1.**
3. **F2a:** test de estilos calculados idénticos (OFF) → clases, `--topbar-h`, `ModuleHeaderIcon`, test de grep de sombras.
4. **F2b:** test de que `LookAttribute` sigue al flag y a `gl-look` → tokens bajo el selector; actualizar `DESIGN-SYSTEM.md`.
5. **F3:** tests de componente en rojo:
   - agrupación y orden; `group` obligatorio; ≤4 sin títulos; área vacía omitida;
   - peek por hover/⋯/teclado; peek cerrado `hidden`; Esc devuelve el foco; un solo peek abierto;
   - accesos filtrados por `requires`;
   - los 5 estados de KPI ("cargando no muestra 0", "error muestra Reintentar", "el texto de cero solo tras una lectura exitosa");
   - `home.i18n`.
6. **F4:** matriz §4.3; `/qa` en el navegador visible con base local vacía (reglas qa-browser), incluido el proyector si está disponible; `/design-review`; `/review-animations`; `/review`. **PR 2.**

## 7. Riesgos

| Riesgo | Mitigación |
|---|---|
| dev es la demo diaria | Flag OFF por defecto. Se enciende tras las compuertas tech-lead + security y con visto bueno del dueño. Reversa = deploy del gateway (~6 min) |
| Barra flotante tapa "Paso N de 8" en Configuración | `--topbar-h` compartido + caso bloqueante en la matriz [E6] |
| Intent perdido por remontaje | Traspaso de un solo uso + e2e [E1] |
| Acceso de escritura visible para un rol de lectura | `requires` + regresión [E4] |
| KPI que afirma algo falso | Tabla de estados §3.4 [D2] |
| `backdrop-filter` lento en iPads viejos | `@supports` + medición de FPS en `/qa` |
| Pantallas que no refluyen a 320px | Modo informe + backlog (D9), deuda que ya existía |
| Look "generado" (grano + vidrio + serif + halo) | Halo nunca con desplazamiento 0, borde del color del área, serif solo en el `h1` [D4] |

## 8. Flag

`module_ui_refresh`: fila nueva en `.claude/docs/FLAGS.md`, con el mismo formato que `module_config_setup`. **No** entra en la lista por defecto del compose: en local se enciende con `ENABLED_MODULES=<lista>,ui_refresh` y en las pruebas con `page.route` [E14]. Es solo presentación y no protege ninguna ruta. **Retiro:** cuando el dueño acepte, se borran el estilo viejo, el flag y los tests que solo cubren la vista vieja.

## 9. Estimación (tiempo de agente)

| Fase | Estimado |
|---|---|
| F0 | ~45 min |
| F1 | 2.5–3.5 h (9 intents, 2 de solo enfoque; `pendingIntent`; registro; e2e) |
| F2a | ~2.5 h (TopBar, tarjetas, 17 encabezados, test de grep) |
| F2b | ~2 h |
| F3 | ~4 h (estados de KPI, teclado, i18n) |
| F4 | ~3 h + correcciones |
| **Total** | **~15–16 h** (antes 12–13 h: crecieron los estados de KPI, el refactor en línea y los 17 encabezados) |

## 10. Fuera de alcance

- Capa B del buscador (registros de alumnos), que sigue en `PLAN-command-palette-v2.md`.
- El resto de la deriva del registro ⌘K (`documents`, `billing`, `reports`, `settings.create.teacher`).
- Reflujo a 320px de los 10 módulos que no son el inicio (backlog, D9).
- Partir el flag en dos.
- La franja "Tu día" (dirección B).

## 11. Pendiente

- [x] `/plan-design-review` (16 hallazgos, aplicados)
- [x] `/plan-eng-review` (16 hallazgos, aplicados)
- [x] **Aprobación del dueño** (checkpoint 2, 2026-10-11)
- [x] F1 construido en `feature/home-intents` (PR 1). Desvíos del plan:
  - Además de `Finance.tsx` se borraron los 11 archivos que solo él alcanzaba (`useFinance`, sus paneles —incluido `GeneratePanel`— y `totals`).
  - Las 8 aserciones de `registry.test.ts:70-128` se dejaron: siguen siendo ciertas (esos comandos no tienen pantalla con intent). Cambiaron la de `:143` (cobro) y la de `CommandPaletteHost.test.tsx` ("Crear alumno" → `module:students:new`), y se agregaron 18 casos de intents y `requires`.
  - ⌘K ganó 7 comandos nuevos (Buscar alumno, Agregar profesor, Ver cargos vencidos, Registrar egreso, Registrar ausencia, Nueva actividad, Ver existencias). `settings.create.teacher` ("Crear profesor" → Configuración) sigue fuera de alcance (§10).
- [ ] F2a–F4 (PR 2)
