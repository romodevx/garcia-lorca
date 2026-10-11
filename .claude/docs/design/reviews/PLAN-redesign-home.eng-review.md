# Revisión de ingeniería: PLAN-redesign-home.md

- **Objetivo revisado:** `.claude/docs/design/PLAN-redesign-home.md` en `feature/redesign-home` @ `b1232561` (el plan no se modificó; este archivo es el informe).
- **Fecha:** 2026-10-10 · skill `/plan-eng-review`, modo FULL_REVIEW.
- **Cómo se hizo:** lectura de código y `grep`; sin correr la suite, sin levantar servidores. Esta sesión no podía preguntar al dueño: cada pregunta de la skill se decidió con la opción recomendada y queda registrada en "Decisiones auto-tomadas". Las que de verdad le tocan al dueño están en "Decisiones para el dueño".
- **Relación con la revisión de diseño** (`PLAN-redesign-home.design-review.md`): coincidimos de forma independiente en los estilos en línea, en el conteo de sombras y en los módulos sin área. Aquí se agregan la evidencia de ingeniería y los hallazgos que esa revisión no cubre: transporte del intent, destino equivocado del cobro, permisos de escritura, el acoplamiento con la puesta en marcha, la matriz y el flag.

## Veredicto corto

El plan está bien orientado (F1 separado, flag OFF, inventario de comportamiento, matriz real en navegador), pero **tres hechos del §5 están mal** y uno de ellos rompe el diseño de F1 tal como está escrito: el shell se **vuelve a montar en cada cambio de ruta**, así que un intent pasado como prop o estado de `AppShell` se pierde. Además, el test rojo que propone F1 apunta al módulo equivocado, y la mitad de los accesos candidatos ya son la vista por defecto de su pantalla (no necesitan intent). Con las enmiendas de abajo, el plan se puede ejecutar.

---

## Hallazgos

Formato: `[severidad] (confianza N/10) archivo:línea — descripción`, luego evidencia y la enmienda concreta al texto del plan.

### 1. [BLOQUEANTE] (9/10) F1 §5.2: el intent no puede viajar como prop ni estado del shell; Next vuelve a montar `<AppShell>` en cada navegación

- **Plan:** §5.2 "`open(id, intent)` pasa el intent a la pantalla; cada pantalla destino recibe `intent?: …`".
- **Evidencia:**
  - `apps/web/app/AppShell.tsx:126-131`: `go` sólo hace `router.push(viewToPath(next))`; la pantalla sale de la URL (`AppShell.tsx:118` `pathToView(pathname)`).
  - `apps/web/app/[[...slug]]/page.tsx`: todo el shell es una sola página catch-all.
  - `apps/web/app/_screens/config/levelDeletion/pendingNotice.ts:4-7`: "Next REMOUNTS <AppShell> on every route change … state set on the deletion page dies with it". `apps/web/app/schedule/coverFocus.ts:3-5` dice lo mismo y por eso usa un valor de módulo que se toma una sola vez.
  - `AppShell.tsx:752`: `requestCoverFocus(absence); go({ screen: "schedule" })`. Ése es el patrón que ya funciona para "abrir Horarios en un estado concreto".
- **Consecuencia:** guardar el intent en estado de `AppShell` (o pasarlo como prop calculada desde ese estado) da un test unitario verde con `AppShell` montado una sola vez y un fallo en el navegador. Es justo la clase de defecto que la memoria `palette-three-defects-2026-09` describe: jsdom verde, función ausente.
- **Enmienda (§5.2, reemplazar):** "`open(id, intent)` deja el intent en un **traspaso de un solo uso a nivel de módulo** (`app/routing/pendingIntent.ts`: `requestIntent(screen, intent)` / `takeIntent(screen)`), igual que `coverFocus.ts`, y después llama a `go`. La pantalla destino lo toma en su inicializador de estado (`useState(() => takeIntent("expenses"))`). Así 'se consume una vez' (§5.3) sale gratis: F5 y volver atrás no reabren el cajón. Un test e2e (no sólo jsdom) cubre al menos un intent de punta a punta."

### 2. [BLOQUEANTE] (9/10) El test rojo de F1 apunta al módulo equivocado: "Cobrar" vive en `payments`, no en `finance`

- **Plan:** §5.1 "`navigate("module:finance:payment")` llega al formulario de cobro"; §5 tabla "Registro de pagos · Cobrar a un alumno".
- **Evidencia:**
  - `apps/web/app/command/registry.ts:50`: `{ moduleId: "finance", … intent: "payment" }`.
  - `AppShell.tsx:23` monta `FinanceScreen`; `Finance.tsx` (el único que acepta `intent`, `Finance.tsx:18-36`) sólo se importa en tests: es código huérfano.
  - `FinanceScreen.tsx:235-259` no tiene formulario de cobro general; sólo `PayDrawer` por cargo (`:476`), que exige elegir un cargo.
  - La ventanilla es `PaymentsScreen` (`data.ts:54` "Cobrar en ventanilla"), y su buscador ya tiene `autoFocus` (`PaymentsScreen.tsx:133`). El cobro (`CollectDrawer`, `:185`) exige un alumno elegido (`:166`).
- **Enmienda (§5.1):** "Test rojo: el comando 'Registrar pago' de ⌘K navega a `/pagos` (route `module:payments`, **sin intent**) y el foco queda en 'Buscar alumno'. Se borra `intent: "payment"` del registro y **se elimina `Finance.tsx` + `useFinance` sección `payment` si nada más los usa** (código muerto que hoy confunde a cualquier lector, como confundió a este plan)."

### 3. [IMPORTANTE] (9/10) La mitad de los accesos candidatos ya son la vista por defecto; otros tres no existen como se describen

Ver la tabla verificada abajo. Resumen: de 16 accesos, **5 ya existen y sólo necesitan intent**, **6 son la vista por defecto** (el intent sobra), **3 necesitan trabajo** (enfoque o mover el destino) y **2 no existen** como los nombra el plan.
- **Enmienda (§5, tabla):** reemplazarla por la tabla verificada de este informe, y agregar la regla: "un acceso que coincide con la vista por defecto del módulo **no lleva intent**: es la misma tarjeta. En el peek se muestra sólo si el texto aporta (p. ej. 'Pasar lista de hoy'), y apunta al módulo sin intent."

### 4. [IMPORTANTE] (9/10) Los accesos de escritura deben filtrarse por permiso de escritura, no sólo por el de lectura del módulo

- **Evidencia:** los cajones sólo existen con permiso de escritura: `ExpensesScreen.tsx:43,137` (`expenses:write`), `ScholarshipsScreen.tsx:35,96` (`scholarships:write`), `ActivitiesScreen.tsx:38,99` (`activities:write`), `ScheduleScreen.tsx:44-46,92` (`manage` + `canWrite`). En cambio `isModuleVisible` (`data.ts:109`) y el registro (`registry.ts:93-96`) sólo miran el permiso de lectura.
- **Consecuencia:** un rol de sólo lectura vería "Registrar gasto", haría clic y caería en la lista sin cajón. Para el usuario es un botón que no hace nada, el mismo síntoma que motivó D4.
- **Enmienda (§3.2 y §5):** "Cada acceso declara `requires?: string[]` (permisos de escritura). El peek y ⌘K lo omiten si el principal no los tiene. Test: rol con `expenses:read` sin `expenses:write` no ve 'Registrar gasto' ni en el peek ni en ⌘K." Nota: esto toca `buildCommandRegistry`, que hoy sólo recibe permisos de lectura (`registry.ts:84`); el `permissions` que le pasa AppShell es la lista completa, así que alcanza con leer de ahí.

### 5. [IMPORTANTE] (9/10) "17 aserciones" está desactualizado: hoy son 8 rutas de acción sin intent

- **Plan:** §5 "`registry.test.ts` lo afirma como correcto en 17 lugares"; §5.4 "Se reescriben las 17 aserciones".
- **Evidencia:** `registry.test.ts` tiene 11 aserciones sobre `.route`: 8 de acciones sin intent (`:70,71,72,89,90,107,127,128`), 2 de navegación que son correctas sin intent (`:142,148`) y 1 que **ya** afirma un intent (`:143` `module:finance:payment`, que el hallazgo 2 cambia). El "17" viene de la memoria del 2-sep (`palette-three-defects-2026-09.md`), cuando el registro tenía 20 comandos.
- **Además:** §7 dice que `palette.spec.ts` "cambia sus expectativas". No es así: sus 5 pruebas (`palette.spec.ts:33,73,91,103,127`) sólo miden geometría, foco, vacío, accesibilidad y Esc. Ninguna navega a un comando.
- **Enmienda (§5.4 y §7):** "Se reescriben las 8 aserciones de rutas de acción (`registry.test.ts:70-128`) y la de `:143`. Se agrega a `palette.spec.ts` (proyecto `demo`, `E2E_SKIP_RESET=1`) **una** prueba nueva: ejecutar 'Registrar egreso' abre el cajón." Quitar la frase sobre `palette.spec.ts` de §7.

### 6. [IMPORTANTE] (9/10) La barra superior está acoplada a la puesta en marcha por su alto de 78px; la cápsula flotante rompe el pegado de "Paso N de 8"

- **Evidencia:**
  - `TopBar.tsx:36-47` fija en línea `position: sticky; top: 0; height: 78; backdropFilter…`. Un estilo en línea le gana a `html[data-look="refresh"] .topbar` (lo mismo advierte `globals.css:231-234`, y es el hallazgo 1 de la revisión de diseño).
  - `globals.css:735-738`: `--setup-topbar-h: 78px` y `scroll-padding-top` calculado sobre ese alto. `globals.css:756-760`: `.setup-mobilebar { position: sticky; top: var(--setup-topbar-h) }`.
  - `module_config_setup` está **ON en dev** (`FLAGS.md:38`).
- **Consecuencia:** con `module_ui_refresh` ON, una barra a 12px del borde y con otro alto deja la barra de pasos encimada o con un hueco, y el foco puede quedar tapado (WCAG 2.4.11) en la pantalla que hoy se presenta en las demos diarias.
- **Enmienda (§3.3, agregar):** "F2 empieza con un refactor **sin cambio visual** (flag OFF): los estilos en línea de `TopBar` pasan a `.topbar` en `globals.css`, y el alto se publica como `--topbar-h` en `:root`. `--setup-topbar-h` pasa a ser `var(--topbar-h)`, y bajo `data-look` vale `calc(alto de la cápsula + 12px)`. La matriz §4.3 incluye `/configuracion` con `module_config_setup` ON a menos de 900px." Corregir también §4.2: el patrón que esconde EN/tema/correo existe a **768px** (`globals.css:177`), no a 720.

### 7. [IMPORTANTE] (8/10) `data-look` no debe ponerlo `AppShell`: se monta de nuevo en cada ruta y no cubre todas las páginas

- **Plan:** §3.3 "`AppShell` pone `data-look="refresh"` en `<html>` cuando `useFlag(...)`".
- **Evidencia:** los flags se piden **una vez** en `AppFlagsProvider` (`flags-provider.tsx:32-46`), que vive en `layout.tsx` y sobrevive a la navegación. `AppShell` se monta de nuevo en cada ruta (hallazgo 1): un efecto con limpieza quitaría y volvería a poner el atributo en cada clic, y eso parpadea. Además `/activar` y `/comprobantes/...` no pasan por `AppShell` (`app/activar/page.tsx`, `app/comprobantes/[kind]/[id]/page.tsx`). Hay un parpadeo garantizado al arrancar: el primer pintado ocurre antes de que `/api/v1/features` conteste.
- **Enmienda (§3.3):** "Un componente `<LookAttribute/>` dentro de `AppFlagsProvider` sincroniza `data-look` con el flag. Se guarda el último valor en `localStorage` (`gl-look`) y el script previo al pintado de `layout.tsx` (el mismo de `gl-theme`) lo aplica antes del primer pintado. Si el gateway dice otra cosa, se corrige al llegar. Los comprobantes impresos quedan fuera con `@media print` (sin grano ni halo)."

### 8. [IMPORTANTE] (8/10) "Todos los zooms" en los 11 módulos significa que cada pantalla debe reflujar a 320px: es alcance nuevo, y la estimación no lo cubre

- **Plan:** §4.1 (400% = 320px), §7 "la matriz §4.3 corre sobre los 11 módulos", §9 "F4 ~2 h + correcciones".
- **Evidencia:** hoy `responsive.spec.ts` sólo prueba 1024×768 y 2560 (`:38-98`). Pantallas como `/alumnos` y `/finanzas` son tablas anchas. WCAG 1.4.10 exime las tablas de datos, siempre que el **contenedor** de la tabla haga scroll y no la página.
- **Consecuencia:** 15 viewports × 2 estados del flag × 11 rutas son 330 cargas, y es muy probable que varias pantallas fallen a 320px **con el flag OFF también**: eso es deuda que ya existe, no algo que cause el rediseño. Mezclarla en el PR 2 lo hace imposible de revisar y viola "comportamiento congelado".
- **Enmienda (§4.3 y §9):** "Inicio: matriz completa y bloqueante. Los otros 10 módulos: bloqueantes hasta 200% (640px); de 300 a 400% se corren **en modo informe** (`test.info().annotations`, sin fallar) y lo que falle entra al backlog como tarea separada, con evidencia. Las tablas se aceptan con scroll propio del contenedor (excepción de WCAG para tablas), y la aserción (1) mide `document.documentElement`, no la tabla."

### 9. [IMPORTANTE] (8/10) La emulación de zoom con viewport es válida para el reflujo, pero no es zoom real; le faltan dos ajustes

- **Valoración:** para el *layout*, el zoom del navegador equivale a reducir el ancho CSS del viewport (1280/4 = 320). La aritmética del §4.3 es correcta (853×533, 640×400, 427×267, 320×200; 3840×2160 y 2880×1620 para 50 y 67%). Es la aproximación habitual para probar WCAG 1.4.10.
- **Lo que no emula:** el zoom real también sube `devicePixelRatio` (bordes de 1px, el `--hair` que propone el plan, nitidez del grano y costo del `backdrop-filter`), y el tamaño mínimo de fuente del navegador. `hasTouch: true` en "Desktop Chrome" emula el toque. No he comprobado si con eso `(any-pointer: coarse)` coincide en esta versión de Chromium (confianza 5/10), así que la prueba (4) puede pasar sin ejercitar el ⋯.
- **Enmienda (§4.3):** "Los casos de zoom ≥100% usan `deviceScaleFactor` igual al zoom (1.5, 2, 3, 4). Antes de afirmar algo, el caso táctil comprueba su precondición: `expect(await page.evaluate(() => matchMedia('(any-pointer: coarse)').matches)).toBe(true)`. El iPad con trackpad (hover y toque a la vez) no se puede emular: se cubre en el `/qa` manual."

### 10. [IMPORTANTE] (8/10) El inventario de `boxShadow` literal tiene 27 archivos, no 10

- **Evidencia:** `grep` de `boxShadow` con valores en px dentro de `apps/web/app/**/*.tsx` (sin tests): 27 archivos. El mismo halo de icono `0 6px 16px color-mix(...)` aparece en **18**, entre ellos `StudentsScreen.tsx:1006`, `TeachersScreen.tsx:143`, `AnnouncementsScreen.tsx:139`, `ScholarshipsScreen.tsx:89`, `ActivitiesScreen.tsx:92`, `FinanceScreen.tsx:372`, `FinanceHeader.tsx:31`, `UniformsScreen.tsx:164` y `Dashboard.tsx:55`, que no están en la lista del plan. Los botones flotantes (`StudentsScreen.tsx:1156`, `ExpensesScreen.tsx:182`, `ScholarshipsScreen.tsx:119`, `ActivitiesScreen.tsx:121`), `Controls.tsx:89` (botón primario), `Brand.tsx:38`, `HelpTip.tsx:245` y `ChargeRow.tsx:105` tampoco. Las franjas `inset 3px 0 0` (tablas) y los anillos `0 0 0 3px` (puntos de estado, campo inválido en `Field.tsx:111`) son semánticos y **deben quedarse**.
- **Enmienda (§3.3):** "Tres familias, no una lista de archivos: (a) halo de icono de pantalla → `--shadow-icon` (18 archivos); (b) sombras de elevación (FAB, popover, botón primario) → `--shadow-pop` / `--shadow-fab`; (c) franjas `inset` y anillos de estado → se quedan. Una prueba `grep` en Vitest (como `contratos.test.ts`) falla si aparece un `boxShadow` con px fuera de (c)." Mejor aún: extraer un `<ScreenIcon color>` compartido. Hay 18 copias idénticas del mismo `div` de 56px, y el rubro de código compartido se cumple: misma conducta, menos riesgo.

### 11. [IMPORTANTE] (8/10) `MODULES` tiene módulos visibles que el §3.1 no agrupa

- **Evidencia:** `data.ts:81` `billing` y `data.ts:82` `reports` no son `hidden` y no tienen área en §3.1. `documents` (`data.ts:74`) sí es `hidden`.
- **Enmienda (§3.1):** "`group` es **obligatorio** en `ModuleDef` (el compilador obliga a cubrir los módulos nuevos). `billing` → Dinero, `reports` → Sistema, `documents` → Alumnos y familias. Test: todo módulo no oculto tiene grupo."

### 12. [MENOR] (9/10) "Nuevo aviso" ya abre el formulario; F1 no cambia nada ahí

- **Evidencia:** `AnnouncementsScreen.tsx:133-170` es sólo el formulario (`<form>` con Título y Mensaje); no hay lista ni cajón. `Dashboard.tsx:361` ya hace `onOpen("announcements")`.
- **Enmienda (§2 B3 y §1 F1):** "B3 no cambia. Opcional: el título recibe `autoFocus` cuando se llega desde 'Nuevo aviso' (intent `compose`, sólo enfoque)."

### 13. [MENOR] (9/10) "Ninguna pantalla recibe intent salvo `Finance.tsx`" es inexacto

- **Evidencia:** `ScheduleEntry.tsx:24` toma `takeCoverFocus` y `ScheduleScreen.tsx:45,48` abre en "Por cubrir": eso ya es un intent, con otro nombre.
- **Enmienda (§5):** citar `coverFocus.ts` como el precedente que F1 generaliza.

### 14. [MENOR] (8/10) El registro del flag en compose contradice la convención de `module_config_setup`

- **Plan:** §8 "+ la lista `ENABLED_MODULES` del compose local".
- **Evidencia:** `docker-compose.yml:126-127`: `config_setup` **no** está en la lista por defecto; se enciende con `ENABLED_MODULES=<lista>,config_setup`. `FLAGS.md:38` documenta eso.
- **Enmienda (§8):** "No se agrega a la lista por defecto (default OFF). Se documenta el comando para encenderlo en local, y las pruebas lo fuerzan reescribiendo `/api/v1/features` con `page.route`, como `config-setup.flag.spec.ts:20-27`. Ese mismo mecanismo cubre 'flag ON y OFF' en §4.3 y en `narrow.mobile.spec.ts`." Recordatorio de la memoria `feature-flags-env-not-db`: apagarlo en dev es un deploy del gateway, así que el plan de reversa son ~6 min, no un clic.

### 15. [MENOR] (8/10) Las capturas "con datos" de F0 chocan con el `globalSetup` que vacía la base

- **Evidencia:** memoria `e2e-demo-project-wipes-db`: cualquier proyecto de Playwright sin `E2E_SKIP_RESET=1` vacía la base. `responsive` corre dentro de `pnpm e2e` (`package.json:13`).
- **Enmienda (§6.1):** "Capturas 'antes/después con datos': proyecto propio, `E2E_SKIP_RESET=1` y base sembrada (`pnpm demo:ready`). La matriz responsive corre sobre base vacía, así que los KPI salen en cero y eso cubre §3.4."

### 16. [MENOR] (8/10) Las llaves `home.*` no las cubre `registry.i18n.test`

- **Evidencia:** `registry.i18n.test.ts:15-26` sólo recorre el namespace `palette`.
- **Enmienda (§3.1):** "Test hermano `home.i18n.test.ts`: cada `group` y cada `desc` de módulo visible tiene llave no vacía en ES y EN."

---

## Tabla verificada de intents (§5)

Leyenda: **EXISTE** = el cajón o la pestaña ya están y sólo falta el intent · **POR DEFECTO** = ya es lo primero que pinta el módulo; no necesita intent · **TRABAJO** = hay que agregar enfoque o mover el destino · **NO EXISTE** = no se puede abrir sin un paso previo.

| Módulo | Acceso del plan | Estado | Evidencia | Qué hacer |
|---|---|---|---|---|
| Estudiantes | Inscribir alumno | **EXISTE** (se llama "Nuevo alumno") | `StudentsScreen.tsx:797` `add`, `:1156` FAB, `:1158` `<QuickAdd>` | intent `new` → `setAdd(true)`. Renombrar a "Nuevo alumno": inscribir a un grupo es un cajón por alumno (`:588-735`) |
| Estudiantes | Buscar alumno | **TRABAJO** | buscador del filtro compartido, `searchHint` `:1036`, sin ref ni autofocus | intent `search` → enfocar; requiere que el filtro exponga `autoFocus`/ref. Alternativa: dejarlo a ⌘K |
| Avisos | Nuevo aviso | **POR DEFECTO** | `AnnouncementsScreen.tsx:133-170`: la pantalla es el formulario | sin intent (opcional: enfocar Título) |
| Actividades | Nueva actividad | **EXISTE** | `ActivitiesScreen.tsx:22` `Mode`, `:82` `create`, `:99` (sólo `activities:write`) | intent `new` → `mode = {kind:"form"}`; filtrar por `activities:write` |
| Asistencia | Pasar lista de hoy | **POR DEFECTO** | `DirectorAttendance.tsx:86` `view="list"`; `TeacherAttendance` sólo lista | sin intent |
| Calificaciones | Capturar calificaciones | **POR DEFECTO** | `DirectorCapture.tsx:39` `section="capture"`; `TeacherCapture` sólo captura | sin intent |
| Horarios | Registrar suplencia | **EXISTE** (se llama "Registrar ausencia") | `ScheduleScreen.tsx:49` `absenceTarget`, `:92` botón (sólo `manage` + `canWrite`), `:122` `<AbsenceDrawer>` | intent `absence` → `setAbsenceTarget({kind:"new"})`; texto "Registrar ausencia"; filtrar por escritura |
| Docentes | Alta de docente | **TRABAJO** | `TeachersPanel.tsx:104` formulario en línea "Agregar profesor", siempre visible; no hay cajón | intent `new` → enfocar el primer campo (requiere ref) |
| Registro de pagos | Cobrar a un alumno | **POR DEFECTO** | `PaymentsScreen.tsx:133` buscador con `autoFocus`; `CollectDrawer` exige alumno (`:166`) | sin intent; **mover** `create.payment` del registro de `finance` a `payments` |
| Finanzas | Ver adeudos | **EXISTE** (pestaña) | `FinanceScreen.tsx:253` `tab` con `"overdue"`, `:356` "Vencidos" | intent `overdue` → pestaña inicial "Vencidos" |
| Finanzas | Generar colegiaturas | **NO EXISTE en Finanzas** | sólo en Configuración: `TuitionPanel.tsx:142-146,252` "Generar cargos del mes" (`config:write`); `GeneratePanel` sólo en el huérfano `Finance.tsx:10,111` | quitar de Finanzas y del KPI en cero (§3.4), o apuntar a Configuración → Colegiaturas (decisión del dueño) |
| Becas | Asignar beca | **NO EXISTE como acceso directo** | `ScholarshipsScreen.tsx:36,67`: `ScholarshipAssignPage` exige una beca elegida | sustituir por "Nueva beca": `:79` `setEditing({})` → `ScholarshipDrawer` `:125` (**EXISTE**, `scholarships:write`) |
| Inventario | Vender artículo | **POR DEFECTO** | `UniformsScreen.tsx:45` `tab="counter"` (Mostrador) | sin intent |
| Inventario | Existencias | **EXISTE** (pestaña) | `UniformsScreen.tsx:16,140` `"inventory"` | intent `inventory` → pestaña inicial |
| Egresos | Registrar gasto | **EXISTE** (se llama "Registrar egreso") | `ExpensesScreen.tsx:52` `drawer`, `:137` botón (sólo `expenses:write`), `:189` `<ExpenseDrawer>` | intent `new` → `setDrawer({})`; filtrar por `expenses:write` |

**Totales:** 5 EXISTE (+1 sustituto: Nueva beca) · 6 POR DEFECTO · 3 TRABAJO (Buscar alumno, Alta de docente, y mover el cobro a `payments`) · 2 NO EXISTE (Generar colegiaturas en Finanzas, Asignar beca). Intents reales que F1 tiene que construir: **7** (`students:new`, `activities:new`, `schedule:absence`, `finance:overdue`, `scholarships:new`, `uniforms:inventory`, `expenses:new`), más 2 opcionales de sólo enfoque (`students:search`, `teachers:new`). Eso baja F1 de "3–4 h" a algo más cercano a 2–3 h.

**Tipado recomendado** (auto-decidido, ver D3): `type IntentOf = { students: "new" | "search"; finance: "overdue"; … }` y `ActionSpec` genérico sobre `moduleId`, para que el compilador rechace un intent que la pantalla no entiende. Así se evita que el registro y las pantallas vuelvan a desincronizarse (el defecto D3 del buscador).

---

## ¿Debe el intent ir en la URL?

**Auto-decidido: no; traspaso de un solo uso en memoria** (hallazgo 1). Razones: (a) §5.3 exige "se consume una vez", y en la URL F5 y "atrás" reabrirían el cajón, a veces con un formulario a medias; (b) los cajones no son lugares a los que se quiera regresar, y la app ya decidió eso para otros traspasos (`coverFocus.ts`, `pendingNotice.ts`); (c) no agrega rutas que `pathToView` tenga que validar. Lo que sí es estado navegable (pestaña "Vencidos", filtros) ya tiene precedente en la URL (`/alumnos?nivel=`, `StudentsScreen.tsx:112-130`). Si el dueño quiere un enlace compartible a "Finanzas vencidos", la pestaña puede vivir en la URL en otro PR.

## ¿Un solo flag para la piel global y el inicio?

**Sólido, con una condición.** Ambas cosas son sólo presentación (no protegen rutas, `FLAGS.md` lo exige así), el dueño lo pidió en D3 y dos flags harían que la matriz tuviera 4 estados en vez de 2. El costo es la granularidad de la reversa: si la piel rompe una pantalla (hallazgo 6), apagarla también apaga el inicio nuevo, y apagar es un deploy. **Condición:** en el código, la piel (`data-look`) y la variante del inicio leen el flag por separado, cada una en un solo sitio, para que partirlo en dos flags después sea un cambio de una línea. Registrar esto en `FLAGS.md`.

---

## Diagrama: flujo del intent después de las enmiendas

```
 ⌘K comando / peek de tarjeta / botón hero
              │  route "module:<id>[:<intent>]"
              ▼
 AppShell.navigate ──► open(id, intent)
                          │ 1. requestIntent(screen, intent)   (módulo, un solo uso)
                          │ 2. go({screen})  → router.push(path)
                          ▼
           Next vuelve a montar <AppShell>  (el estado anterior muere)
                          ▼
 Pantalla destino:  useState(() => takeIntent(screen))
       ├─ "new"      → abre el cajón existente   (si tiene permiso de escritura)
       ├─ "overdue"  → pestaña inicial
       └─ undefined  → vista por defecto (F5, atrás, enlace directo)
```

## Diagrama de pruebas (qué cubre cada capa)

```
 Unit (Vitest/jsdom)                      E2E (Playwright, navegador real)
 ─────────────────────                    ─────────────────────────────────
 pendingIntent: take-once, por pantalla   intent de punta a punta (≥1, p.ej. egresos:new)
 registry: 8+1 rutas reescritas,          palette.spec: ejecutar comando abre cajón
   tipado IntentOf, filtro por escritura  responsive: matriz §4.3 ON/OFF (page.route)
 cada pantalla: intent → cajón/pestaña,   config-setup a <900px con ui_refresh ON
   sin intent → vista por defecto         ⋯ táctil con precondición matchMedia
 Dashboard ON/OFF: B1–B12 + grupos,       narrow.mobile ON
   ≤4 sin títulos, área vacía, peek       capturas antes/después (E2E_SKIP_RESET=1)
 LookAttribute: sigue al flag, gl-look
 home.i18n: ES/EN completos
 grep-test: sin boxShadow literal fuera de (c)
```

Regresiones obligatorias (regla de la casa): una para el hallazgo 1 (intent sobrevive al remontaje) y una para el 4 (rol de sólo lectura no ve accesos de escritura).

## Modos de falla

| Falla | ¿Test? | ¿Manejo? | ¿Visible para el usuario? | Crítico |
|---|---|---|---|---|
| Intent perdido por remontaje | sólo si hay e2e | con la enmienda 1 | sí: el botón "no hace nada" | **sí** |
| Acceso de escritura a rol de lectura | con la enmienda 4 | filtro `requires` | sí: botón sin efecto | **sí** |
| Barra flotante tapa "Paso N de 8" | con la enmienda 6 | `--topbar-h` | sí, en la demo diaria | **sí** |
| Parpadeo de piel al arrancar | no automatizable fácil | `gl-look` previo al pintado | sí, leve | no |
| Pantalla no refluye a 320px | modo informe | backlog | sí, sólo con 400% | no |
| `backdrop-filter` lento en iPad | no | `@supports` + medición en `/qa` | sí | no |

**Fallas críticas señaladas: 3.**

## NO en alcance

- Capa B del buscador (registros de alumnos): sigue en `PLAN-command-palette-v2.md`.
- Corregir el resto de la deriva del registro (`documents`, `billing`, `reports`, `settings.create.teacher` que debería ir a `teachers`). Se anota; F1 sólo toca los intents que usa el inicio, más el traslado del cobro.
- Reflujo a 320px de los 10 módulos no-inicio (backlog, hallazgo 8).
- Partir el flag en dos.

## Lo que ya existe y el plan debe reutilizar

`coverFocus.ts` / `pendingNotice.ts` (traspaso de un solo uso) · `desbordeLateral` (`responsive.spec.ts:31`) · forzar flag por `page.route` (`config-setup.flag.spec.ts:20-27`) · script previo al pintado de `gl-theme` (`layout.tsx:60-66`) · `canReachView` para las rutas · `QuickAdd`, `ExpenseDrawer`, `ScholarshipDrawer`, `AbsenceDrawer` tal cual.

## Decisiones auto-tomadas (sesión sin dueño disponible)

- **D1 Alcance:** se acepta tal cual (FULL_REVIEW), con la matriz de los 10 módulos reducida a modo informe de 300 a 400% (hallazgo 8). Recomendación aplicada.
- **D2 Transporte del intent:** traspaso en memoria de un solo uso, no en la URL.
- **D3 Tipado:** `IntentOf` por módulo, para que el compilador detecte la deriva.
- **D4 Flag:** uno solo, con lectura separada para la piel y el inicio.
- **D5 Cobro:** `create.payment` pasa a `module:payments` sin intent; se borra `Finance.tsx`, que está huérfano.
- **D6 Voz externa (Outside Voice):** omitida; Codex no está instalado en esta máquina (memoria `palette-three-defects-2026-09`) y esta sesión es un subagente.

## Decisiones para el dueño

1. **"Generar colegiaturas"** no existe en Finanzas; vive en Configuración → Colegiaturas y exige `config:write`. ¿Se quita del peek y del KPI en cero (recomendado), o el acceso lleva a Configuración?
2. **"Asignar beca"** necesita elegir una beca primero. ¿Se sustituye por "Nueva beca" (recomendado)?
3. **Zoom 300–400% en los otros 10 módulos:** ¿modo informe y backlog (recomendado), o bloqueante en este PR? Bloqueante probablemente duplica F4.
4. **Accesos que son la vista por defecto** (Asistencia, Calificaciones, Avisos, Cobrar, Vender): ¿se muestran en el peek como atajo con texto (apuntan al módulo) o sólo la tarjeta? Recomendado: sólo la tarjeta, y el peek nada más donde abre algo distinto.
5. **Un flag o dos:** se mantiene uno (D3 del dueño). Confirmar que apagarlo en dev es un deploy del gateway (~6 min), no un clic.

## Tareas de implementación (derivadas)

| # | Prioridad | Tarea | Hallazgo |
|---|---|---|---|
| T1 | P1 | `pendingIntent.ts` + `open()` + e2e de un intent | 1 |
| T2 | P1 | Mover `create.payment` a `payments`; borrar `Finance.tsx` | 2 |
| T3 | P1 | Reemplazar la tabla §5 por la verificada; 7 intents + `IntentOf` | 3 |
| T4 | P1 | `requires` de escritura en las acciones y en el peek | 4 |
| T5 | P2 | Reescribir las 8+1 aserciones; una prueba nueva en `palette.spec` | 5 |
| T6 | P1 | TopBar a clases + `--topbar-h`, acoplado a la puesta en marcha | 6 |
| T7 | P2 | `<LookAttribute>` en el provider + `gl-look` previo al pintado | 7 |
| T8 | P2 | Matriz: inicio bloqueante; resto en modo informe por encima de 200%; DSF; precondición táctil | 8, 9 |
| T9 | P2 | `--shadow-icon` / `<ScreenIcon>` + grep-test | 10 |
| T10 | P2 | `group` obligatorio + `home.i18n.test` | 11, 16 |
| T11 | P3 | Flag fuera del compose por defecto; `page.route` en las suites | 14 |

Paralelización: T1–T5 son el PR 1 (secuencial, comparten `registry.ts` y `AppShell.tsx`). T6, T7, T9 y T10 tocan archivos distintos y pueden ir en paralelo dentro del PR 2. T8 va al final.

## Completion summary

- Paso 0 (alcance): aceptado tal cual, con un recorte en la matriz (D1).
- Arquitectura: 4 hallazgos (1, 6, 7, 2) · Calidad de código: 5 (3, 4, 10, 11, 13) · Pruebas: 5 (5, 8, 9, 15, 16) · Rendimiento: 0 nuevos (el `backdrop-filter` ya está en los riesgos del plan) · Otros: 2 (12, 14).
- Fallas críticas: 3 · Voz externa: omitida · Decisiones sin resolver: 5 (del dueño).

## GSTACK REVIEW REPORT

| Review | Trigger | Why | Runs | Status | Findings |
|--------|---------|-----|------|--------|----------|
| CEO Review | `/plan-ceo-review` | Scope & strategy | 0 | — | — |
| Outside Review | none (codex not installed) | Independent 2nd opinion | 0 | skipped | — |
| Eng Review | `/plan-eng-review` | Architecture & tests (required) | 1 | ISSUES OPEN | 14 issues, 3 critical gaps |
| Design Review | `/plan-design-review` | UI/UX gaps | 1 (file only, not in log) | ISSUES OPEN | 2 bloqueantes, 8 importantes (ver design-review.md) |
| DX Review | `/plan-devex-review` | Developer experience gaps | 0 | — | — |

- **OUTSIDE COVERAGE:** codex, plan-review, skipped (not installed), 0 findings.
- **VERDICT:** ninguna revisión está CLEAR. Hay que aplicar las enmiendas 1, 2 y 6 antes de construir; eng review required (re-run after amendments).

**UNRESOLVED DECISIONS:**
- Generar colegiaturas: quitar del peek y del KPI en cero, o llevar a Configuración
- Asignar beca → Nueva beca
- Zoom 300–400% en los otros 10 módulos: modo informe o bloqueante
- Accesos que son la vista por defecto: en el peek o sólo la tarjeta
- Un flag (reversa = deploy del gateway): confirmar
