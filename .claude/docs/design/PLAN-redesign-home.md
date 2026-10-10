# Plan — Rediseño del Inicio (opción C) + estilo "Captions adaptado" en toda la app

Estado: **borrador para aprobación** · 2026-10-10 · rama `feature/redesign-home` (desde `dev`)
Vista previa aprobada: https://claude.ai/artifact/W1gxk8oLEgu8dCs9iusedw (dirección C + estilo Captions adaptado)

## 0. Decisiones del dueño (2026-10-10)

| # | Decisión |
|---|---|
| D1 | Dirección **C**: módulos agrupados por área, tarjetas, accesos directos al pasar el cursor. |
| D2 | Estilo **Captions adaptado** (grano, barra flotante esmerilada, bordes finos, halo verde, íconos sobrios por área, serif solo en el saludo). Sin cian, sin titulares de 48px, sin espacios de 160px. |
| D3 | El estilo se aplica a **toda la app de una vez**, no solo al inicio. |
| D4 | **Arreglar el intent** (defecto D2 del superbuscador): cada acceso directo abre su formulario, no solo el módulo. Es un cambio de comportamiento y va **primero, en su propio PR**. |
| D5 | En pantallas táctiles, los accesos se abren con un **botón ⋯ de 44px**; tocar la tarjeta sigue abriendo el módulo. |
| D6 | **Tablet y escritorio, todos los tamaños y todos los zooms.** Teléfono fuera de alcance (ver §4.1: el zoom obliga igual a que el layout aguante 320px). |

## 1. Fases y PRs

| Fase | PR | Qué | Comportamiento | Flag |
|---|---|---|---|---|
| F0 | — | Línea base: capturas "antes", tests de caracterización del inicio | no cambia | — |
| F1 | PR 1 | Intents: los accesos de ⌘K y de "Nuevo aviso" abren el flujo pedido | **cambia** (D4) | ninguno: es arreglo de defecto |
| F2 | PR 2 | Capa de estilo global (tokens + barra superior) | no cambia | `module_ui_refresh` |
| F3 | PR 2 | Inicio C (grupos, tarjetas, peek, ⋯, KPIs en cero) | no cambia (usa F1) | `module_ui_refresh` |
| F4 | PR 2 | Matriz responsive/zoom en Playwright + `/qa` visible + `/design-review` | — | ON y OFF |

F2 y F3 comparten flag porque el dueño quiere el cambio completo de una vez (D3). Con el flag OFF, la app queda **idéntica a hoy**.

## 2. Inventario de comportamiento del inicio (el contrato que no se toca)

Fuente: `apps/web/app/_screens/Dashboard.tsx`, `Dashboard.test.tsx` (13), `Dashboard.regression-1.test.tsx` (4).

| # | Comportamiento hoy | Dónde vive en C |
|---|---|---|
| B1 | Saludo por hora (días/tardes/noches) + nombre real | Hero, en serif |
| B2 | Fecha `es-MX` capitalizada + "· Ciclo X" solo si se conoce el ciclo | Hero, igual |
| B3 | "Nuevo aviso" solo con `comms:read`, abre `announcements` | Hero; con F1 abre el formulario de aviso nuevo |
| B4 | KPI Alumnos solo con `students:read` → abre `students`; excluye rechazados/bajas | Fila KPI, igual |
| B5 | KPIs Por cobrar / Cobrado solo con `finance:read` (+ `config:read` para leer cargos) → abren `finance` | Fila KPI; en cero muestra el siguiente paso (§3.4) |
| B6 | Sin lecturas antes de resolver permisos; maestro no lee alumnos/ciclos/cargos | Igual (lógica intacta) |
| B7 | Módulos visibles = `isModuleVisible(m, permissions)` (flag + permiso; ocultos fuera) | Igual; solo cambia el **acomodo** |
| B8 | Esqueleto mientras `permissions === undefined` | Esqueleto con forma de grupos |
| B9 | `permissionsFailed` → alerta + Reintentar | Igual |
| B10 | Cero módulos → "No tienes módulos asignados" | Igual |
| B11 | Clic en tarjeta → `onOpen(id)` | Igual (toda la tarjeta, salvo ⋯ y los accesos) |
| B12 | Maestro ve exactamente Asistencia, Calificaciones, Horarios | Igual, **sin títulos de área** (≤4 módulos) |

Todo test existente sigue verde con el flag ON y OFF. Los que dependan del orden de la cuadrícula se parametrizan, no se reescriben.

## 3. Diseño

### 3.1 Áreas (nuevo campo `group` en `MODULES`, `_design/data.ts`)

| Área | Color | Módulos, en orden de uso diario |
|---|---|---|
| Alumnos y familias | `--forest` | Estudiantes, Avisos, Actividades |
| Académico | `--purple` | Asistencia, Calificaciones, Horarios, Docentes |
| Dinero | `--orange` | Registro de pagos, Finanzas, Becas, Inventario, Egresos |
| Sistema | neutro | Configuración, Administración (sección compacta al final) |

- Un área sin módulos visibles no se pinta. Con ≤4 módulos visibles no hay títulos de área.
- Las descripciones se reescriben con verbos (copy aprobado en la vista previa) y pasan por `humanizer`. **Necesitan llaves i18n ES/EN**: hoy `Dashboard` pinta `m.label`/`m.desc` en duro y la app tiene botón EN. Se agregan `home.group.*` y `home.desc.*` al catálogo existente y se cubren con `registry.i18n.test`.
- Colores por área sustituyen al color por módulo **solo en el inicio con el flag ON**. El buscador ⌘K conserva `module.color` (fuera de alcance).

### 3.2 Tarjeta con accesos (peek)

Estructura accesible (sin botones anidados):

```
<article class="home-card" data-group="dinero">
  <button class="home-card__main" aria-describedby="desc-x">  ← cubre la tarjeta (stretched button), onOpen(id)
  <p id="desc-x">…</p>
  <button class="home-card__more" aria-expanded aria-controls="peek-x" aria-label="Accesos de Finanzas">⋯</button>
  <ul id="peek-x" class="home-card__peek"> <li><button>Cobrar a un alumno →</button></li> … </ul>
</article>
```

- **Puntero fino con hover** (`@media (hover:hover) and (pointer:fine)`): el peek sube al pasar el cursor, con 120 ms de retardo al entrar y 80 ms al salir (evita parpadeos al cruzar la cuadrícula). El ⋯ queda oculto.
- **Táctil o híbrido** (`@media (any-pointer:coarse)`): el ⋯ de 44×44 px es visible. Abre y cierra el peek; tocar fuera o Esc lo cierra. Solo un peek abierto a la vez. iPad con trackpad recibe ambos.
- **Teclado**: Tab llega a la tarjeta y luego al ⋯; `:focus-within` muestra el peek y Esc lo cierra y devuelve el foco al ⋯.
- Accesos por módulo: 2 o 3, **solo los que F1 deja funcionando de verdad**. Un módulo sin intent probado no lleva peek, solo la tarjeta.
- Movimiento: `transform` + `opacity`, 200 ms con `--ease`. Con `prefers-reduced-motion` el peek aparece sin desplazarse.

### 3.3 Capa de estilo global (`html[data-look="refresh"]`)

`AppShell` pone `data-look="refresh"` en `<html>` cuando `useFlag("module_ui_refresh")`. Todo el estilo nuevo vive en `globals.css` bajo ese selector, en claro y oscuro. Sin el atributo, nada cambia.

| Token / pieza | Hoy | Con el flag |
|---|---|---|
| `--bg` / fondo | `#F5F7F5` + degradado | blanco papel + grano SVG (opacidad 0.05–0.07) + lavado verde arriba |
| `--shadow-card` | sombra doble | `none`; borde de 1px `--hair` |
| `--shadow-hover` | sombra oscura | halo `0 14px 44px rgba(122,196,0,.20)` (oscuro: `.13`) |
| `--r-lg` (tarjetas) | 22px | 16px |
| Barra superior | pegada, 78px, borde inferior | cápsula flotante a 12px de los bordes, radio 28px, desenfoque 28px + saturación, borde fino |
| Saludo | Poppins 800 | Libre Baskerville 400 (`next/font`, solo ese uso) |
| Íconos de módulo | cuadro de color sólido | cuadro tintado al 13% + ícono del color del área |

- 141 usos de tokens se actualizan solos. **10 archivos** con `boxShadow` literal pasan a tokens antes de encender (lista: AttendanceHeader, QrScanDialog, PaymentsScreen, TeacherCapture, DirectorCapture, ScheduleScreen, Catalog, SetupHeader, ExpensesScreen, AdminScreen).
- Lima queda reservado para lo primario y lo seleccionado (regla "un acento" de Captions).

### 3.4 KPIs en cero

Con el flag ON y el valor en cero: "Aún no hay cargos este ciclo" + "Ir a Finanzas →" (mismo `onOpen("finance")` de hoy). "Generar colegiaturas" como destino directo **solo si F1 deja ese intent probado**; si no, el texto no lo promete.

## 4. Responsive y zoom (D6)

### 4.1 Qué significa "todos los zooms" sin teléfono

El zoom del navegador reduce el ancho CSS: 1280 px al 200% son 640 px, y al 400% son **320 px**. WCAG 2.2 · 1.4.10 (Reflow) pide que a 320 px no haya scroll horizontal ni contenido perdido. Así que el layout **tiene que aguantar 320 px** aunque nadie lo use en un teléfono. Lo que sí queda fuera: navegación pensada para teléfono, gestos y pruebas en dispositivos de teléfono.

### 4.2 Reglas

- Cuadrícula: `repeat(auto-fill, minmax(min(100%, 232px), 1fr))`, sin número fijo de columnas. Las áreas se apilan una debajo de otra.
- Tarjetas: alto según contenido (sin alto fijo, para que el texto al 200% no se corte). El peek se superpone dentro de la tarjeta; si su contenido es más alto que la tarjeta, la tarjeta crece (`min-height` calculado con `max-content`, no `overflow:hidden` sobre texto).
- Barra superior flotante: `position: sticky` **solo si `min-height: 560px`**. Más bajo (tablet acostada con zoom, 1366×768 al 150%) pasa a estática, para no tapar el foco (WCAG 2.4.11) ni comerse la pantalla.
- Por debajo de 720 px la barra esconde EN/tema/correo (ya existe ese patrón) y la búsqueda se vuelve ícono.
- `backdrop-filter` con `@supports not` → fondo opaco. El grano es `background-image` normal (sin `fixed`, que da tirones en iPad).
- Texto: tamaños en `rem`/`clamp`, sin `px` fijos en contenedores de texto. WCAG 1.4.4 (200%) y 1.4.12 (espaciado de texto) cubiertos.
- La suite `narrow.mobile.spec.ts` (Inventario de pie, en teléfono) debe seguir verde con el flag ON.

### 4.3 Matriz de prueba (Playwright, flag ON y OFF)

| Caso | Viewport CSS |
|---|---|
| Tablet vertical | 768×1024, 820×1180 |
| Tablet acostada | 1024×768, 1180×820, 1366×1024 |
| Laptop | 1280×800, 1440×900 |
| Escritorio | 1920×1080, 2560×1440 |
| Zoom 150 / 200 / 300 / 400% sobre 1280×800 | 853×533, 640×400, 427×267, 320×200 |
| Zoom 50 / 67% sobre 1920 | 3840×2160, 2880×1620 |

Aserciones por caso: (1) sin desborde lateral (`scrollWidth − clientWidth ≤ 1`, reutiliza `desbordeLateral` de `responsive.spec.ts`); (2) ningún título ni descripción de tarjeta cortado (`scrollWidth ≤ clientWidth` por elemento); (3) el elemento con foco nunca queda debajo de la barra; (4) los accesos se alcanzan por hover (puntero fino) y por ⋯ (`hasTouch: true`); (5) capturas a `.gstack/redesign/home/after/`.

## 5. F1 — Intents (cambio de comportamiento, PR propio)

Hoy `AppShell.open(id, _intent)` descarta el intent, y `registry.test.ts` lo **afirma como correcto en 17 lugares**. Ninguna pantalla recibe intent salvo `Finance.tsx` (huérfano).

1. Test rojo: `navigate("module:finance:payment")` llega al formulario de cobro. Luego, uno por cada intent de la lista.
2. `open(id, intent)` pasa el intent a la pantalla; cada pantalla destino recibe `intent?: <Unión de literales>` y abre su cajón o pestaña **existente** en el primer render.
3. El intent se consume una vez: volver al inicio y entrar de nuevo no reabre el cajón.
4. Se reescriben las 17 aserciones de `registry.test.ts`.

Candidatos (cada uno se confirma leyendo la pantalla; si el cajón o la pestaña no existe, se cae de la lista y no se inventa):

| Módulo | Accesos candidatos |
|---|---|
| Estudiantes | Inscribir alumno · Buscar alumno |
| Avisos | Nuevo aviso (también el botón del hero) |
| Actividades | Nueva actividad |
| Asistencia | Pasar lista de hoy |
| Calificaciones | Capturar calificaciones |
| Horarios | Registrar suplencia (`AbsenceDrawer`) |
| Docentes | Alta de docente |
| Registro de pagos | Cobrar a un alumno |
| Finanzas | Ver adeudos · Generar colegiaturas |
| Becas | Asignar beca (`ScholarshipAssignPage`) |
| Inventario | Vender artículo (`CounterSalePanel`) · Existencias |
| Egresos | Registrar gasto (`ExpenseDrawer`) |

Los mismos intents alimentan ⌘K, así que el buscador por fin cumple "mínimo de clics".

## 6. Orden TDD

1. F0: caracterización del acomodo actual (flag OFF) + capturas "antes" en los 4 estados (con datos, cargando, permisos fallidos, sin módulos).
2. F1: un test rojo por intent → implementación → refactor. PR 1, `/review`, `/qa` del buscador y "Nuevo aviso".
3. F2: test de que `data-look` sigue al flag; tokens bajo el selector; migración de los 10 `boxShadow` literales (sin cambio visual con OFF).
4. F3: tests de componente, primero en rojo: agrupación y orden; ≤4 sin títulos; área vacía omitida; peek por hover, ⋯ y teclado; Esc devuelve el foco; un solo peek abierto; KPI en cero; i18n EN.
5. F4: matriz §4.3; `/qa` en el navegador visible con base local vacía (reglas qa-browser); `/design-review`; `/review-animations`.

## 7. Riesgos

- **dev es la demo diaria**: flag OFF por defecto. Se enciende en dev solo tras las compuertas tech-lead + security y con visto bueno del dueño.
- **D3 toca todas las pantallas** a la vez: la matriz §4.3 corre sobre los 11 módulos de `responsive.spec.ts`, no solo sobre el inicio.
- **`backdrop-filter` en iPads viejos**: cae a opaco con `@supports`, y se mide el FPS al hacer scroll en `/qa`.
- **Hover en tarjetas pegadas**: el retardo de 120/80 ms evita que se abran peeks al pasar el ratón por encima de la cuadrícula.
- **F1 cambia lo que hace ⌘K**: es el comportamiento que el dueño pidió en septiembre, pero los e2e del palette (`palette.spec.ts`) cambian sus expectativas.

## 8. Flag

`module_ui_refresh` — se registra en `.claude/docs/FLAGS.md` (fila nueva, mismo formato que `module_config_setup`) + la lista `ENABLED_MODULES` del compose local. Solo presentación; no protege ninguna ruta. **Retiro:** cuando el dueño acepte, se borran el estilo viejo, el flag y los tests que solo cubren la vista vieja.

## 9. Estimación honesta (tiempo de agente)

| Fase | Estimado |
|---|---|
| F0 | ~45 min |
| F1 | 3–4 h (depende de cuántas pantallas tengan el cajón listo) |
| F2 | ~2.5 h |
| F3 | ~3.5 h |
| F4 | ~2 h + correcciones que salgan |

## 10. Pendiente antes de construir

- [ ] `/plan-design-review` y `/plan-eng-review` sobre este plan.
- [ ] Aprobación del dueño (checkpoint 2).
