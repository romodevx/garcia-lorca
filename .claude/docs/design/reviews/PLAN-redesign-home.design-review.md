# Revisión de diseño: PLAN-redesign-home.md

Fecha: 2026-10-10 · rama `feature/redesign-home` (HEAD `b1232561`) · skill `/plan-design-review`, en modo no interactivo.

Revisé: `.claude/docs/design/PLAN-redesign-home.md`, la vista previa aprobada (`scratchpad/inicio-propuestas.html`, dirección C con "Captions adaptado"), `apps/web/app/_screens/Dashboard.tsx`, `apps/web/app/globals.css`, `apps/web/app/_design/{TopBar,Page,data}.tsx|ts`, `.claude/docs/design/DESIGN-SYSTEM.md` y `~/.claude/rules/ui-forms-and-modals.md`.

## Cómo se hizo esta revisión (léelo antes que el resto)

- **No interactiva.** El skill pide aprobar cada hallazgo antes de corregir el plan. Aquí no podía preguntarle a nadie, así que cada hallazgo trae su **corrección recomendada como propuesta**. **El plan no se editó.** Lo que de verdad le toca decidir al dueño está aparte, en "Decisiones para el dueño".
- **No hice maquetas** (lo pidió quien me lanzó). La vista previa aprobada es la referencia visual.
- **No consulté a modelos externos** (Codex ni un subagente). Decidido automáticamente: "B) seguir sin ellos", para no salir del modo de solo lectura. Todo lo que sigue es la opinión de un solo modelo.
- **No corrí los scripts de gstack** (preámbulo, telemetría, `gstack-review-log`) ni escribí en `~/.gstack`. La restricción era "solo lectura y un archivo de reporte". Por eso este reporte no aparece en el tablero de revisiones.
- No existe `DESIGN.md`. La referencia es `.claude/docs/design/DESIGN-SYSTEM.md` junto con `globals.css`.

## Calificaciones

| Dimensión | Nota | Por qué no es 10 |
|---|---|---|
| Paso 0: qué tan completo está el plan | **6/10** | Tiene un inventario de comportamiento ejemplar (B1–B12) y un razonamiento serio sobre zoom y reflujo. Le faltan tres cosas: la trampa del estilo en línea, los estados de carga y error de los KPI, y el comportamiento en monitores anchos. |
| 1. Arquitectura de la información | **7/10** | Las áreas y el orden por uso diario están claros. Faltan tres módulos sin área asignada, la jerarquía de encabezados y qué pasa con el encabezado "Módulos". |
| 2. Estados de interacción | **5/10** | El KPI en cero no distingue entre "cargando", "falló" y "de verdad es cero". Tampoco están definidos el estado del ⋯ abierto ni la tarjeta sin accesos. |
| 3. Recorrido y arco emocional | **6/10** | Las prisas del director están bien pensadas (accesos directos, siguiente paso en cero). Le falta el recorrido de quien usa solo el teclado, y el primer clic de "toda la app" cae en encabezados con el estilo viejo. |
| 4. Riesgo de verse "hecho por IA" | **6/10** | El plan se contradice con el acento lima. El halo verde en modo oscuro es literalmente el patrón *dark-glow*. Grano, vidrio, serif y cuadros de ícono, todo junto, se acerca a un "look" reconocible (lo aprobó el dueño y no lo reabro). |
| 5. Alineación con el sistema de diseño | **4/10** | Hay **estilos en línea que ninguna regla bajo `data-look` puede vencer**. El inventario de sombras sale corto (10 archivos contra unos 25). `--hair` no existe. `DESIGN-SYSTEM.md` quedaría desactualizado. |
| 6. Responsive y accesibilidad | **6/10** | Es muy bueno en reflujo y zoom. Fallan tres cosas: con el teclado, cada tarjeta suma 3 o 4 paradas de Tab; el anillo de foco lima no llega a 3:1 sobre blanco; y el modo de colores forzados no está cubierto. |
| **Global (la nota más baja de las seis)** | **4/10** | Sube a ≥8 con las correcciones 1, 2, 3, 5 y 6. |

## Hallazgos

### 1. [BLOQUEANTE] Los estilos en línea vencen a `html[data-look="refresh"]`: con el flag ON, la barra superior y las tarjetas del inicio no cambiarían
- **Evidencia:** `TopBar.tsx:35-48` pone `position: sticky`, `top: 0`, `height: 78`, `backdropFilter` y `borderBottom` **en línea**. Todo `Dashboard.tsx` (`ModuleCard` :17-103, `KpiCard` :105-165) es estilo en línea, con el hover hecho con `useState`. El plan §3.3 dice "Todo el estilo nuevo vive en `globals.css` bajo ese selector". El repositorio ya documentó esta trampa tres veces (`globals.css:351`, `:545`, `:599`; `Page.tsx:13-16`): un estilo en línea gana sobre cualquier regla de autor.
- **Corrección al plan (§3.3 y §6, paso F2):** agregar como **primer paso de F2**: "Mover a clases de `globals.css` todo lo que el flag cambia: `TopBar` (position, top, height, margin, radio, fondo, desenfoque, borde), los cuadros de ícono de encabezado y las tarjetas. Test de caracterización: con OFF, `getComputedStyle` es idéntico al de hoy (`.topbar`, la tarjeta de módulo y la tarjeta KPI)". En §3.2 agregar: "La vista nueva del inicio se escribe con clases. El hover, el foco y el peek van en CSS (`:hover`, `:focus-within`, `@media (hover:hover)`), no en `useState`: un estilo en línea no puede usar media queries".

### 2. [BLOQUEANTE] El KPI en cero mentiría mientras carga o cuando la lectura falla
- **Evidencia:** `Dashboard.tsx:313` arranca en `{pending: 0, collected: 0, ...}`. En `:323`, `:339` y `:340` los errores se tragan con `.catch(() => {})`. En `:328`, si no hay ciclo, se sale sin avisar. Con §3.4, una red lenta o un 500 mostraría "**Aún no hay cargos este ciclo**" al director en la demo diaria. Hoy por lo menos dice "$0.00", que es ambiguo. El texto nuevo es una afirmación falsa.
- **Corrección al plan (§3.4):** sustituir por una tabla de estados por KPI:

  | Estado | Lo que ve el usuario |
  |---|---|
  | Cargando | Etiqueta y un renglón esqueleto en lugar del número (sin "0") |
  | Error | "No se pudo cargar" con un botón **Reintentar** dentro de la tarjeta (el clic en la tarjeta sigue abriendo el módulo) |
  | Sin ciclo activo | "Sin ciclo activo" con "Ir a Configuración →" (solo con `config:read`) |
  | Cero con lectura exitosa | El texto de §3.4 |
  | Con valor | El número de hoy |

  Agregar a §6, paso F3, los tests "KPI cargando no muestra 0", "KPI con error muestra Reintentar" y "el texto de cero solo aparece tras una lectura exitosa". Que la vista nueva deje de tragarse los errores (con OFF, el comportamiento B5 queda igual).

### 3. [IMPORTANTE] El inventario de "10 archivos con `boxShadow` literal" se queda corto, y "toda la app" quedaría a medias
- **Evidencia:** `grep` encuentra sombras literales en unos 25 archivos. Son **17 encabezados de módulo** con cuadro de color sólido y resplandor de color (StudentsScreen:1006, FinanceScreen:372, FinanceHeader:31, ExpensesScreen:122, AdminScreen:225, TeachersScreen:143, AnnouncementsScreen:139, ScholarshipsScreen:89, ActivitiesScreen:92, UniformsScreen:164, PaymentsScreen:117, ScheduleScreen:82, Catalog:83, SetupHeader:20, AttendanceHeader:21, TeacherCapture:69, DirectorCapture:59), más **4 FAB** (Students:1156, Expenses:182, Scholarships:119, Activities:121), el resplandor lima del `Btn` primario (`Controls.tsx:89`), `ChargeRow.tsx:105` y `Brand.tsx:38`. Además, `--shadow-hover` **solo lo usa el Dashboard**, así que "141 usos de tokens se actualizan solos" es cierto para `--shadow-card`, pero el halo no llega a ningún otro lado.
- **Consecuencia:** el inicio estrena cuadros tintados y sobrios, pero el primer clic lleva a un encabezado con un cuadro saturado y resplandor. Contradice D3.
- **Corrección al plan (§3.3):** que la lista no se escriba a mano: "F0 genera el inventario con `grep -rn boxShadow app | grep -v 'var(--shadow'`". Agregar a la tabla de §3.3 la fila "Cuadro de ícono del **encabezado de módulo**: hoy color sólido con resplandor; con el flag, tintado al 13% con ícono del color del área (igual que en el inicio)". Agregar también la fila "FAB y `Btn` primario: resplandor lima → ver Decisión O4". Extraer un componente `ModuleHeaderIcon` para no editar el mismo estilo 17 veces.

### 4. [IMPORTANTE] El plan se contradice con el lima
- **Evidencia:** §3.3 dice "Lima queda reservado para lo primario y lo seleccionado". En la misma tabla, `--shadow-hover` se vuelve un halo lima en **cada** tarjeta al pasar el cursor, y la vista previa también pinta el borde lima al 45% (`inicio-propuestas.html:122`). En modo oscuro, `rgba(122,196,0,.13)` sobre `#07150D` es el patrón *dark-glow*.
- **Corrección al plan (§3.3):** el dueño aprobó el halo (D2), así que no se quita. Hay que escribir la excepción en lugar de negarla: "Lima: lo primario, lo seleccionado y el halo de hover (`--halo`, token propio, sin reutilizar `--shadow-hover`). El borde de hover usa el color del **área** al 45%, no lima. Oscuro: halo al .10 con desplazamiento 14px, nunca de desplazamiento 0". Así, las tarjetas de Dinero no se iluminan de verde.

### 5. [IMPORTANTE] Teclado: el peek suma de 2 a 3 paradas de Tab por tarjeta, y esos botones ocultos se pueden enfocar
- **Evidencia:** §3.2: "Tab llega a la tarjeta y luego al ⋯; `:focus-within` muestra el peek", pero con puntero fino "el ⋯ queda oculto". Con 14 tarjetas, el director recorre de 42 a 56 paradas para llegar a Administración. En la vista previa, el peek oculto es solo `translateY(101%)` (`:208`): los botones siguen en el árbol de accesibilidad y se pueden enfocar.
- **Corrección al plan (§3.2, viñeta "Teclado"):** "Cerrado, el peek está `hidden` (fuera del orden de Tab y del árbol de accesibilidad). El ⋯ existe siempre: visible con puntero grueso y, con puntero fino, visible en `:focus-within` de la tarjeta. Recorrido: Tab a la tarjeta → Tab al ⋯ → Enter abre → Tab o flechas entre accesos → Esc cierra y regresa al ⋯. Son 2 paradas por tarjeta. El hover muestra el peek para el ratón sin cambiar el orden de Tab". Así se cumple WCAG 1.4.13 (descartable con Esc, se puede pasar el cursor encima, persistente).

### 6. [IMPORTANTE] La geometría del peek no se puede implementar como está escrita
- **Evidencia:** §4.2 dice "El peek se superpone dentro de la tarjeta; si su contenido es más alto que la tarjeta, la tarjeta crece (`min-height` calculado con `max-content`)". Con CSS no se puede dimensionar un padre a partir de un hijo `position:absolute`. En la vista previa, `.pcard{overflow:hidden; min-height:140px}` y 3 accesos de unos 32px tapan toda la descripción. Al 200% de zoom, los accesos se cortan.
- **Corrección al plan (§4.2):** "La tarjeta reserva `min-height` en `rem` = bloque de título + 3 renglones de acceso (escala con el zoom). El peek por hover se superpone **solo sobre la descripción** (el título y el ícono siguen visibles), con `max-height: 100%` y `overflow:auto` como último recurso. El peek que se abre con ⋯ (puntero grueso o teclado) es un desplegable **dentro del flujo**: la tarjeta crece, aceptable porque es una acción explícita". Agregar a la aserción (2) de §4.3: "ningún acceso del peek queda cortado al 200% ni al 400%".

### 7. [IMPORTANTE] En monitores anchos, cada área deja la mitad de la fila vacía
- **Evidencia:** `globals.css:257` sube `--page-max` a 1800px desde 1600. Con `auto-fill, minmax(232px,1fr)` caben 7 columnas: Alumnos y familias (3) deja 4 huecos, Académico (4) deja 3. Con zoom de 67% sobre 1920 (2880 CSS) caben unas 12. La vista previa evita esto con `.main{max-width:1240px}` (`:109`), y el plan no lo copió.
- **Corrección al plan (§4.2):** ver Decisión O1. Recomendación: "El cuerpo del inicio se acota a `max-inline-size: 1320px` (el `--page-max` base), centrado, en todo ancho; la cuadrícula usa `auto-fit`". Agregar a §4.3 la aserción "en 2560 y 2880, ninguna fila de un área deja más de una columna vacía".

### 8. [IMPORTANTE] Hay módulos del registro sin área
- **Evidencia:** `data.ts:81` Facturación (`module_billing`), `:82` Reportes (`module_reports`) y `:74` Documentos (`hidden`) no aparecen en §3.1. Si se enciende uno de esos flags, no se sabe dónde se pinta.
- **Corrección al plan (§3.1):** "`group` es **obligatorio** en el tipo `ModuleDef` (el compilador obliga a asignarlo). Facturación → Dinero; Reportes → ver Decisión O2; Documentos → Alumnos y familias". Test: todo módulo de `MODULES` tiene `group`.

### 9. [IMPORTANTE] Los KPI en cero están definidos solo para "Por cobrar", y la vista previa no coincide con el plan
- **Evidencia:** la vista previa manda "Cobrado" en cero a **Registro de pagos** (`inicio-propuestas.html:373`) y "Por cobrar" a **Generar colegiaturas** (`:370`). El plan B5 manda los dos a `finance`, y §3.4 solo describe "Por cobrar". El KPI Alumnos en cero tampoco está cubierto.
- **Corrección al plan (§3.4):** tabla de tres filas:
  - Alumnos = 0 → "Aún no hay alumnos" con "Inscribir alumno →" (intent de F1, si quedó probado; si no, "Ir a Estudiantes →").
  - Por cobrar = 0 → texto actual del plan.
  - Cobrado = 0 → "Sin cobros todavía" con "Ir a Registro de pagos →", solo si el usuario ve `payments`; si no, Finanzas.

  Anotar que esto cambia el destino del clic **solo con el flag ON y en cero**, y que el comportamiento B5 sigue igual con OFF.

### 10. [IMPORTANTE] El anillo de foco no se ve sobre tarjetas planas
- **Evidencia:** `globals.css:148`: `outline: 3px solid color-mix(in oklab, var(--lime) 55%, transparent)`. Sobre blanco da un verde claro de unos 1.6:1, por debajo del 3:1 que pide WCAG 1.4.11 / 2.4.13. Hoy la sombra ayuda a ubicar la tarjeta. Con bordes finos y sin sombra, el foco es lo único que queda.
- **Corrección al plan (§3.3, nueva fila):** "Anillo de foco con el flag: `outline: 2px solid var(--forest)` con `outline-offset: 2px` (en oscuro `--lime`). Contraste ≥ 3:1 contra la superficie y contra el fondo". Agregar a §4.3 la aserción "contraste del anillo de foco ≥ 3:1 (claro y oscuro)".

### 11. [MENOR] Semántica: encabezados y listas
- **Evidencia:** §3.2 propone un `<article>` por tarjeta, pero no define niveles de encabezado. Hoy existen `<h1>` (saludo) y `<h2>` "Módulos" con el subtítulo "Todo el sistema, a la vista" (`Dashboard.tsx:384-385`). La vista previa usa "Tus módulos" cuando hay ≤4.
- **Corrección al plan (§3.1):** "`h1` = saludo. Con áreas: cada área es un `<section aria-labelledby>` con su `h2` y una `<ul>` de tarjetas (la tarjeta es un `<li>`, sin `<article>`). Se quita el `h2` 'Módulos' y su subtítulo. Con ≤4 módulos: un `h2` 'Tus módulos' y una `<ul>`". La flecha "→" de cada acceso lleva `aria-hidden`.

### 12. [MENOR] WCAG 2.4.11: falta escribir el mecanismo, no solo la prueba
- **Corrección al plan (§4.2):** "Token `--topbar-h` (alto de la cápsula más 12px de margen). Con la barra pegada: `html { scroll-padding-top: calc(var(--topbar-h) + 16px) }`". Así, la aserción (3) de §4.3 verifica un mecanismo que existe.

### 13. [MENOR] `--hair` no existe, y una línea fina demasiado tenue desaparece en un proyector
- **Evidencia:** no aparece en `globals.css`. La vista previa usa `#E2E9DF` y `#1F3828` (`:13`, `:39`), con unos 1.2:1 contra el blanco.
- **Corrección al plan (§3.3):** "Nuevo token `--hair` en claro y oscuro. La tarjeta en reposo usa `--border-strong` (`#D6DED2`) si la `/qa` en el proyector del colegio muestra que las tarjetas se pierden". Y en modo de colores forzados: `@media (forced-colors: active) { .home-card { border: 1px solid CanvasText } }`, porque ahí el halo y las líneas finas desaparecen.

### 14. [MENOR] El emoji del saludo
- **Evidencia:** `Dashboard.tsx:354` pinta "🌱". La vista previa lo cambia por el ícono SVG `sprout` en lima (`:357`).
- **Corrección al plan (B1):** "El emoji se sustituye por `<Icon name="sprout">` con `aria-hidden` (agregarlo a `Icon.tsx` si no existe)".

### 15. [MENOR] La animación de entrada no está definida
- **Evidencia:** hoy hay `fade-up` con un retraso escalonado de `i*0.03s` por tarjeta (`Dashboard.tsx:41`). La vista previa escalona por **sección** (`:200-201`). §3.2 solo describe el movimiento del peek.
- **Corrección al plan (§3.2):** "Entrada: una sola, por sección (`fade-up`, 60ms entre secciones), sin escalonar por tarjeta. Hover: `translateY(-2px)`. Con `prefers-reduced-motion`: sin entrada y sin desplazamiento".

### 16. [MENOR] `DESIGN-SYSTEM.md` quedaría desactualizado
- **Evidencia:** dice `--r-lg/18px` (en `globals.css:29` vale 22, y el plan lo baja a 16), dice "No inventar estilos nuevos" y describe el cuadro de ícono sólido en `--forest`.
- **Corrección al plan (§6, paso F2):** "Agregar a `DESIGN-SYSTEM.md` una sección 'Look refresh (`module_ui_refresh`)' con los tokens nuevos (`--hair`, `--halo`, `--topbar-h`, radios), el cuadro tintado, el anillo de foco y la regla del lima. Corregir el valor de `--r-lg`".

## Lo que ya existe y el plan debe reutilizar
- `Page` (`_design/Page.tsx`) y `--page-max`; `desbordeLateral` de `responsive.spec.ts`; el patrón existente de ocultar EN, tema y correo por debajo de 720px; `useFlag` (ya se usa en `AppShell.tsx`); `Icon` (con `sprout`, si existe); `fade-up`; `--ease`; `::selection` y las barras de desplazamiento ya tematizadas (`globals.css:122`, `:131-134`).

## Fuera de alcance (pospuesto a propósito)
- Los colores por área en ⌘K (el plan ya lo deja fuera; correcto).
- La franja "Tu día" de la dirección B (necesita datos nuevos).
- La navegación pensada para teléfono (D6).

## Decisiones para el dueño
- **O1. Monitores anchos (≥1600 px o zoom de 67%):** (A, recomendada) acotar el inicio a 1320px centrado, que es lo que muestra la vista previa aprobada; (B) acomodar dos áreas lado a lado desde 1600px (más denso, pero se parece a la dirección A, que el dueño no eligió).
- **O2. Dónde va Reportes cuando se encienda `module_reports`:** recomendación: **Sistema** (son indicadores, no trabajo diario). Alternativa: Dinero, si los reportes van a ser sobre todo financieros.
- **O3. Encabezados de los 17 módulos con el flag ON:** (A, recomendada) cuadro tintado igual que en el inicio (cumple D3: "toda la app de una vez"); (B) dejarlos sólidos y cambiar solo el inicio y la barra (menos trabajo, pero el primer clic se ve de otra app).
- **O4. Resplandor lima del FAB y del `Btn` primario:** (A, recomendada) se queda, porque es "lo primario" y el lima le corresponde; (B) aplanarlo para que el halo de hover sea el único resplandor de la app.
- **Nota, sin reabrir D2:** grano, barra de vidrio, serif en el saludo, halo de color y cuadros de ícono sobre cada título, todo junto, se acercan a un "look" de interfaz generada que se reconoce. Mitigación ya incluida en los hallazgos 4 y 13: el halo nunca de desplazamiento 0, el borde de hover del color del área y la serif solo en el `h1`.

## Tareas de implementación (derivadas de los hallazgos)
- [ ] **T1 (P1)**: mover a clases los estilos en línea de TopBar, tarjetas e íconos de encabezado; test de estilos calculados idénticos con OFF (H1).
- [ ] **T2 (P1)**: estados del KPI: cargando, error, sin ciclo, cero y valor; sin errores tragados en la vista nueva (H2, H9).
- [ ] **T3 (P1)**: peek `hidden` cuando está cerrado; ⋯ visible al enfocar; 2 paradas de Tab por tarjeta; tests de teclado (H5).
- [ ] **T4 (P1)**: geometría del peek en `rem`; desplegable dentro del flujo cuando se abre con ⋯; aserción de zoom (H6).
- [ ] **T5 (P2)**: inventario por grep; `ModuleHeaderIcon` en 17 encabezados (H3, O3).
- [ ] **T6 (P2)**: `group` obligatorio; Facturación, Reportes y Documentos asignados (H8, O2).
- [ ] **T7 (P2)**: tokens `--halo`, `--hair`, `--topbar-h`; anillo de foco con flag; `scroll-padding-top`; colores forzados (H4, H10, H12, H13).
- [ ] **T8 (P2)**: límite de ancho del inicio y aserción en 2560/2880 (H7, O1).
- [ ] **T9 (P3)**: semántica de encabezados y listas, ícono `sprout`, una sola entrada animada, actualización de `DESIGN-SYSTEM.md` (H11, H14, H15, H16).

## Resumen

| | |
|---|---|
| Paso 0 | 6/10 |
| Pasos 1–6 | 7 · 5 · 6 · 6 · 4 · 6 (todos quedan igual: ninguna corrección se aplicó al plan) |
| Global | 4/10 → ≥8/10 estimado si se adoptan H1, H2, H3, H5 y H6 |
| Hallazgos | 16 (2 bloqueantes, 8 importantes, 6 menores) |
| Decisiones del dueño | 4 (O1–O4) |
| Maquetas | 0 (excluidas por instrucción) |
| Voces externas | ninguna (decidido automáticamente) |

**UNRESOLVED DECISIONS:**
- H1–H16: correcciones propuestas, todavía sin aplicar al plan (falta que el autor del plan las apruebe).
- O1–O4: les toca decidir al dueño.
