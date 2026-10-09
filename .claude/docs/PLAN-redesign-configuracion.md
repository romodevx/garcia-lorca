# PLAN — Rediseño de Configuración académica: "Ruta de arranque" (v2)

Fecha: 2026-10-09 · Rama: `feature/redesign-configuracion-academica` · Skill: `/redesign`
Estado: **pendiente de aprobación del dueño (checkpoint 2)**

v2 incorpora `/plan-design-review` (4/10, 6 bloqueos, 12 sugerencias) y `/plan-eng-review`
(5 bloqueos, 11 sugerencias). Los reportes están en
`.gstack/redesign/configuracion-academica/plan-{design,eng}-review.md`.
Los demás insumos están en la misma carpeta, fuera de git: `behavior.md` (contrato,
acciones A1–A56), `design-review.md` (24 problemas), `before/` (capturas) y
`variants/a-ruta.html` (la maqueta elegida, 5/5 en el tablero).

## 0. La restricción

No cambian la funcionalidad, los endpoints, los contratos, los datos, los permisos ni las
reglas de negocio. Cada acción de `behavior.md` llama a la misma función con los mismos
argumentos. Cambian el orden, la agrupación, la jerarquía, el texto y el movimiento, y solo
**en la vista nueva**: con el flag apagado, la pantalla de hoy queda idéntica, con el mismo
aspecto y el mismo tráfico. Lo que tocaría comportamiento va en §10.

## 1. La dirección

Dos columnas. A la izquierda va la tarjeta **"Puesta en marcha"**, con 8 pasos y el estado
de cada uno. Debajo, separado por un filete, va **Herramientas** (Migrar alumnos y
Papelera). A la derecha va **el paso abierto**.

**Dónde manda el plan sobre la maqueta.** La maqueta fija la composición: dos columnas, la
lista de pasos con estado, un primario por paso y el pie anterior/siguiente. Donde difiere,
manda este plan:
- **(a)** Materias muestra "Revisar" y nunca ✓.
- **(b)** No hay barra de progreso.
- **(c)** El cuerpo de cada paso es el panel actual, no la tabla dibujada. Grupos conserva
  sus chips y su fila en línea.
- **(d)** Los chips de grado del paso 6 sí llevan las píldoras "1 grupo" / "sin grupos",
  calculadas con la lectura de §4.1.
- **(e)** El paso abierto se marca con fondo y círculo lima, sin barra lateral.
- **(f)** En lugar de emoji van íconos de `_design/Icon`.

### 1.1 Pasos

| # | Paso | Paneles (sin cambiar lo que hacen) | Listo cuando (para el ciclo **elegido**) |
|---|------|-----------------------------------|------------------------------------------|
| 1 | Datos del colegio | `SchoolProfilePanel` | el perfil tiene nombre |
| 2 | Ciclo escolar | `CycleBar` | el ciclo elegido es el activo |
| 3 | Niveles y grados | `LevelsPanel` (+ asistencia, calificación, eliminar) · `GradeLevelsPanel` | hay ≥ 1 nivel y cada nivel tiene ≥ 1 grado |
| 4 | Periodos | `TermsPanel` · `LevelPeriodsEntry` | hay ≥ 1 nivel y cada uno tiene ≥ 1 periodo en el ciclo |
| 5 | Materias | `SubjectsPanel` | nunca se marca: muestra "Revisar" (§4.2) |
| 6 | Grupos | `GroupsPanel` | hay ≥ 1 grado y cada grado tiene ≥ 1 grupo en el ciclo |
| 7 | Colegiaturas | `TuitionPanel` (+ `TuitionPage`) | hay ≥ 1 nivel y cada nivel tiene tabulador en el ciclo |
| 8 | Calendario | `CalendarPanel` | opcional: nunca bloquea ni cuenta |

Una colección vacía **nunca** cuenta como lista. Una escuela sin niveles tiene los pasos 3,
4, 6 y 7 pendientes.

### 1.2 Texto (pasa por `humanizer` y se fija en `setupSteps.test.ts`)

| Paso | Propósito (dentro del paso) | Sub-línea si está listo | Sub-línea si está pendiente |
|---|---|---|---|
| 1 | Salen en cada comprobante. | {nombre} | Los comprobantes salen sin nombre |
| 2 | El ciclo decide fechas, periodos y cobros. | {ciclo} · activo | Sin ciclo activo / Borrador · {activo} es el ciclo en curso |
| 3 | Lo que ofrece el colegio y sus grados. | {n} niveles · {m} grados | Faltan grados en {nivel} |
| 4 | Cómo se parte el año para calificar. | {n} niveles con periodos | Falta en {nivel} |
| 5 | Materias por grado. | — | Revisar |
| 6 | Cada grado necesita al menos un grupo para inscribir alumnos. | {n} grupos | Falta en {grado} de {nivel} |
| 7 | Precio por nivel en el ciclo. | {n} niveles con precio | Ningún nivel tiene precio / Falta {nivel} |
| 8 | Días sin clases y actividades. Opcional. | {n} días cerrados | Opcional |

Si falta en más de un lugar: "Falta en {primero} y {n} más".

**Encabezado:** "Volver al inicio" + ícono + "Configuración académica". El subtítulo nombra
el ciclo elegido y lo que falta: "Ciclo 2026–2027 · faltan Grupos y Colegiaturas", con un
máximo de 2 nombres y luego "y N más". Si el ciclo es un borrador, lo dice ("Ciclo
2027–2028 · borrador"). Con los 6 pasos medibles listos, el subtítulo pasa a "Ciclo
2026–2027 · listo para inscribir" y la tarjeta se titula "Configuración" en vez de "Puesta
en marcha"; las marcas siguen visibles para entrar a editar.

Salen de la vista nueva las 3 tarjetas de conteo (problemas #5 y #6) y el "Migrar alumnos"
del encabezado (#4).

### 1.3 Un solo primario lima por paso

| Paso | Lima | Baja a secundario | "Siguiente" |
|---|---|---|---|
| 1 | Capturar datos (si está vacío) | — (lleno: "Editar datos" en contorno) | texto si vacío / lima si lleno |
| 2 | Nuevo ciclo (si no hay ciclos) | Activar ciclo (como hoy) | texto / lima |
| 3 | Nuevo nivel | Guardar calificación (contorno, dentro de los ajustes del nivel) | texto |
| 4 | Crear trimestres (si no hay periodos) | Periodos por grado, Copiar a otros niveles | texto / lima |
| 5 | Agregar materia | — | texto |
| 6 | — (fila en línea) | — | lima |
| 7 | — | Generar cargos del mes, Definir/Editar | lima |
| 8 | — | Nuevo día cerrado, Nueva actividad | no tiene |

El paso 1 no lleva "← anterior". "Eliminar nivel" queda como texto rojo al pie de los
ajustes del nivel, separado de "Guardar calificación" y de "Nuevo nivel" (#10).

### 1.4 Paso que abre

1. Si ya se abrió uno en esta sesión (`sessionStorage`, clave `config-setup-step`), se
   reabre ese.
2. Si no, el primero pendiente.
3. Si todos están listos, el 3, que es lo más editado.

Se decide **una vez**, cuando terminan las lecturas de estado (o fallan), y nunca salta solo
después. Al salir del modo tabulador sigue abierto el paso 7. Los pasos se abren en
cualquier orden: la lista guía, no encierra.

### 1.5 Herramientas

- **Migrar alumnos del sistema anterior**: solo con `onOpenStudentImport`. Debajo, en
  `--text-faint`, va "Necesita niveles, grados y grupos."
- **Papelera**: solo con `canDeleteLevels` y sin número. Abre en la columna derecha con el
  mismo marco, sin rótulo de paso ni pie. En la lista, el renglón queda marcado como
  abierto y ningún paso lo está.

### 1.6 Tamaños de pantalla

- **≥ 1200 px:** lista de 330 px + paso.
- **900–1199 px:** lista de 260 px, con las sub-líneas cortadas con elipsis y el texto
  completo en `title`. El paso 3 apila Niveles sobre Grados. Si la matriz de meses del paso
  4 no cabe, abre en "Lista"; "Tabla" sigue disponible y nunca hay scroll horizontal.
- **< 900 px:** una barra "Paso 6 de 8 · Grupos ▾" de ≥ 48 px de alto, pegada bajo la barra
  superior, con `scroll-padding-top` igual al alto de las dos. El desplegable es un
  `<button aria-expanded>`; Esc lo cierra y devuelve el foco. Colegiaturas va en filas
  apiladas, para que "Definir" no se salga (#23).

### 1.7 Teclado, lector de pantalla y movimiento

- Cada paso es un `<button>` dentro de `<nav aria-label="Puesta en marcha">`, con
  `aria-current="step"` en el abierto.
- Al abrir un paso, el foco va al `<h2>` del panel (`tabIndex={-1}`). Un `aria-live="polite"`
  anuncia "Paso N de 8, {título}". Siguiente y anterior hacen lo mismo.
- Movimiento: el cuerpo del paso entra con opacidad 0→1 y 8 px de desplazamiento, en 160 ms
  ease-out, y **termina en `transform: none`** (si queda un transform, los toasts fijos
  cambian de contenedor). Al quedar listo, el círculo pasa a ✓ con escala 0.8→1 en 180 ms.
  Nada más se anima. Con `prefers-reduced-motion` no hay desplazamiento ni escala.

### 1.8 Destino de los 24 problemas

| # | Destino |
|---|---|
| 1, 2, 3, 22 | resueltos por la estructura de pasos (§1.1, §1.3) |
| 4, 5, 6 | §1.2 (se quitan el duplicado y los conteos) |
| 7 | §1.3 (Generar cargos baja a secundario). Ocultarlo es §10 P2 |
| 8 | §1.1, paso 4 (los dos "Periodos" juntos) |
| 9 | §4.3 (vacíos con la acción de al lado) |
| 10 | §1.3 |
| 11 | **no se resuelve aquí.** Los chips de grado de Materias son un multi-select, no un selector duplicado (§3.4); solo cambia su rótulo a "En qué grados" |
| 12 | en la vista nueva: chips "1º" y no "1. 1º"; el nivel sin "· por ciclo", y "· por periodo" solo cuando aplica |
| 13 | en la vista nueva: toda alta con el estilo de fila en línea (`InlineCreateRow`) donde ya existe |
| 14 | en la vista nueva: el título de cada panel es h2; "Actividades" pasa a h3 |
| 15, 16, 18 | §10 P6 (diálogos globales) |
| 17 | §10 P7 |
| 19, 20, 21 | §10 P1, P3, P4 |
| 23 | §1.6 |
| 24 | §10 P9 (contraste y entradas de 16 px son tokens globales: tocan todas las pantallas) |

## 2. Mapa: cada acción vieja → dónde vive ahora

Las acciones A1–A56 conservan etiqueta (salvo los rótulos de §1.8), llamada y efecto.

- **A1–A3** Volver → encabezado. Migrar (encabezado y tarjeta) → Herramientas, con una sola
  entrada; las dos llamaban a `onOpenStudentImport`.
- **Datos del colegio** → paso 1, con el mismo diálogo y el mismo `saveSchoolProfile`.
- **Ciclo** (chip, Nuevo ciclo, Crear ciclo, Activar) → paso 2. Elegir un ciclo cambia el de
  toda la pantalla, como hoy.
- **Niveles** (chip, Nuevo nivel, Guardar y crear otro, asistencia, calificación, Eliminar
  nivel) → paso 3, columna izquierda.
- **Grados** (chip, Nuevo grado, Enter guarda) → paso 3, columna derecha.
- **Periodos** (Crear trimestres, Nuevo periodo, meses, Tabla/Lista, Copiar a otros niveles,
  Periodos por grado) → paso 4.
- **Materias** (nombre, "Todos los grados" y los chips por grado, Agregar, Renombrar,
  Eliminar, Deshacer) → paso 5, con los chips de nivel arriba.
- **Grupos** → paso 6, con los chips de nivel y de grado arriba.
- **Colegiaturas** (Definir/Editar → `TuitionPage`, Generar cargos, copiar del ciclo
  anterior) → paso 7.
- **Calendario** (Nuevo día cerrado, Quitar + Deshacer, Nueva actividad) → paso 8.
- **Papelera** (Restaurar) → Herramientas.
- **`notice`** (8 s) → igual que hoy.

La selección de nivel y de grado sigue siendo **un solo** estado (`selectedLevelId`,
`selectedGradeLevelId`). Los chips de los pasos 4 a 6 lo leen y lo escriben, así que lo
elegido en el paso 3 sigue elegido en el 4.

## 3. Restricciones que no se negocian

1. **Los pasos no abiertos se esconden con el atributo `hidden`; no se desmontan.** Siguen
   montados, con las mismas lecturas al entrar, los mismos borradores sin guardar y los
   mismos deshacer que hoy. `hidden` los saca del orden de tabulación y del árbol de
   accesibilidad. (Desmontar perdería borradores, mataría el deshacer de Materias,
   confirmaría antes de tiempo los "Quitar" del calendario y obligaría a leer más; ver
   eng-review, hallazgo 6.)
2. **Los toasts se pintan con portal.** `UndoToast` y `Toast` hoy se pintan dentro del panel,
   y dentro de un paso escondido **desaparecerían** mientras el temporizador de 8 s sigue y
   confirma. Pasan a `createPortal(…, document.body)`. Así se ven igual en las dos vistas y
   además se desactiva la trampa de "fixed dentro de un ancestro con transform".
3. **Modo tabulador: igual que hoy.** Los demás pasos se desmontan, no hay retorno temprano
   y `TuitionPanel` ocupa un lugar fijo del árbol (hermanos `{cond && …}`, nunca un arreglo
   cuyos índices se muevan). La regresión "Editar llega en blanco" se repite en la vista
   nueva.
4. **El selector de grados de Materias se queda.** No es un duplicado: "Todos los grados" y
   los chips por grado deciden **en qué grados** se crea la materia (A49,
   `createSubjectInGrades`).
5. **`config:write` no se revisa en la pantalla**, ni en la vista nueva ni en la vieja (§10
   P5).

## 4. Estado de los pasos

### 4.1 Lecturas nuevas (solo con el flag encendido)

Para marcar los pasos 3, 4 y 6 hay que ver todos los registros, no solo los del elegido. Los
clientes ya aceptan omitir el filtro, y el gateway lo pasa sin tope de filas (verificado:
`handler/config.go:474,756,818`):

- `listGradeLevels(API_URL)`
- `listTerms(API_URL, cycleId)`
- `listGroups(API_URL, cycleId)`

Los pasos 1, 7 y 8 salen de paneles que ya están montados, mediante callbacks y **sin leer
dos veces**:
- `SchoolProfilePanel` → `onLoaded(profile | "failed")`
- `TuitionPanel` → `onPlansLoaded(cycleId, plans | "failed")`
- `CalendarPanel` → `onLoaded(cycleId, count)`

Los resultados de un ciclo que ya no es el elegido se descartan.

Tráfico: 3 GET más al entrar y al cambiar de ciclo, y 1 GET por cada alta (§4.2). No hay
endpoint, contrato, dato ni permiso nuevo. Las lecturas de config pasan por `config:read`;
las de tabuladores, por finance, como hoy.

### 4.2 Refresco tras cada alta

`useSetupStatus` expone `refresh(kind)`. La vista nueva envuelve los callbacks existentes
con `.then(() => refresh(kind))`, sin tocar la llamada ni sus argumentos:

| Alta | Se refresca |
|---|---|
| crear nivel · restaurar nivel | grados |
| crear grado | grados |
| crear periodo · copiar meses | periodos |
| crear grupo | grupos |
| crear o activar ciclo | todo |
| guardar datos | perfil (vía `onLoaded`) |
| guardar tabulador · generar cargos | tabuladores (vía `onPlansLoaded`) |

Materias no se marca. `SubjectsPanel` ya lee todas las materias en una sola llamada (la v1
de este plan decía otra cosa), pero cuándo están "listas" las materias de un grado es una
decisión de producto (§10 P10).

### 4.3 Estados de cada paso

| Situación | Lo que se ve |
|---|---|
| Estado cargando | número en gris, sub-línea "Revisando…", sin spinner |
| Lectura fallida | sin marca y sin sub-línea; sin mensaje de error (el estado ayuda, no es un dato) |
| Listo | círculo `--forest` con ✓ y la sub-línea de §1.2 |
| Pendiente | círculo con número; la sub-línea de §1.2 en `--warn-fg` |
| Abierto | fondo lima suave y círculo lima |
| Pasos 4–8 sin ciclo | "Primero crea el ciclo escolar." + enlace de texto "Ir al paso 2" |
| Pasos 4–6 sin niveles | "Primero agrega un nivel." + "Ir al paso 3" |
| Pasos 4–5 sin nivel elegido | chips de nivel + "Elige un nivel para ver sus periodos" / "…sus materias" (no se elige solo: eso es §10 P8) |
| Paso 6 sin grado elegido | chips de nivel y grado (con píldoras) + "Elige un grado para ver sus grupos." |

## 5. Archivos

Nuevos, en `apps/web/app/_screens/config/setup/`:

> **Ajuste tras la revisión de código (S3, 2026-10-09):** `useCatalogState.ts` y
> `SetupPresentation.tsx` viven en `_screens/config/`, no en `setup/`: los usan la vista
> vieja y los paneles compartidos, y `setup/` debe poder borrarse sin romperlos. La
> dependencia va de `setup/` hacia `config/`, nunca al revés
> (`dependencyDirection.test.ts`). La única entrada a `setup/` es `AppShell`.

- `useCatalogState.ts`: **extracción** del estado, los efectos y los handlers de
  `Catalog.tsx`. Devuelve grupos (`cycles`, `levels`, `grades`, `groups`, `pricing`,
  `notice`), no una bolsa de 20 campos.
- `setupSteps.ts`: el registro `STEPS` (id, título, si se mide, si tiene primario propio) y la
  función pura `deriveSteps(input)`, que calcula el estado de cada paso, el subtítulo y el
  paso inicial.
- `useSetupStatus.ts`: las lecturas de §4.1 con `refresh(kind)` y protección contra
  carreras. Sigue el patrón con llave de `useLevelTerms`.
- `SetupSteps.tsx`: la lista, Herramientas y la barra móvil.
- `StepFrame.tsx`: "Paso N de 8", la línea de propósito y el pie. **No pinta título propio**:
  el `<h2>` del panel es el título del paso.
- `LevelGradeChips.tsx`: chips de nivel y grado sobre el estado compartido, con píldoras
  opcionales.
- `steps/StepProfile.tsx … steps/StepCalendar.tsx`: un componente delgado por paso, que solo
  conecta piezas.
- `CatalogSetup.tsx`: la vista nueva. Arma todo lo anterior.
- `SetupPresentation.tsx`: un contexto `useSetupPresentation()` cuyo valor por omisión es
  "vista vieja". Los paneles lo leen para tres cosas de presentación: pintar `Section` sin
  tarjeta, el `kind` de sus botones (§1.3) y los rótulos de §1.8. Así se evita una prop
  nueva en cada panel, y la vista vieja no cambia porque no tiene el proveedor.

Tocados:

- `AppShell.tsx`: elige la vista **ahí**, con `useFlag("module_config_setup")`, junto a las
  demás lecturas de flags. `Catalog.tsx` no lee el flag: sus pruebas lo renderizan sin
  `FlagsProvider`, y `useFlag` truena fuera del proveedor.
- `Catalog.tsx`: usa `useCatalogState` y nada más.
- `parts.tsx` (`Section`), `LevelsPanel`, `GradingControl`, `GradeLevelsPanel`, `TermsPanel`,
  `CopyMonthsInline`, `LevelPeriodsEntry`, `SubjectsPanel`, `TuitionPanel`, `CalendarPanel`:
  leen el contexto de presentación (§1.3, §1.8).
- `SchoolProfilePanel`, `TuitionPanel`, `CalendarPanel`: los callbacks de §4.1, opcionales y
  solo de aviso. `TuitionPanel` además gana su protección `active` en la recarga.
- `_design/Toast.tsx`, `UndoToast`: portal a `document.body` (§3.2).
- `packages/ui/src/flags/types.ts`: `"module_config_setup"` en `FlagKey`.
- `services/gateway/internal/config/flags.go`: `module_config_setup` en `AllModuleFlags`.
- `docker-compose.yml:125`: `ENABLED_MODULES: "${ENABLED_MODULES:-<la lista de hoy>}"`. El valor
  por omisión no cambia, así que el flag queda apagado. Para el QA local:
  `ENABLED_MODULES=<lista>,config_setup docker compose up -d gateway`.
- `.claude/docs/FLAGS.md`: registrar el flag. Terraform no se toca: queda apagado en dev
  hasta pasar las compuertas.
- `.claude/docs/design/DESIGN-SYSTEM.md`: nuevo patrón **"Superficie de puesta en marcha"**
  (lista de pasos + paso abierto, sin tarjetas de conteo ni FAB), con Configuración como
  referencia, y quitar "Config (catálogo)" de la anatomía de lista. Si no se anota, el
  siguiente agente que aplique el mandato regresaría las tarjetas de conteo.

## 6. Flag

`module_config_setup`, **apagado por omisión**, solo de presentación: no protege endpoints
porque no hay ninguno nuevo. La frontera sigue en `module_config` + `config:read/write`.
Con el flag apagado: la misma pantalla, las mismas lecturas y el mismo aspecto (salvo el
portal de los toasts, que no se ve). Retirarlo es una tarea de deuda técnica: borrar la
vista vieja, el flag y las pruebas que solo la cubren.

## 7. Pruebas (TDD: cada una falla primero)

1. **Gateway:** `AllModuleFlags` incluye `module_config_setup`, y `/features` lo devuelve en
   `false` (tabla en Go).
2. **`setupSteps.test.ts`** (puras, de tabla):
   - listo o pendiente por paso;
   - colección vacía ⇒ pendiente;
   - lectura fallida ⇒ sin marca;
   - ciclo elegido en borrador;
   - subtítulo con 0, 1, 2 y 3 o más pasos pendientes;
   - todo listo ⇒ "listo para inscribir";
   - paso inicial: sesión ⇒ primero pendiente ⇒ 3;
   - Materias nunca cuenta y Calendario nunca bloquea;
   - todo el texto de §1.2.
3. **`useSetupStatus.test.ts`:**
   - refresco tras cada alta de §4.2;
   - carrera: cambiar de ciclo con una lectura pendiente descarta la vieja;
   - lo mismo con `onPlansLoaded`;
   - los mocks **responden según sus argumentos**: con o sin filtro devuelven listas
     distintas, para que nada pase en verde por accidente.
4. **`SetupSteps.test.tsx`:**
   - `aria-current`;
   - foco al `<h2>` y anuncio `aria-live`;
   - Herramientas solo con sus props;
   - barra móvil con `aria-expanded`, que se cierra con Esc y devuelve el foco.
5. **`CatalogSetup.test.tsx`** (flag encendido):
   - una tabla por paso: cada acción A1–A56 llama a la misma función con los mismos
     argumentos;
   - "Siguiente" nunca es un segundo lima (§1.3);
   - "Migrar alumnos" aparece una vez;
   - la selección sobrevive al cambio de paso;
   - los pasos no abiertos dan `not.toBeVisible()`, con consultas por `getByRole` (porque
     `getByText` encuentra nodos escondidos);
   - A49: "Todos los grados" crea la materia en N grados;
   - **el deshacer sigue al alcance tras cambiar de paso**: el Deshacer del calendario
     cancela el DELETE y el de Materias recrea la materia;
   - tabulador de ida y vuelta: "Editar" llega con los valores guardados, al cerrar sigue
     el paso 7 con la misma selección, y el estado se refresca;
   - crear el primer grado del último nivel vacío pone el ✓ en el paso 3 sin recargar;
   - el paso abierto nunca salta solo después de que el usuario eligió uno;
   - cada fila de §4.3;
   - carga, error de carga y `notice`.
6. **Vista vieja, flag apagado:**
   - las 531 pruebas actuales (incluidas las 89 de caracterización, commit `d31b8154`)
     pasan **sin tocarlas**;
   - una prueba nueva verifica que la vista vieja **nunca** llama a `listGradeLevels` sin
     nivel ni a `listTerms`/`listGroups` sin su filtro (cero tráfico nuevo).
7. **Portal de toasts:** las pruebas existentes de `Toast` y `UndoToast` siguen verdes, más
   una que verifica que el toast vive en `document.body`.
8. **e2e:**
   - `setup-from-zero` corre tal cual, con el flag apagado;
   - una variante con el flag encendido fuerza el flag con
     `page.route("**/api/v1/features")`, corre en **su propio proyecto de Playwright** y
     vacía la base en su `beforeAll` con el mismo script local, porque `globalSetup` vacía
     una sola vez por corrida;
   - ojo: sin `E2E_SKIP_RESET=1`, correr Playwright borra la base local.

## 8. Orden de construcción

1. `useCatalogState`: extracción, con su propio commit y las 531 pruebas en verde.
2. Flag: gateway, `FlagKey`, FLAGS.md y la interpolación en compose.
3. `deriveSteps` y `STEPS`.
4. Callbacks de los paneles y protección `active` en `TuitionPanel`.
5. `useSetupStatus` con refresco.
6. Portal de los toasts.
7. Contexto de presentación y su uso en los paneles; la vista vieja sin cambios.
8. `SetupSteps`, `StepFrame` y `LevelGradeChips`.
9. Pasos 1 a 8 y `CatalogSetup`.
10. El cambio de vista en `AppShell`.
11. Tamaños de pantalla.
12. Texto (`humanizer`) y movimiento (`find-animation-opportunities` → `animate` →
    `/review-animations`).
13. Verificación: pruebas con el flag encendido y apagado; `/qa` en el navegador visible con
    el flag encendido; capturas en `after/`; `/design-review`; `/review`; DESIGN-SYSTEM.md.

Estimación honesta: 3 a 4 días de un equipo humano · unas 5 a 6 h con CC.

## 9. Preguntas para el dueño

1. **Lecturas nuevas (§4.1, §4.2).** Con el flag encendido: 3 GET al entrar y al cambiar de
   ciclo, y 1 por cada alta. Recomiendo sí: sin ellas la lista no puede decir qué falta.
2. **Esconder los pasos y llevar los toasts a un portal (§3.1, §3.2).** Las dos revisiones
   lo recomiendan. El portal toca `Toast` y `UndoToast`, que comparten todas las pantallas,
   aunque no cambia nada visible.

## 10. Propuestas que cambian comportamiento (fuera de este trabajo)

- **P1** Prellenar "Nuevo ciclo" con el siguiente al activo (#19).
- **P2** Ocultar "Generar cargos del mes" sin tabuladores, o explicar por qué no hará nada (#7).
- **P3** URL propia para el tabulador y migas con enlace (#20).
- **P4** "Mes de inicio" como selector de mes con nombre (#21).
- **P5** Ocultar la escritura a quien no tiene `config:write`.
- **P6** Diálogos globales: fondo sólido atenuado (la regla global contradice
  DESIGN-SYSTEM §8), `<dialog>` nativo y un orden único en el pie (#15, #16, #18).
- **P7** Datos del colegio como página o como formulario dentro del paso 1 (#17).
- **P8** URL por paso (`?paso=grupos`) y autoselección cuando hay un solo nivel.
- **P9** Contraste del texto de ayuda (≥ 4.5:1) y entradas de 16 px en toda la app (#24).
- **P10** Definir cuándo están "listas" las materias, para marcar el paso 5.
- **Bugs que encontró la caracterización** (aparte, con prueba de regresión): el calendario
  muestra "HTTP 500" crudo, un error de carga viejo sigue a la vista tras recargar bien, y
  `deleteSchoolEvent` no codifica el id.
