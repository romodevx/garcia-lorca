# Plan — Rediseño de Avisos ("Redactar y ver", variante A)

Estado: **v2 APROBADO por el dueño el 2026-10-12** (D3: columna 960 px, foco a la alerta = layout, BC1–BC7 fuera) · 2026-10-12 · rama `feature/redesign-avisos` (desde `dev`)
Insumos: `.gstack/redesign/avisos/behavior.md` (contrato) · `diagnosis.md` (P1–P14) ·
`variants/variant-a.png` (elegida en D2) · `before/` (capturas).

## 0. Restricción dura

Cero cambios de funcionalidad, endpoints, contratos, datos, permisos o reglas. La
pantalla nueva llama **exactamente** lo mismo que la vieja, con los mismos argumentos y
en el mismo orden de validación. Se garantiza por construcción: las dos vistas usan
**un solo hook** con todo el estado y las llamadas (§3.1), y por prueba: la suite de
caracterización corre contra las dos vistas (§5).

## 1. Flag

- Clave **`module_announcements_view`**: sólo presentación, igual que `module_config_setup`.
  OFF por defecto. No protege ninguna ruta; la frontera sigue siendo `module_comms` +
  `comms:read`/`comms:write` en el gateway.
- Gateway: `featureflag.Key("announcements_view")` en `services/gateway/internal/config/flags.go`
  (`AllModuleFlags` 21 → 22) + pruebas en `flags_test.go` y `handler/features_test.go`
  (se reporta en `/features`, default OFF, `comms` no lo enciende).
- `.claude/docs/FLAGS.md`: fila nueva con el mismo formato que `module_config_setup`
  (presentación, retiro como deuda técnica).
- Web: se lee **una vez** en `AppShell` (`useFlag("module_announcements_view")`) y elige
  `AnnouncementsCompose` o `AnnouncementsScreen`. Ningún componente de la pantalla lee el flag.

## 2. Layout nuevo (≥ 1024 px)

```
‹ Volver al inicio
[icono] Avisos / Un mensaje para un alumno, un grupo, un nivel o toda la escuela
┌──────────── redactor (1.55fr) ───────────┬──── resumen (1fr, sticky) ────┐
│ ① ¿A quién va?                           │ ASÍ LO LEEN LOS PAPÁS          │
│ [Un alumno][✓ Grupos][Niveles][Toda…]    │ ┌ notificación (título/cuerpo)┐│
│ Grupos: secciones por nivel              │ └─────────────────────────────┘│
│   PREESCOLAR  1° A  2° A …               │ Va a                           │
│   PRIMARIA   ✓2° A ✓2° B …               │ 2° Primaria · A, 2° Primaria · B│
│ Alumno: buscador + lista de resultados   │ [error role=alert | status]    │
│ 2 seleccionados                          │ [ ✓ Enviar aviso ]  (único lima)│
│ ② ¿Qué les dices?                        │                                │
│ Título *   Mensaje *                     │                                │
└──────────────────────────────────────────┴────────────────────────────────┘
★ Felicitación de cumpleaños · Mensaje automático · 7:00      [Editar texto ⌄]
```

- < 1024 px (tablet vertical): una columna; el resumen va después del redactor y queda
  `position: sticky; bottom` con sólo "Va a …" + Enviar, para que el botón no se pierda (P5).
- Teléfono: fuera de alcance (la web es tablet + escritorio, `responsive.spec.ts`).

## 3. Componentes

### 3.1 Reutilizado / extraído (refactor bajo verde)
| Pieza | Qué es |
|---|---|
| `useAnnouncementComposer()` (nuevo archivo, **extraído** de `AnnouncementsScreen`) | Todo el estado y efectos actuales: cargas A1–A4, ciclo A6, scope/picked/toggle B1–B5, título/cuerpo, `send` D1–D8, llave de idempotencia. Devuelve datos y handlers; no pinta nada. `AnnouncementsScreen` pasa a usarlo sin cambiar su JSX (refactor; la caracterización debe seguir verde). |
| `BirthdayGreeting` | **Sin cambios.** La vista nueva lo envuelve en un `<details>` (§3.2). |
| `TextField`, `HelpTip`, `TextArea` (`_design/form`), `Btn`, `Icon`, `Page`, `ModuleHeaderIcon` | Igual que hoy. `Mensaje` pasa a `TextArea` con `required` (mismo label "Mensaje", el asterisco arregla P9). |
| `matchesSearch` (`_design/textSearch.ts`) | Filtro del buscador de alumno (sin acentos, mayúsculas). |
| `sortStudents`, `formatStudentName` | Igual que hoy. |
| `labelGroups` (`config/api.ts`) | Cambio **aditivo**: `LabelledGroup` gana `levelId` y `ordinal` opcionales tomados de los grados que ya lee. Misma llamada, misma red; ningún lector existente se entera. |

### 3.2 Nuevo (cada uno con prueba que falla primero)
| Componente | Resuelve |
|---|---|
| `audience.ts` · `groupTargetsByLevel(groups, levels)` puro | P2: secciones en el orden de `listLevels`, grados por `ordinal`, grupos por nombre; los que no tienen nivel van al final bajo "Otros". |
| `AudienceTabs` | P7/P11: control segmentado con `aria-pressed`, ✓ en el activo y contraste ≥ 4.5:1; título de paso "¿A quién va?" en display 17px. |
| `GroupedTargets` | P2/P3/P10: secciones por nivel, chips de 44 px, sin `max-height` (la lista crece; nada se corta). Para Niveles: una sola fila. |
| `StudentSearch` | P1: `input type="search"` (label "Buscar alumno"), lista de hasta 8 resultados con su grupo (`gradeGroup`), "N de 401 alumnos"; el elegido queda arriba aunque el texto cambie. Selección única (B5) y `aria-pressed` igual que hoy. |
| `ComposeSummary` (aside sticky) | P4/P5/P6/P13/P14: vista previa "Así lo leen los papás" (título + cuerpo en vivo), "Va a" con los nombres elegidos o "Toda la escuela · ciclo X", el `role=alert` y el `role=status` actuales con el mismo texto, y el **único** botón lima "Enviar aviso" (`type=submit` del mismo `<form>`, por `form=` id). |
| `AutomaticMessages` | P4: `<details>` "Felicitación de cumpleaños · Mensaje automático · 7:00" con botón fantasma "Editar texto"; adentro, `BirthdayGreeting` intacto. Cerrado por defecto. |
| `AnnouncementsCompose.tsx` | La pantalla: compone lo anterior con `useAnnouncementComposer()`. |

Copy nuevo (pasa por `humanizer`): "¿A quién va?", "¿Qué les dices?", "Así lo leen los
papás", "Va a", "Buscar alumno", "N de M alumnos", "Mensaje automático", "Editar texto",
"Elige a quién va dirigido" como pista muda (no error) mientras no hay nadie elegido.
Los mensajes de error y éxito **no cambian** (las pruebas los fijan).

### 3.3 Movimiento
`find-animation-opportunities` decide; candidatos: altura del `<details>` al abrir, la
vista previa al cambiar (sin animar el texto), el ✓ del chip. Todo con
`prefers-reduced-motion: reduce` → sin movimiento. Se construye con `animate` y se
revisa con `/review-animations`.

## 4. Inventario → dónde vive ahora

| behavior.md | Vista nueva |
|---|---|
| A1–A4 cargas, A6 ciclo | `useAnnouncementComposer` (idéntico) |
| A5/A7 felicitación (carga, "Cargando…", error) | `BirthdayGreeting` dentro de `AutomaticMessages` (se monta al montar la pantalla, igual que hoy; `<details>` cerrado no lo desmonta) |
| B1 cuatro audiencias, default Grupos | `AudienceTabs` |
| B2 cambiar audiencia limpia selección | hook (idéntico) |
| B3 orden/labels | `GroupedTargets` (agrupado, mismo label y `aria-label`) · `StudentSearch` (mismo `formatStudentName`) |
| B4 multi + quitar · B5 alumno único · B6 "N seleccionados" | hook + `GroupedTargets`/`StudentSearch`; contador debajo de la lista |
| B7 escuela sin lista | `AudienceTabs` → nada debajo; "Va a: Toda la escuela" |
| C1 título (sin mayúsculas, ayuda) · C2 mensaje (ayuda, 4 renglones) | redactor, mismas ayudas |
| D1–D3 validación, D4 `announce`, D5 éxito, D6 error y llave, D7/D8 | hook (idéntico); mensajes en `ComposeSummary` junto al botón |
| E1–E5 felicitación | `BirthdayGreeting` sin cambios |
| F1 Volver · F2 accesos | igual |

## 5. Pruebas

- **Caracterización contra las dos vistas:** `AnnouncementsScreen.behavior.test.tsx` y
  `AnnouncementsScreen.test.tsx` se parametrizan con `describe.each([Old, New])` (sólo se
  cambia el `render`; las aserciones quedan). Las que buscan por rol/label funcionan en
  ambas; el buscador de alumno exige teclear antes de elegir → helper `pickStudent()` por
  vista.
- Nuevas (fallan primero): `audience.test.ts` (orden por nivel/ordinal/nombre, sin nivel
  al final), `labelGroups` (campos aditivos), `AudienceTabs` (aria-pressed), `StudentSearch`
  (filtra sin acentos, conserva el elegido, máximo 8, conteo), `ComposeSummary` (vista
  previa en vivo, "Va a" por audiencia, un solo botón lima), `AutomaticMessages` (cerrado
  por defecto, `BirthdayGreeting` montado), `AppShell` (flag ON → vista nueva; OFF → vieja).
- Gateway: flag registrado, default OFF.
- `contratos.test.ts` (props muertas) y `look.*` siguen verdes.
- e2e: `announcements.flag.spec.ts` con `/features` reescrito por `page.route` (como
  `config-setup.flag.spec.ts`), flag ON y OFF, con `E2E_SKIP_RESET=1` en local.
- Todo verde con flag ON y OFF; `/qa` visible con flag ON recorre `behavior.md` y llena `after/`.

## 6. Archivos

Nuevos: `_screens/announcements/{useAnnouncementComposer.ts, audience.ts, AudienceTabs.tsx,
GroupedTargets.tsx, StudentSearch.tsx, ComposeSummary.tsx, AutomaticMessages.tsx,
AnnouncementsCompose.tsx}` + sus pruebas · `e2e/announcements.flag.spec.ts`.
Cambian: `AnnouncementsScreen.tsx` (usa el hook), `config/api.ts` (`labelGroups` aditivo),
`AppShell.tsx` (una línea de flag), `globals.css` (sólo si el grid sticky lo pide, bajo
`.announce-*`), `services/gateway/internal/config/flags.go` (+ pruebas), `FLAGS.md`.
No cambian: `comms/api.ts`, `BirthdayGreeting.tsx`, ningún servicio Go fuera del registro
del flag, ningún `.proto`.

## 7. Fuera del rediseño (propuestas de cambio de comportamiento, decide el dueño)

- **BC1** Ignorar el segundo clic mientras se envía (hoy re-envía con la misma llave).
- **BC2** Mostrar cuántas familias recibirán el aviso **antes** de enviarlo (requiere un
  conteo o un `dry-run` en el backend).
- **BC3** Historial de avisos enviados (no existe backend).
- **BC4** Errores por campo en lugar de uno general (cambia el texto de los mensajes).
- **BC5** Botón "Quitar todos" en la selección (acción nueva; hoy se quitan uno por uno).

## 8. Estimación

Humano: ~2 días · CC: ~2–3 h (hook + 6 componentes + pruebas + flag + QA visible).

## 9. Enmiendas v2 (revisiones de diseño y de ingeniería)

Revisiones: `reviews/PLAN-redesign-avisos.design-review.md` (20 hallazgos: 5 altos, 10
medios, 5 bajos) · `reviews/PLAN-redesign-avisos.eng-review.md` (16: 4 altos, 8 medios,
4 bajos). **Donde esta sección contradice §1–§6, manda esta sección.**

### Bloqueantes resueltos
| # | Enmienda |
|---|---|
| E1 | `"module_announcements_view"` se agrega a la unión de claves en `packages/ui/src/flags/types.ts` (si no, `useFlag` no compila). |
| E2/D7 | El resumen es **un solo nodo** a cualquier ancho: en una columna cambia de lugar por CSS, nunca se duplica. Un solo "Enviar aviso", un solo `role=alert`, un solo `role=status`. |
| E3 | Sin `form=`: el `<form>` **es** la rejilla de dos columnas y el resumen vive dentro. `BirthdayGreeting` (que trae su propio `<form>`) queda **fuera**. Prueba nueva: Enter en Título envía. |
| E4 | Prueba "A0" escrita en verde sobre la vista vieja y corrida en las dos: cada carga ocurre exactamente una vez, teclear en el buscador no hace peticiones, y el payload completo de `announce` se compara en las cuatro audiencias. |
| D1 | Enter en "Buscar alumno" no envía (`preventDefault` en ese input). Prueba que falla primero. |
| D2 | El ✓ es un `Icon` con `aria-hidden`; los nombres accesibles ("Grupos", "1° PRIMARIA · A") no cambian. |
| D3/E7 | `labelGroups` agrega `gradeName`, `levelId` y `ordinal` (aditivos). El chip muestra "1° A" bajo su encabezado de nivel y conserva el `aria-label` completo de hoy. Que el nombre accesible no coincida con el texto visible (WCAG 2.5.3) queda como **BC6**. |
| D4 | De la maqueta **no** se construyen ni "Quitar todos" (BC5) ni "texto predeterminado" en la fila de cumpleaños (requiere estado de `BirthdayGreeting`). La fila dice "Mensaje automático · 7:00". |
| D5 | Se conserva `Page variant="reading"` (960 px). La rejilla es `minmax(0,1.55fr) minmax(320px,1fr)` y pasa a una columna con `@container` en `.page-column` por debajo de 860 px. `responsive.spec.ts` no se toca. |

### Medios incorporados
- **E10** El hook expone `chooseScope`, `toggle` y `submit`; la regla B2 (cambiar audiencia limpia la selección) vive sólo en el hook.
- **E16** El flag de vista va dentro de la rama `commsEnabled`. AppShell se prueba con 3 casos: comms OFF, comms ON + vista OFF, ambos ON.
- **E6** El orden de los grupos reutiliza `config/schoolOrder.ts` (nivel → grado → letra). No hay una tercera implementación; si hace falta, se generaliza para leer los campos nuevos de `LabelledGroup`.
- **E9/D9** La pista sin selección dice "Todavía no eliges destinatarios", no el texto del error.
- **D8** "Va a" muestra hasta 3 nombres y luego "y N más".
- **D10/D11** Estados del buscador: vacío (muestra el elegido, si hay), sin coincidencias ("Ningún alumno coincide con «…»"), más de 8 ("Mostrando 8 de N, sigue escribiendo"), el elegido aparece una sola vez y con su grupo. Grupos y niveles: "Cargando…" mientras llegan y "Este ciclo no tiene grupos" si la lista llega vacía.
- **D12** La alerta queda junto al botón con `tabIndex={-1}` y recibe el foco en cada envío fallido. Es presentación; el texto y el orden D1–D3 no cambian. **El dueño confirma que esto cuenta como layout.**
- **D13/D14** `BirthdayGreeting` recibe una sola prop de presentación, `embedded`, para no pintar tarjeta dentro de tarjeta. El `<summary>` es el control; no lleva un botón adentro.
- **D6** Lo pegado respeta `--topbar-h` arriba y `scroll-padding-bottom` abajo, para que el foco nunca quede tapado (WCAG 2.4.11).
- **D15** Lo nuevo usa clases `.announce-*` en `globals.css`, no estilos en línea, para que `module_ui_refresh` (encendido en dev) le llegue. Se prueba con los dos flags ON y OFF, en claro y oscuro.
- **E5** `pickStudent()` por vista es la única diferencia entre las dos suites. B5 se afirma por los `targetIds` enviados.
- **E11** El gateway pasa de 21 a 22 flags, con una fila nueva en la tabla de flags de presentación y la prueba de features en forma de tabla.
- **E12** El e2e lleva su propio proyecto de Playwright, una entrada en `testIgnore` y un script, y envía a un solo alumno.
- **E13** `TextArea rows={4}`, como C2.
- **E14** El error de carga de la felicitación (A5) queda dentro del panel plegado: se ve al abrirlo. Se verifica en el `/qa` visible.
- **E15** El buscador usa `useMemo` para los nombres ya normalizados.
- **D16/D17** La vista previa imita la notificación de la app de padres (ícono, título en negritas, dos renglones). Mientras se envía, el botón dice "Enviando…" con el estilo de ocupado y sigue activo (la regla de la casa).

### Decisiones del dueño (checkpoint 2)
1. Aprobar el plan v2.
2. D5: columna de 960 px (recomendado) o el ancho completo de la maqueta.
3. D12: mover el foco a la alerta, ¿es layout? (recomendado: sí).
4. BC1–BC7 quedan **fuera** salvo que el dueño pida alguno. BC6: que cada chip se anuncie como lo que muestra. BC7: "llega sola a las 7:00" no es cierto en el dev de GCP, porque el scheduler de cumpleaños nunca se encendió ahí. Esto ya pasaba antes del rediseño.
