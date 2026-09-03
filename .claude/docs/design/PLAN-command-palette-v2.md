<!-- /autoplan restore point: /Users/andresromo/.gstack/projects/romodevx-garcia-lorca/dev-autoplan-restore-20260902-180541.md -->

# PLAN — Superbuscador (Command Palette) v2 · reparar y completar

> Sucede a `PLAN-command-palette.md` (CP-1, entregado con defectos). Infra cross-cutting,
> NO un módulo de negocio. Memoria: [[command-palette]] · [[ux-minimum-clicks]] · [[design-system-mandate]].
> Estado: **PLAN — en revisión /autoplan.**

## El problema, medido

El palette existe, tiene 141 tests en verde, y **el usuario dice que "no hace nada".**
Las dos cosas son ciertas. Tres defectos independientes, cada uno suficiente por sí solo
para que la función se sienta muerta.

### D1 — El diálogo es invisible (0 líneas de CSS)

`CommandPalette.tsx:83` monta `Command.Dialog` de `cmdk`.

- `cmdk@1.1.1` envía **0 archivos CSS** (verificado en `node_modules`; es unstyled por diseño).
- El repo tiene **0 reglas** para `[cmdk-*]`: `apps/web/app/globals.css` no lo menciona, y es
  la única hoja de estilos del proyecto.

El portal se monta sin posición, sin fondo y sin `z-index`. **El palette abre correctamente
y no se ve.** Clic en la caja del TopBar → `onOpenPalette()` → `setPaletteOpen(true)` →
diálogo invisible. Eso es exactamente el síntoma reportado.

### D2 — El `intent` se recibe y se tira (el "mínimo de clics" no existe)

`AppShell.tsx:234`:

```ts
const open = useCallback((id: string, _intent?: string): void => {
```

El guion bajo es la convención de "parámetro deliberadamente ignorado". La cadena está
muerta en cinco eslabones:

| Eslabón | Estado |
|---|---|
| `registry.ts` emite `module:finance:payment` | ✅ |
| `navigate()` parsea el intent (`AppShell.tsx:283`) | ✅ |
| `open(id, _intent)` lo descarta | ❌ |
| El tipo `View` no tiene campo de intent | ❌ |
| `<FinanceScreen onBack={home} />` no recibe `intent` (`AppShell.tsx:309`) | ❌ |
| `Finance.tsx:33` **sí** implementa `intent?: FinanceIntent` | ✅ pero muerto |

Resultado: los 20 comandos aterrizan en la vista por defecto de su módulo. "Crear alumno"
abre la lista de Estudiantes, no el formulario. **Aun con el CSS de D1, el palette sería un
cambiador de módulos lento — no un superbuscador.** `Finance.tsx` es la única pantalla que
implementa intent, y nunca lo recibe.

### D3 — El registro se desfasó de los módulos reales

`ACTIONS` en `registry.ts` se escribió contra la lista de módulos de junio. Hoy desfasa
en ambas direcciones:

| Deriva | Detalle |
|---|---|
| Módulos vivos SIN comandos | `teachers` (Docentes), `payments` (Registro de Pagos) |
| Comandos hacia módulos muertos | `documents` (`hidden: true`), `scholarships` (sin backend), `billing` (fuera de MVP) |
| Comando al módulo equivocado | `create.payment` → `module:finance:payment`, pero la ventanilla es hoy el módulo `payments` |
| Acción bajo el módulo equivocado | `create.teacher` vive bajo `settings`, existiendo ya el módulo `teachers` |

### Por qué los tests no lo vieron

141 tests verdes en `app/command` y `packages/ui/src/command`. Corren en **jsdom, que no
aplica CSS**: `Command.Dialog` está en el DOM, luego "se renderiza". D1 es invisible para
la suite por construcción. D2 y D3 tampoco se cubren porque ningún test recorre
`registry → navigate → open → screen` de punta a punta: se prueba el registro y se prueba
el matcher, nunca el trayecto.

**Ese es el hueco real: no hay una sola prueba que verifique que un comando aterriza donde promete.**

---

## Alcance

Tres etapas. CP-1.5 es la que devuelve la función prometida; CP-2 es la que cumple la
mitad del spec que nunca se construyó.

### CP-1.5 — Reparar y completar la Capa A (la función que ya se pagó)

**Must:**
1. **CSS del palette** siguiendo el design system (`.claude/docs/design/DESIGN-SYSTEM.md`):
   overlay, diálogo centrado, `z-index` sobre el TopBar, grupos, item activo, iconos con
   su color de módulo, atajo visible, responsive móvil (hoja inferior), `prefers-reduced-motion`.
2. **Cablear el intent de punta a punta:** `View` lleva `intent`, `open()` lo propaga,
   cada pantalla que declare un intent lo recibe como prop.
3. **Intents reales por pantalla** — cada acción `create`/`quick` aterriza en su formulario
   o vista, no en la portada del módulo.
4. **Resincronizar el registro** con `MODULES` de hoy: alta de `teachers` y `payments`,
   baja de los muertos, `create.payment` → `module:payments:new`.
5. **Guarda contra deriva:** test que falla si un `ACTIONS.moduleId` no existe en `MODULES`,
   o si un módulo visible no aporta al menos su comando `nav`.
6. **Test de trayecto E2E (Playwright, navegador real):** ⌘K abre algo **visible**, se escribe,
   Enter, y la pantalla destino es la correcta. Es la única clase de prueba que atrapa D1.

**Should:** contador de resultados y estado vacío con sugerencia; `aria-live` en el conteo.

### CP-2 — Capa B: registros (la mitad que nunca se construyó)

Hoy **no existe forma de buscar un alumno por nombre desde el servidor**:
`ListStudentsRequest` (proto, línea 155) sólo tiene `level`, `status`, `grade_group`,
`page_size`, `page_token`. **No hay campo de texto.** Sin esto, `"Juan Pérez"` es imposible.

**Must:**
1. **Contrato:** campo `query` en `ListStudentsRequest` (+ `buf lint`/`breaking`).
2. **Índice de búsqueda propio** en Postgres (`pg_trgm` sobre nombre + CURP + correo del
   tutor), con RLS, alimentado por los eventos que el servicio ya emite. **No federación:
   los módulos no exponen `Search` RPC** — decisión ya tomada en el plan v1.
3. **Ruta de gateway** `/api/v1/search?q=` con permisos del principal, cota dura de resultados
   y rate-limit.
4. **Debounce + cancelación** en el cliente; el historial guarda **sólo IDs de comando**,
   jamás datos de registro.
5. Comandos de registro: `Juan Pérez` → perfil; `4to A` → grupo.

**Should:** quick-actions sobre el registro encontrado ("Juan → adeudos", "Juan → asistencia"),
apoyadas en rutas que ya existen (`/api/v1/finance/accounts/{id}`, `/api/v1/family/students/{id}/*`).

**Could:** `morosos` (ya existe `/api/v1/finance/overdue`), `adeudo enero`, info-queries
(`total estudiantes`, `ingresos junio`).

**Won't (ahora):** `$128,300` como búsqueda (requiere índice sobre montos y no resuelve un
trabajo real: nadie busca por importe exacto), búsqueda semántica/NL, voz.

---

## Catálogo, corregido contra los módulos de HOY

Fuente: `apps/web/app/_design/data.ts`. Se marcan las diferencias con el spec del dueño.

| # | Módulo (hoy) | Flag | Comandos | Nota vs. spec |
|---|---|---|---|---|
| 1 | Estudiantes | `module_students` | nav, crear, inscribir, trasladar | — |
| 2 | **Docentes** | `module_teachers` | nav, crear docente, asignar materia | **Nuevo, no estaba en el spec** |
| 3 | **Registro de Pagos** | `module_finance` | nav, registrar pago | **Nuevo**; aquí vive la ventanilla |
| 4 | Finanzas | `module_finance` | nav, adeudos, morosos | — |
| 5 | Asistencia | `module_attendance` | nav, tomar asistencia de hoy | QR diferido; es pase de lista |
| 6 | Calificaciones | `module_grades` | nav, capturar, crear evaluación | — |
| 7 | Avisos | `module_comms` | nav, crear aviso | — |
| 8 | **Inventario** | `module_uniforms` | nav, pedido, talla | Ya no es sólo uniformes: libros y artículos |
| 9 | Reportes | `module_reports` | nav, indicadores | — |
| 10 | Configuración | `module_config` | nav, ciclo, nivel, grupo | — |
| 11 | **Administración** | `module_iam` | nav, crear usuario, crear rol | **Nuevo, no estaba en el spec** |
| — | ~~Documentos~~ | `module_docs` | — | **Desactivado** (`hidden: true`) |
| — | ~~Becas~~ | `module_scholarships` | — | **Sin backend**; permiso que nadie tiene |
| — | ~~Facturación~~ | `module_billing` | — | **Fuera de MVP** (stub de proto) |

Tres módulos del spec no existen hoy y tres módulos de hoy no están en el spec.
El registro debe seguir a `MODULES`, no al spec de junio — y la guarda de deriva (CP-1.5 #5)
es lo que impide que se vuelva a separar.

---

## Seguridad (se mantiene lo decidido en v1)

- Las acciones **navegan a un formulario prellenado; jamás mutan**. Un comando nunca escribe.
- El filtrado por flag+permiso es **UX**; el gateway (403/404) sigue siendo la frontera.
- **Deuda conocida, sin cerrar:** v1 decidió ensamblar el registro **server-side (RSC)** por
  usuario+flags+rol. Lo construido es `"use client"` (`CommandPaletteHost.tsx:1`), así que
  **el mapa completo de acciones viaja en el bundle**. No filtra datos de alumnos, pero sí
  revela qué módulos y acciones existen. Se decide en la revisión: cerrarlo o registrarlo.
- CP-2: la búsqueda de registros se filtra server-side por IAM+RLS, sin fuga por conteo ni
  por tiempo de respuesta; rate-limit en `/api/v1/search`.

## Tests

```
[CP-1.5]
  ├── E2E navegador real: ⌘K → visible (altura/posición/contraste)   [TEST · atrapa D1]
  ├── E2E: escribir "crear alumno" → Enter → formulario abierto      [TEST · atrapa D2]
  ├── trayecto: cada comando del registro aterriza en su destino     [TEST · atrapa D2]
  ├── guarda de deriva: ACTIONS.moduleId ⊆ MODULES                   [TEST · atrapa D3]
  ├── guarda de cobertura: módulo visible ⇒ tiene comando nav        [TEST · atrapa D3]
  └── responsive móvil + reduced-motion                              [TEST]
[CP-2]
  ├── búsqueda de alumno por nombre, acento plegado                  [TEST]
  ├── aislamiento RLS: un rol no ve lo que no le toca                [TEST]
  ├── sin fuga por conteo ni por timing                              [TEST]
  ├── debounce + cancelación de la petición en vuelo                 [TEST]
  └── historial guarda sólo IDs, nunca datos de registro             [TEST]
```

## Lo que NO entra

- Reescribir el palette: el core (matching, folding, recientes, i18n, flags) está bien
  construido y probado. El defecto es de entrega, no de diseño.
- QR de asistencia (sigue diferido).
- Módulos sin backend (Becas, Documentos, Facturación): sus comandos aparecen solos el día
  que su flag y su permiso existan.

---

# REVISIÓN /autoplan

## FASE 1 — CEO (estrategia y alcance) · modo SELECTIVE EXPANSION

### 0A · Reto a las premisas

| # | Premisa del plan | Veredicto | Evidencia |
|---|---|---|---|
| P1 | "El core está bien construido; el defecto es de entrega, no de diseño" | **Parcialmente falsa** | 141 tests cubren registro/matcher/folding/recientes y `cmdk` es librería probada — eso sostiene la premisa. Pero el ensamblado **client-side** (`CommandPaletteHost.tsx:1` `"use client"`) contradice la decisión de v1 de ensamblar server-side. Eso es un defecto de **diseño**, no de entrega. La premisa se acepta con esa corrección. |
| P2 | "El personal escolar usará un palette de teclado" | **Asumida, no validada** | Riesgo real de gusto-de-ingeniero. Mitigante: el disparador del TopBar es un **clic**, no sólo ⌘K, así que la función no depende de que nadie memorice un atajo. Se acepta la premisa **por el clic**, no por el teclado. |
| P3 | "Buscar registros exige índice nuevo (proto + `pg_trgm` + ruta `/api/v1/search`)" | **FALSA — y es el hallazgo más caro del plan** | Ver 0B. |

**P3, en detalle.** `services/gateway/internal/client/students.go:103` ya **drena todas las
páginas** y devuelve el padrón completo (cota 20k). Su propio comentario lo dice: *"The web
resolves student names from the full roster."* Y **seis pantallas ya lo consumen**
(`Dashboard.tsx:250`, `PaymentsScreen.tsx:52`, `GradeCapture.tsx:58`, `DirectorCapture.tsx:51`,
`usePassList.ts:129`, `AnnouncementsScreen.tsx:62`). El `Student` que llega ya trae
`firstName`, `lastName`, `curp`, `gradeGroup`, `guardianName` — exactamente los campos que
el spec pide buscar.

Con 401 alumnos, **el padrón ya está en el navegador**. Buscar "Juan Pérez" es alimentar esa
lista al `matchCommands` que ya existe y ya está probado, como comandos de tipo `record`.

> **Cero cambios de proto. Cero migración. Cero ruta nueva. Cero trabajo de RLS.**
> El permiso ya lo aplica el endpoint existente (`students:read`).

### 0B · Mapa de apalancamiento (qué ya existe)

| Sub-problema | Ya existe | Falta |
|---|---|---|
| Fuzzy con folding de acentos | `packages/ui/src/command/match.ts`, `fold.ts` (probados) | nada |
| Agrupar/ordenar por tipo | `KIND_ORDER`, tipo `record` **ya definido y sin usar** | poblarlo |
| Padrón de alumnos en cliente | `listStudents()` — 6 pantallas lo usan | reusarlo en el palette |
| Teclado/foco/ARIA | `cmdk` | **CSS (D1)** |
| Recientes | `recents.ts` (sólo IDs) | nada |
| i18n es+en | `packages/i18n/.../palette.json`, 60+ claves | claves de los módulos nuevos |
| E2E en navegador real | `apps/web/e2e` + proyecto `demo` (sólo lectura, base sembrada) y `mobile` | el spec del palette |
| Deep-link por intent | `Finance.tsx:33` lo implementa | **propagarlo (D2)** |

El proyecto `demo` de Playwright es sólo-lectura contra base sembrada. El palette **navega y
nunca muta**, así que su E2E cae ahí de forma natural, sin tocar datos ni el flujo de demo.

### 0C · Estado soñado

```
HOY            palette invisible (D1) · intent tirado (D2) · registro desfasado (D3)
               → el buscador es decorativo

ESTE PLAN      ⌘K/clic abre algo visible · cada comando aterriza en su formulario
               · el registro sigue a MODULES por prueba · "Juan Pérez" resuelve
               contra el padrón que ya está en memoria
               → "todo a un buscador" cumplido para navegación, ejecución y alumnos

IDEAL 12M      registros de más dominios (pagos, calificaciones, avisos) conforme
               cada módulo los emita · comandos contextuales (sabe en qué pantalla
               estás) · índice server-side SÓLO si el colegio pasa de ~2 mil alumnos
```

**Delta:** este plan deja el palette cumpliendo el north star para el 100 % de la navegación,
el 100 % de las acciones de creación y el tipo de registro que importa (alumnos). Lo que no
cubre — registros de otros dominios — es **aditivo**, no un rediseño.

### 0C-bis · Alternativas de implementación para la Capa B

| # | Enfoque | Esfuerzo (humano / CC) | Riesgo | Veredicto |
|---|---|---|---|---|
| A | **Padrón en cliente** + `matchCommands` existente | ~1 d / **~30 min** | Techo ~2k alumnos; sólo alumnos | **ELEGIDA** (P3 pragmático, P4 DRY, P5 explícito) |
| B | Proto `query` + `pg_trgm` + `/api/v1/search` (el plan original) | ~5-8 d / ~3 h | Migración, RLS, rate-limit, fuga por timing — superficie de seguridad nueva | Rechazada **hoy**; es la salida si crecen |
| C | Federación: cada servicio expone `Search` RPC | ~10 d+ / ~5 h | Ya rechazada en v1 como el mayor riesgo | Rechazada |

La elección de A **no cierra la puerta a B**: el contrato del palette es el mismo
(`kind: "record"`), así que cambiar la fuente de datos más adelante no toca la UI.

### 0D · Decisiones de alcance (SELECTIVE EXPANSION)

| Decisión | Resolución | Principio |
|---|---|---|
| CSS del palette (D1) | **APROBADO** — sin esto no hay función | P1 completitud |
| Cablear intent (D2) | **APROBADO** — 1 archivo + props; es el north star | P2 radio de impacto |
| Resincronizar registro (D3) | **APROBADO** — dato, no arquitectura | P3 pragmático |
| Guarda de deriva por test | **APROBADO** — impide que D3 vuelva | P1 completitud |
| E2E en navegador real | **APROBADO** — única clase que atrapa D1; infra ya existe | P1 completitud |
| Capa B por padrón en cliente | **APROBADO** — sustituye a CP-2 | P3 + P4 + P5 |
| Proto `query` + índice `pg_trgm` | **DIFERIDO a TODOS.md** — se activa si pasan ~2k alumnos | P3 pragmático |
| Registro server-side (RSC) | **TASTE DECISION** — ver gate | P5 vs P1 |
| `$128,300` como búsqueda | **RECHAZADO** — ya estaba en "Won't"; nadie busca por importe exacto | P3 |

### 0E · Interrogatorio temporal

- **HORA 1:** CSS puesto. El palette **se ve**. El dueño ya percibe que "sirve". Es el
  arreglo de mayor retorno del plan y va primero.
- **HORA 2-3:** intent cableado. "Crear alumno" abre el formulario. El north star se cumple.
- **HORA 4:** registro resincronizado + guardas. Docentes y Registro de Pagos aparecen.
- **HORA 5-6:** alumnos como registros. "Juan Pérez" resuelve.
- **HORA 6+:** E2E en navegador real. **Sin esto el defecto vuelve** — la suite en jsdom
  seguirá verde mientras el palette esté invisible.

### 0F · Modo confirmado
**SELECTIVE EXPANSION.** Radio de impacto: `AppShell.tsx`, `registry.ts`, `globals.css`,
`CommandPaletteHost.tsx`, + un E2E. Todo dentro de < 1 día CC.

---

## FASE 3 — ENG (arquitectura, pruebas, modos de falla)

### Arquitectura: dónde se rompe la cadena

```
  TopBar (clic)  ────┐
  ⌘K / Ctrl+K   ────┤
                     ▼
              setPaletteOpen(true)          AppShell.tsx:95
                     │
                     ▼
        CommandPaletteHost  ── flags + permisos ──> buildCommandRegistry()
                     │                                    registry.ts:96
                     ▼
        CommandPalette ──> matchCommands (fold+fuzzy)  packages/ui/.../match.ts
                     │
                     ▼
            cmdk Command.Dialog  ◄══════ ✗ D1: 0 reglas CSS ⇒ INVISIBLE
                     │
                     ▼ onSelect
              navigate("module:payments:new")   AppShell.tsx:283  ✅ parsea
                     │
                     ▼
              open(id, _intent)  ◄════════════ ✗ D2: descarta el intent
                     │
                     ▼
              setView({screen})  ◄──────────── ✗ D2: View no tiene intent
                     │
                     ▼
              <Screen onBack/>   ◄──────────── ✗ D2: no recibe la prop
```

Dos cortes independientes. D1 mata la percepción; D2 mata el valor.

### La suite codifica el defecto

`app/command/registry.test.ts` **asserta las rutas sin intent como correctas**:

```
:69  settings.create.cycle    → "module:settings"   // "All route back to the config surface"
:88  students.create.enroll   → "module:students"   // "Both route to the students surface"
:106 grades.create.capture    → "module:grades"
:126 admin.create.user        → "module:admin"
```

Y `:139` afirma *"the create shortcut opens it ON the payment form"* — cierto en el registro,
**falso en la app**, porque `open()` tira el intent tres capas más abajo.

> **Consecuencia para el trabajo:** arreglar D2 **no es sólo añadir tests, es reescribir
> aserciones existentes.** 17 de 20 comandos tienen su ruta sin-intent congelada en verde.
> Esto sube el costo de la tarea 2 y hay que decirlo por adelantado.

### Cobertura: qué prueba qué

```
                        unit(jsdom)  trayecto  E2E(navegador)
 registro/flags/permisos    ✅          —           —
 fuzzy + folding            ✅          —           —
 recientes (sólo IDs)       ✅          —           —
 el diálogo SE VE           ✗           ✗          ← ÚNICO lugar donde D1 muere
 comando → pantalla         ✗          ←            ← ÚNICO lugar donde D2 muere
 registro ⊆ MODULES         ✗          ←  guarda de deriva mata D3
```

jsdom no aplica CSS: **ninguna cantidad de tests unitarios puede atrapar D1.** Por eso el E2E
en navegador real no es un "nice to have", es la única red bajo el defecto que ya nos pasó.

### Registro de modos de falla

| # | Modo de falla | Hoy | Detección | Mitigación del plan |
|---|---|---|---|---|
| F1 | Palette invisible | **ACTIVO** | ninguna | CSS + E2E de visibilidad |
| F2 | Comando aterriza en la portada, no en el formulario | **ACTIVO** | ninguna (la suite lo bendice) | cablear intent + reescribir aserciones + test de trayecto |
| F3 | Módulo nuevo sin comandos | **ACTIVO** (`teachers`, `payments`) | ninguna | guarda de cobertura |
| F4 | Comando apunta a módulo muerto | **ACTIVO** (`documents`, `scholarships`, `billing`) | ninguna | guarda de deriva |
| F5 | Padrón grande vuelve lento el palette | latente (401 hoy) | ninguna | cota de resultados + medir; salida = índice server-side |
| F6 | El mapa de acciones viaja en el bundle | **ACTIVO** | ninguna | TASTE DECISION (ver gate) |
| F7 | El padrón se re-descarga en cada apertura | sería nuevo | ninguna | cachear en memoria del shell (ya hay 6 llamadas duplicadas) |

**Brecha crítica:** F1 y F2 están activos hoy, sin ninguna detección, en la función que el
dueño enseña a diario. F3 y F4 ya ocurrieron y nadie se enteró.

### Nota sobre F7 (deuda que este plan revela, no crea)

Seis pantallas llaman `listStudents(API_URL)` por separado (`Dashboard.tsx:250`,
`PaymentsScreen.tsx:52`, `GradeCapture.tsx:58`, `DirectorCapture.tsx:51`, `usePassList.ts:129`,
`AnnouncementsScreen.tsx:62`). Cada una descarga el padrón completo. El palette sería la
séptima. Cachear una vez en el shell sirve a las siete y **quita seis descargas**, no añade una.
Está en el radio de impacto y es < 1 día CC ⇒ **aprobado por P2**.

---

## TAREAS (radio de impacto, orden de ejecución)

Orden por retorno: T1 devuelve la percepción de que "sirve" en la primera hora;
T2 devuelve el valor real; T5/T6 impiden que los tres defectos vuelvan.

| # | Tarea | Archivos | Mata | Esfuerzo (humano / CC) |
|---|---|---|---|---|
| **T1** | **CSS del palette.** Selectores verificados en `cmdk@1.1.1`: `[cmdk-overlay]`, `[cmdk-dialog]`, `[cmdk-root]`, `[cmdk-input]`, `[cmdk-list]`, `[cmdk-group]`, `[cmdk-group-heading]`, `[cmdk-item]`, `[cmdk-item][data-selected="true"]`, `[cmdk-empty]`. Tokens existentes (`--r-lg`, `--shadow-pop`, `--surface`) **y sus variantes oscuras** — `globals.css:87` ya define el tema oscuro y el palette debe respetarlo. | `globals.css` | **D1** | ~0.5 d / **~20 min** |
| **T2** | **Cablear el intent.** `View` gana `intent?: string`; `open(id, intent)` deja de tirarlo; cada pantalla con intent lo recibe como prop. **Incluye reescribir las 17 aserciones de `registry.test.ts` que congelan la ruta sin intent** (`:69`, `:88`, `:106`, `:126`, …). | `AppShell.tsx`, `registry.test.ts` | **D2** | ~1 d / **~40 min** |
| **T3** | **Intents por pantalla.** Cada `create`/`quick` aterriza en su formulario. `Finance.tsx:33` ya lo implementa y sirve de patrón; el resto abre su drawer/panel de alta. | pantallas de `_screens/` | **D2** | ~1 d / **~45 min** |
| **T4** | **Resincronizar el registro.** Alta de `teachers` y `payments`; baja de `documents`/`scholarships`/`billing`; `create.payment` → `module:payments:new`; `create.teacher` sale de `settings` al módulo `teachers`. Claves i18n es+en. | `registry.ts`, `palette.json` ×2 | **D3** | ~0.5 d / **~25 min** |
| **T5** | **Guardas por prueba.** (a) todo `ACTIONS.moduleId` existe en `MODULES`; (b) todo módulo visible aporta su `nav`; (c) ninguna acción apunta a módulo `hidden`. | `registry.test.ts` | **D3** | ~0.25 d / **~15 min** |
| **T6** | **E2E en navegador real** (proyecto `demo`, sólo lectura): ⌘K abre algo con caja visible y contraste; escribir "crear alumno" + Enter abre el formulario. Variante `mobile` para la hoja inferior. | `e2e/palette.spec.ts`, `e2e/palette.mobile.spec.ts` | **D1+D2** | ~0.5 d / **~30 min** |
| **T7** | **Alumnos como registros.** Reusar `listStudents()` + `matchCommands`; `kind: "record"` ya existe en `KIND_ORDER` sin usar. Cachear el padrón en el shell (**quita 6 descargas duplicadas**, no añade una). Sin cambios de backend. | `CommandPaletteHost.tsx`, shell | Capa B | ~1 d / **~40 min** |
| **T8** | **Quick-actions de registro** ("Juan → adeudos", "Juan → asistencia") sobre rutas existentes. | `registry.ts` | Should | ~0.5 d / **~25 min** |

**Total: ~5.25 días humano / ~4 h CC.** T1-T6 (reparar lo pagado) son ~3.75 d / ~3 h.

### Diferido a TODOS.md
- Proto `query` + índice `pg_trgm` + `/api/v1/search`: **se activa si el colegio pasa de ~2 mil alumnos.** Hoy son 401 y el padrón ya viaja completo al cliente.
- Registros de otros dominios (pagos, calificaciones, avisos) conforme cada módulo los emita.
- `$128,300`, info-queries, comandos contextuales.

### Riesgo de afinación en T7 (el ranking dominado por tipo)

`match.ts:35` ordena **primero por `kind`** y sólo después por score textual. Para comandos
es correcto y es literalmente la prioridad que pidió el dueño (`KIND_ORDER` =
`module < create < recent < record < quick`, y `DEFAULT_LIMIT = 8` ya cumple "máx 8-10").

El riesgo aparece al meter alumnos como `record`: **cualquier módulo o acción que coincida
parcialmente con el texto sepulta a un alumno con coincidencia exacta.** "Juan" funciona
porque ningún módulo coincide. Un alumno llamado "Ana" compite contra cualquier comando que
contenga "ana".

**Mitigación en T7:** cuando el score textual de un `record` es de **subcadena exacta** y el
del comando es sólo subsecuencia, el registro sube. Es un ajuste dentro de `match.ts` con su
propio test; **no** un rediseño del ranking.

---

## FASE 2.5 — DX (el contrato que usan los autores de módulos)

### Por qué falló el DoD del palette

El plan v1 fijó: *"un módulo NO está terminado hasta que registró su nav, sus create y sus
quick."* Falló para `teachers` y `payments`. **No fue descuido de proceso: es la forma de la API.**

`registry.ts:104`:

```ts
const module = moduleById.get(action.moduleId);
if (module === undefined || !allowed(module)) continue;   // ← salta en SILENCIO
```

Un `moduleId` inexistente o con typo **no produce error, ni aviso en consola, ni test rojo**.
El comando simplemente no aparece. Y el caso inverso — un módulo nuevo que nunca se agrega a
`ACTIONS` — tampoco tiene quien lo reclame, porque `ACTIONS` es una lista central que nadie
está obligado a tocar al crear un módulo.

**Diagnóstico:** la deriva de D3 no es un fallo de disciplina, es el comportamiento diseñado.
Una lista central + salto silencioso = deriva garantizada con el tiempo.

### Costo de entrada para un módulo nuevo

Hoy, agregar un módulo y que aparezca en el palette toca **4 archivos en 3 paquetes**:
`_design/data.ts` (el módulo), `command/registry.ts` (sus acciones), `i18n/es/palette.json` y
`i18n/en/palette.json` (sus textos). Ninguno de los cuatro te reclama si olvidas los otros tres.

### Recomendación

**T5 (guardas por prueba) es el mínimo correcto y entra ya.** Convierte el fallo silencioso en
un test rojo, que es exactamente la señal que faltó.

Hacerlo **imposible por construcción** — que cada módulo declare sus propios comandos junto a
su definición, y que `ACTIONS` se derive de ahí — es la solución estructural, pero es un
refactor que toca todos los módulos y **queda fuera del radio de impacto de este plan**.
Se difiere a TODOS.md con disparador explícito: **si la guarda de T5 se pone roja dos veces
por deriva real, se hace el refactor.**

### Rutas stringly-typed

`route: "module:payments:new"` se parsea con `split(":")` (`AppShell.tsx:283`). Un typo en el
intent tampoco falla: cae al `undefined` y la pantalla abre en su vista por defecto — **el
mismo modo de falla silenciosa que D2.** T2 debe tipar el intent por módulo (unión de literales,
como el `FinanceIntent` que `Finance.tsx:19` ya define) en lugar de `string` suelto.

---

## Voces duales — degradación

| Voz | Estado |
|---|---|
| Codex (las 4 fases) | `[codex-unavailable]` — binario no instalado (preflight `CODEX=missing-binary`) |
| Subagente Claude (CEO, Diseño, DX) | `[failed]` — los 3 subagentes terminaron en `idle` sin entregar reporte; peticiones de seguimiento sin respuesta |

**Modo: single-reviewer.** Todo hallazgo de este documento está sostenido por evidencia
directa del código (archivo:línea) o por ejecución verificada, no por consenso entre modelos.
**No hay tabla de consenso** porque no hubo segunda voz — inventarla sería peor que no tenerla.

**Lo que esto cuesta:** las preguntas que una voz independiente debía retar quedan sin retar,
y son justamente las de producto, no las de código:
1. ¿Es un palette de teclado el modelo correcto para una secretaria escolar, o es gusto de
   ingeniero? (mitigante verificado: el disparador del TopBar es **clic**, no sólo ⌘K)
2. ¿La afirmación "el core está bien, sólo falla la entrega" es auto-complaciente?
   (evidencia a favor: 141 tests sobre matcher/folding/registro; evidencia en contra: el
   ensamblado client-side contradice la decisión de v1)
3. ¿La densidad de 8 resultados con iconos, grupos y subtítulos en 520 px es legible?

**Estas tres son juicio del dueño y viajan al gate sin resolver.**

---

## Decision Audit Trail

| # | Fase | Decisión | Clase | Principio | Razón |
|---|---|---|---|---|---|
| 1 | CEO | CSS del palette (T1) entra | Mecánica | P1 | Sin esto no hay función; es el defecto reportado |
| 2 | CEO | Cablear intent (T2/T3) entra | Mecánica | P2 | Es el north star "mínimo de clics"; radio de impacto chico |
| 3 | CEO | Capa B por padrón en cliente, no por índice server-side | **Taste** | P3+P4+P5 | El gateway ya drena el padrón completo y 6 pantallas ya lo consumen; **revierte una decisión aprobada en v1** |
| 4 | CEO | Proto `query` + `pg_trgm` + `/api/v1/search` → diferido | Mecánica | P3 | 401 alumnos; disparador explícito ~2 mil |
| 5 | CEO | `$128,300` como búsqueda → rechazado | Mecánica | P3 | Ya estaba en "Won't" de v1; no resuelve un trabajo real |
| 6 | ENG | Reescribir 17 aserciones de `registry.test.ts` | Mecánica | P1 | La suite congela el defecto; no se puede arreglar D2 sin tocarlas |
| 7 | ENG | E2E en navegador real entra | Mecánica | P1 | Única clase de prueba que atrapa D1; infra ya existe |
| 8 | ENG | Cachear el padrón en el shell (F7) | Mecánica | P2 | En radio de impacto, < 1 d CC, **quita** 6 descargas |
| 9 | DX | Guardas por prueba (T5) entra; refactor estructural difiere | **Taste** | P3+P5 | El test convierte el fallo silencioso en rojo; el refactor toca todos los módulos |
| 10 | DX | Tipar el intent por módulo, no `string` suelto | Mecánica | P5 | Mismo modo de falla silenciosa que D2 |
| 11 | ENG | Ensamblado server-side (RSC) del registro | **Taste** | P5 vs P1 | Deuda de v1 sin cerrar; ver gate |
| 12 | CEO | Registro sigue a `MODULES`, no al spec de junio | **Reto al dueño** | — | 3 módulos del spec no existen; 3 módulos vivos no están en el spec |

## GSTACK REVIEW REPORT

- **Modo:** SELECTIVE EXPANSION · **Fases:** CEO ✅ · Diseño ⚠️ (voz caída) · DX ✅ · ENG ✅
- **Voces duales:** `[codex-unavailable]` + `[subagent-failed]` ⇒ **single-reviewer mode**. Sin tabla de consenso.
- **Defectos con evidencia de código:** 3 (D1 invisible, D2 intent tirado, D3 registro desfasado) + 7 modos de falla, 4 de ellos ACTIVOS hoy sin detección.
- **Hallazgo de mayor impacto:** la Capa B no necesita backend. `gateway/internal/client/students.go:103` ya devuelve el padrón completo y 6 pantallas ya lo consumen ⇒ CP-2 pasa de ~5-8 d a ~1 d.
- **Riesgo principal sin cerrar:** nadie retó de forma independiente si un palette es el modelo correcto para el personal escolar.
- **VERDICT:** listo para implementar T1-T6 (reparar lo pagado). T7-T8 tras la decisión del dueño sobre el gate.

**DECISIONES PENDIENTES: 3 taste + 1 reto al dueño (ver gate).**

---

## ENMIENDA tras el gate (decisiones del dueño, 2026-09-02)

| Pregunta | Decisión | ¿Coincide con la recomendación? |
|---|---|---|
| Capa B | **Padrón en cliente** | Sí |
| Registro RSC | **Cerrarlo en este plan** | **No — override del dueño** |
| Anti-deriva | **Guardas por prueba** | Sí |
| Catálogo | **Seguir los módulos de hoy** | Sí |

### T9 — Ensamblar el registro server-side (RSC) · decisión del dueño

Cierra la deuda de v1 (F6): hoy el mapa completo de acciones viaja en el bundle porque
`CommandPaletteHost.tsx:1` es `"use client"`.

**A favor (verificado):** `layout.tsx` y `page.tsx` **ya son server components**, y
`app/i18n/server.ts:3` ya lee cookies con `next/headers`. El camino existe y tiene precedente
en este repo.

**El costo real, dicho por adelantado.** Es más que mover una función:

1. `page.tsx` debe reenviar la cookie de sesión al gateway y llamar **dos** endpoints que hoy
   se resuelven en cliente: `/api/v1/features` (hoy vía `AppFlagsProvider`) y `/api/v1/me`
   (hoy vía `apiMe`). Ambos ya existen.
2. `buildCommandRegistry()` corre en el servidor; `AppShell` recibe el registro **ya filtrado**
   como prop, en lugar de construirlo.
3. **La fricción de verdad:** la app es una SPA donde el **login ocurre dentro de `AppShell`**.
   En el primer render no hay sesión, así que el registro server-side nace vacío. Al iniciar
   sesión hace falta re-ensamblarlo.
   **Solución:** `router.refresh()` tras un login exitoso — el servidor rearma el registro con
   la cookie ya presente. Es la respuesta idiomática de Next y es pequeña, pero **hay que
   escribirla a propósito y probarla**, o el palette queda vacío justo después de entrar.

**Esfuerzo:** ~1 d humano / **~50 min CC** — no los ~20 min que insinuaba el gate.
**Prueba obligatoria:** tras login, el palette trae los comandos del rol (es el modo de falla
que introduce esta tarea).

> **Recomendación mantenida, decisión respetada:** yo lo habría registrado como deuda porque
> el gateway sigue siendo la frontera real (403/404) y lo que se revela son nombres de acciones,
> no datos de alumnos. El dueño eligió cerrarlo; entra como T9 y **se hace después de T1-T6**,
> para que la reparación de lo que ya está roto no dependa de un cambio de arquitectura.

### Orden de ejecución final

```
T1 CSS ─────────────► el palette SE VE            (~20 min)   ← primero, mayor retorno
T2 intent + tests ──► "crear alumno" abre el form (~40 min)
T3 intents/pantalla ► cada acción aterriza        (~45 min)
T4 registro al día ─► Docentes y Pagos aparecen   (~25 min)
T5 guardas ─────────► la deriva se vuelve roja    (~15 min)
T6 E2E navegador ───► D1 y D2 no pueden volver    (~30 min)
   ── aquí lo pagado ya está reparado y protegido ──
T7 alumnos ─────────► "Juan Pérez" resuelve       (~40 min)
T8 quick-actions ───► "Juan → adeudos"            (~25 min)
T9 RSC ─────────────► el mapa sale del bundle     (~50 min)   ← decisión del dueño
```

**Total ~5 h CC.** T1-T6 = ~3 h.
**Fuera del alcance, con disparador:** índice `pg_trgm` si el colegio pasa de ~2 mil alumnos;
refactor estructural del registro si la guarda de T5 se pone roja dos veces por deriva real.

---

# CORRECCIÓN — las voces duales SÍ llegaron (2026-09-03)

Las tres voces entregaron reporte después de que este documento se diera por cerrado.
**La sección "Voces duales — degradación" queda anulada:** hubo revisión independiente, y
corrigió **tres errores de este plan**. Se dejan ambas secciones para que el rastro sea honesto.

## Errores de ESTE plan, verificados en código

### E1 · `Finance.tsx` es código muerto — T3 es más grande de lo que dije · CRÍTICO

Este plan decía *"`Finance.tsx:33` ya lo implementa y sirve de patrón"*. Falso en lo que importa:

```
AppShell.tsx:12   import { FinanceScreen } from "./_screens/finance/FinanceScreen"
AppShell.tsx:309  <FinanceScreen onBack={home} />
FinanceScreen.tsx:183  interface FinanceScreenProps { onBack: () => void }   ← sin intent
```

`Finance.tsx` y `FinanceScreen.tsx` son **componentes distintos**. A `Finance.tsx` — el único
que implementa `intent?: FinanceIntent` — **no lo importa nadie** salvo su propio test.

> **Consecuencia: CERO pantallas renderizadas aceptan un intent.** No existe implementación de
> referencia. T3 no es "copiar el patrón de Finance", es **crear el patrón desde cero** y
> además decidir qué hacer con `Finance.tsx` (adoptarlo o borrarlo). **Re-estimado: ~1.5 d / ~1 h 15.**

### E2 · Mi guarda de cobertura es una tautología · ALTO

Propuse: *"todo módulo visible aporta su `nav`"*. Los `nav` **se generan** recorriendo `MODULES`
(`registry.ts`: `for (const module of MODULES)`). **El test no puede fallar por construcción.**
Habría quedado verde para siempre y se habría citado como prueba de que la guarda funciona.

### E3 · Mi guarda de deriva atrapa 1 de 4 síntomas · ALTO

Propuse `ACTIONS.moduleId ⊆ MODULES`. **Pasa hoy** con `documents`, `scholarships` y `billing`
todavía registrados: los tres **están** en `MODULES` (`documents` sólo lleva `hidden: true`).
Verificado. Tampoco atrapa `create.payment` apuntando al módulo equivocado.

**Reemplazo (T5 nuevo).** Cerrar por tipos lo que los tipos pueden cerrar:
```ts
export const MODULES = [...] as const satisfies readonly ModuleDef[];
export type ModuleId = (typeof MODULES)[number]["id"];
readonly moduleId: ModuleId;          // un typo ya no compila
if (module === undefined) throw new Error(...)   // el drop deja de ser silencioso
```
Y guardas sobre lo que los tipos no alcanzan: un módulo `hidden` **no** aporta acciones; la
acción abre el módulo que dice abrir.

## Hallazgos nuevos que entran al plan

| # | Hallazgo | Sev | Entra en |
|---|---|---|---|
| N1 | **"Sin resultados" mientras carga el padrón.** El padrón es asíncrono; abrir ⌘K y escribir "Juan" en esa ventana afirma que el niño no existe. **Lo introduce mi propia decisión de Capa B.** | CRÍTICO | **T10 (nueva)** |
| N2 | Sin `max-height`: 10 filas + encabezados ≈ 640 px contra ~600 px útiles en las laptops 1366×768 del colegio. `[cmdk-list]{max-height:min(56vh,380px);overflow-y:auto}`, diálogo a `top:12vh` (centrarlo lo hace saltar), `limit` 10 → 7 | CRÍTICO | T1 |
| N3 | `[data-selected="true"]` **no recibe foco del DOM**, así que el anillo `:focus-visible` de `globals.css:129` nunca dispara: la selección por teclado sería invisible. Nombrar el selector explícitamente | ALTO | T1 |
| N4 | `ui.hint` ("Escribe para navegar o crear") existe en es+en y **no se usa en ningún lado** (verificado). El vacío sólo dice "Sin resultados" | ALTO | T1 |
| N5 | `globals.css:158` oculta `.topbar-search-text` y `.topbar-search-kbd` bajo 768 px: la única superficie que enseña qué escribir desaparece justo donde ⌘K es imposible | ALTO | T1 |
| N6 | **Hoja inferior en móvil: NO.** Este público está en escritorio; el público de teléfono es la app familiar Expo con su propio modal nativo. Diálogo centrado con `inset:8px` bajo 768 px | ALTO | T1 |
| N7 | Sin escala de `z-index`: TopBar es `zIndex:30` inline; "por encima del TopBar" no es especificación. Tokens `--z-topbar/--z-drawer/--z-palette/--z-toast` | MEDIO | T1 |
| N8 | Ruta stringly-typed rompe de 3 formas; `kind !== "module"` es **no-op silencioso** y `types.ts` ya documenta ids de registro `"record:GL-2041"` ⇒ los registros de T7 seleccionarían sin hacer nada. Unión discriminada `{module, intent?}` con `switch` exhaustivo y `never` | ALTO | T2 |
| N9 | Filas irregulares: sólo los `nav` traen `subtitle`, y `<span data-subtitle>` es hermano en línea del título ⇒ se lee como una frase corrida. Rejilla de 2 columnas, `min-height:46px` | MEDIO | T1 |
| N10 | 10 fichas de color a la vez son confeti, no jerarquía. Fondo `--surface-3` y glifo en color del módulo; ficha rellena sólo para `record` | MEDIO | T1 |

### T10 (nueva) · Estado de carga del padrón · CRÍTICO
Mientras `listStudents` está en vuelo, el grupo `record` muestra filas esqueleto y el vacío dice
**"Buscando alumnos…"**, no "Sin resultados". Con test de estado. **Sin esto, mi decisión de
Capa B produce un fallo peor que el bug original: el sistema afirmando que un alumno no existe.**
~20 min CC.

## Re-estimación

| | Antes | Ahora |
|---|---|---|
| T1 CSS | ~20 min | **~50 min** (N2,N3,N4,N5,N6,N7,N9,N10) |
| T2 intent | ~40 min | **~55 min** (N8: unión discriminada) |
| T3 intents/pantalla | ~45 min | **~1 h 15** (E1: no hay patrón que copiar) |
| T5 guardas | ~15 min | **~30 min** (E2,E3: tipos + guardas reales) |
| T10 carga | — | **~20 min** |
| **Total** | ~5 h | **~6 h 45** |

## Reto al dueño — resuelto (2026-09-03)

Las dos voces independientes coincidieron en pedir un cambio a la dirección del dueño.
**El dueño reafirmó su dirección original en ambas.** Queda así:

| Reto de las voces | Decisión del dueño |
|---|---|
| "El modal de teclado es el modelo equivocado para ventanilla; hazlo type-ahead inline" | **Se mantiene el modal.** Es lo pedido y lo ya construido y probado; T1 lo deja presentable. |
| "El dashboard ya pone cada módulo a un clic; ¿hace falta un navegador?" | **Se sigue completo.** Navegar ya lo resuelve el dashboard; **el valor del palette son las acciones y los registros** — T2/T3/T7, justo lo que nunca funcionó. |

Las objeciones de usabilidad que **sí** entran, porque no dependen del modal:
N4 (usar `ui.hint` en el vacío), N5 (no esconder el texto guía en móvil), N6 (diálogo centrado
con `inset:8px`, no hoja inferior), N2 (`max-height` + `top:12vh`), N3 (selector de fila activa).

## Estado final

- **Alcance:** T1-T10, ~6 h 45 CC. T1-T6 (reparar lo pagado) ≈ ~4 h.
- **Orden:** T1 → T2 → T3 → T4 → T5 → T6 ‖ T10 → T7 → T8 → T9.
- **Voces:** Codex `[unavailable]`; 3 subagentes Claude **entregaron** y corrigieron 3 errores
  de este plan (E1, E2, E3) + aportaron 10 hallazgos. Consenso CEO↔Diseño en el reto al modal:
  **planteado y rechazado por el dueño.**
- **VERDICT:** APROBADO. Listo para implementar T1.
