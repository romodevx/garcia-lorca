# Revisión de diseño: PLAN-redesign-avisos.md

Fecha: 2026-10-12 · rama `feature/redesign-avisos` (HEAD `403832ea`) · skill `/plan-design-review`, en modo no interactivo.

Revisé: `.claude/docs/design/PLAN-redesign-avisos.md` (borrador v1), `.gstack/redesign/avisos/behavior.md` (contrato congelado), `diagnosis.md` (P1–P14), la maqueta aprobada `variants/variant-a.png` + `a.html`, las capturas de `before/`, `.claude/docs/design/DESIGN-SYSTEM.md`, `~/.claude/rules/ui-forms-and-modals.md`, `AnnouncementsScreen.tsx`, `BirthdayGreeting.tsx`, sus pruebas, `_design/form/fields.tsx`, `_design/Page.tsx`, `config/api.ts` (`labelGroups`), `students/api.ts`, `globals.css` y `e2e/responsive.spec.ts`.

## Cómo se hizo esta revisión

- **No interactiva.** Cada hallazgo trae su corrección **recomendada** como enmienda propuesta; se eligió siempre la opción recomendada. **El plan no se editó** y no se tocó código ni se hizo commit.
- **Sin maquetas nuevas:** la variante A ya está aprobada y es la referencia visual. **Sin voces externas** (Codex ni subagente): decidido "seguir sin ellas". Es la opinión de un solo modelo.
- Evidencia: lectura estática del plan, la maqueta y el código. No se renderizó la pantalla.
- La etiqueta **[ROMPE CONGELAMIENTO]** marca lo que, hecho como lo pide el plan o la maqueta, cambiaría comportamiento. Eso va a la lista de propuestas del dueño (§7 del plan), nunca al rediseño.

## Calificaciones

| Dimensión | Antes | Después de las enmiendas | Por qué no es 10 hoy |
|---|---|---|---|
| Paso 0: qué tan completo está el plan | **6/10** | 9/10 | El mapeo inventario→vista (§4) y la estrategia de un solo hook son ejemplares. Faltan estados, el ancho de página y varias trampas de nombres accesibles que rompen la caracterización. |
| 1. Arquitectura de la información | **7/10** | 9/10 | Pasos 1-2 + resumen fijo es la jerarquía correcta. La maqueta trae piezas que el plan no construye (y no puede), y el orden en tablet vertical es ambiguo. |
| 2. Estados de interacción | **4/10** | 9/10 | No hay estados de carga, vacío ni sin resultados para grupos, niveles y el buscador; tampoco qué muestra la vista previa en blanco. |
| 3. Recorrido y arco emocional | **6/10** | 8/10 | "Ver antes de enviar" resuelve el miedo central (P6). Pero Enter en el buscador puede enviar el aviso, y el error sigue lejos de su campo (P8). |
| 4. Riesgo de verse "hecho por IA" | **8/10** | 9/10 | Modo OPERATE, sin rechazos duros. El riesgo es "tarjetas apiladas" en la fila de cumpleaños abierta (tarjeta dentro de tarjeta). |
| 5. Alineación con el sistema de diseño | **6/10** | 9/10 | Plan ambiguo sobre estilos en línea frente a la capa `data-look="refresh"`; el desplazamiento de lo pegado ignora `--topbar-clearance`. |
| 6. Responsive y accesibilidad | **5/10** | 9/10 | ✓ dentro del nombre accesible, texto visible ≠ `aria-label` (WCAG 2.5.3), `<button>` dentro de `<summary>`, punto de quiebre por viewport y no por contenedor, barra inferior que tapa el foco (2.4.11). |
| **Global (la más baja de las seis)** | **4/10** | 8/10 | Sube con D1–D6 y D9–D11. |

## Hallazgos

Severidad: **Alta** = bloquea la aprobación del plan · **Media** = corregir antes de construir · **Baja** = pulido o nota.

### D1 · Alta · Enter en "Buscar alumno" envía el aviso
- **Evidencia:** §3.2 `StudentSearch` mete un `input type="search"` dentro del mismo `<form>` cuyo `onSubmit` es `send` (AnnouncementsScreen.tsx:149). En HTML, Enter en un campo de una línea envía el formulario. Con título, mensaje y un alumno ya elegidos, quien teclea un segundo nombre y oprime Enter **manda la nota a la familia equivocada** o a la primera. Sin datos, sale "Un aviso necesita título y mensaje." (D2) en un momento que no tiene sentido.
- **Enmienda:** en §3.2 `StudentSearch`: "Enter dentro del buscador **no envía**: `onKeyDown` con `key === "Enter"` → `preventDefault()`; no elige resultado (elegir con Enter sería interacción nueva, se deja para después)". Prueba que falla primero: escribir y oprimir Enter en el buscador no llama a `announce`. No rompe el congelamiento: el campo es nuevo y la excepción a "Enter envía" de `ui-forms-and-modals.md` se justifica en una línea.

### D2 · Alta · El ✓ visible cambia el nombre accesible de las audiencias y rompe la caracterización
- **Evidencia:** la maqueta pinta "✓ Grupos" y "✓ 2° A" como texto. Las pruebas buscan `getByRole("button", { name: "Grupos" })` y `{ name: "1° B" }` (behavior.test.tsx:87-97, test.tsx:72, 114). Los botones de audiencia **no** llevan `aria-label` hoy; si el ✓ va como texto, su nombre pasa a ser "✓ Grupos" y la suite contra la vista nueva se cae. Para los chips el `aria-label` lo tapa, pero en las audiencias no.
- **Enmienda:** §3.2 `AudienceTabs` y `GroupedTargets`: "el ✓ es `<Icon name="check" aria-hidden>`; nunca texto. El nombre accesible es exactamente el de `SCOPES[].label` / el `aria-label` de hoy". Agregar a §5: una aserción `toHaveAccessibleName("Grupos")` en las dos vistas.

### D3 · Alta · Chips "2° A" bajo el encabezado del nivel: texto visible distinto del `aria-label`, y `labelGroups` no trae lo que hace falta para acortarlo
- **Evidencia:** la maqueta muestra "2° A" bajo "PRIMARIA". B3 congela `aria-label` = la etiqueta completa ("2° PRIMARIA · A", hecha por `labelGroups`, config/api.ts:710-726). Si el chip dice "2° A" y su nombre accesible es "2° PRIMARIA · A", falla WCAG 2.5.3 (el nombre debe contener el texto visible): quien usa control por voz dice "2° A" y nada responde. Además, el cambio aditivo de §3.1 agrega `levelId` y `ordinal`, pero para mostrar "2° A" falta el **nombre del grado sin el nivel** (`gradeWithLevel` lo fusiona).
- **Enmienda:** (a) §3.1 `labelGroups`: los campos aditivos son `levelId`, `ordinal` **y `gradeName`** (el nombre del grado como viene del catálogo). (b) §3.2 `GroupedTargets`: chip visible = `gradeName · name` sin el nivel (ya lo dice el encabezado), `aria-label` **sin cambios** (congelado). Cada sección es `role="group"` con `aria-labelledby` del encabezado del nivel. (c) Registrar la desviación de 2.5.3 y agregar **BC6** a §7: "nombre accesible del chip `2° A, Primaria`" (cambia lo que las pruebas fijan, por eso es del dueño). **[ROMPE CONGELAMIENTO si se cambia el `aria-label` dentro del rediseño.]**

### D4 · Alta · La maqueta aprobada trae dos cosas que el plan no puede construir
- **Evidencia:** `a.html:24` "2 seleccionados · **Quitar todos**" es justo BC5, una acción nueva. `a.html:39` "Mensaje automático · llega sola a las 7:00 · **texto predeterminado**": ese "predeterminado" sale de `isDefault`, que vive dentro de `BirthdayGreeting` (A5). Para mostrarlo en la fila cerrada habría que pedir la plantilla otra vez (llamada nueva) o sacar el estado del componente (§3.1 dice "sin cambios"). Quien construya con la maqueta al lado lo va a copiar.
- **Enmienda:** agregar a §2 una lista "**De la maqueta NO se construye:** «Quitar todos» (BC5) y «texto predeterminado» en la fila de cumpleaños". El texto de la fila queda como en §3.2: "Felicitación de cumpleaños · Mensaje automático · 7:00". **[ROMPE CONGELAMIENTO si se construye.]**

### D5 · Alta · Ancho de página sin decidir: la maqueta mide 1224 px y la columna de lectura está topada en 960 por una prueba e2e
- **Evidencia:** la vista vieja usa `Page variant="reading"` (`.page-column` ≤ `--reading-max` 960 px). `e2e/responsive.spec.ts:79-86` afirma `.page-column ≤ 960` en `/avisos`. La rejilla de la maqueta va de x=28 a x=1252. El plan no dice qué `Page` usa la vista nueva. Además el quiebre "≥ 1024 px" es de **viewport**, pero lo que importa es el ancho de la columna.
- **Enmienda (recomendada):** §2: "La vista nueva usa `Page variant="reading"` (960 px). Rejilla `minmax(0,1.55fr) minmax(320px,1fr)`, hueco 22 px: unos 570 px para el redactor (buena medida de lectura para el mensaje) y unos 368 px para el resumen. El quiebre a una columna es un `@container` sobre `.page-column` por debajo de 860 px, no un `@media` de viewport". Así `responsive.spec.ts` sigue verde con el flag ON y no hay que tocarlo. Si el dueño prefiere el ancho de la maqueta, la alternativa es `wide` + reescribir esa prueba para medir el redactor; es un cambio de prueba de layout que tiene que quedar escrito.

### D6 · Media · Lo pegado choca con la barra superior y tapa el foco
- **Evidencia:** la maqueta usa `position:sticky; top:16px`. Con `module_ui_refresh` (ON en dev) la cápsula superior es pegajosa y ocupa `--topbar-clearance` (DESIGN-SYSTEM.md:103). El resumen se metería debajo. En tablet vertical, la barra inferior pegada ("Va a" + Enviar) tapa el campo con foco al tabular hacia el Mensaje: WCAG 2.4.11, que la regla de formularios cita explícitamente.
- **Enmienda:** §2: "Resumen: `top: calc(var(--topbar-clearance, 0px) + 16px)`. Barra inferior: altura fija en `--announce-bar-h` y `html { scroll-padding-bottom: var(--announce-bar-h) }` mientras la vista está montada (con su clase)". Prueba: e2e a 768×1024 tabula hasta Mensaje y comprueba que no queda debajo de la barra.

### D7 · Media · Tablet vertical: ¿dos "Enviar aviso" y dónde queda la vista previa?
- **Evidencia:** §2 dice que "el resumen va después del redactor y queda sticky bottom con sólo «Va a» + Enviar". Se puede leer de dos formas: el resumen completo abajo **y además** una barra con otro Enviar (dos botones con el mismo nombre: `getByRole("button", {name: /Enviar aviso/})` truena, y hay dos primarios), o la barra **reemplaza** al resumen y la vista previa se pierde en tablet. Tampoco dice dónde salen el `role=alert` y el `role=status` en esa barra.
- **Enmienda:** §2: "< 860 px de columna: **un solo** `ComposeSummary` en el flujo, después del redactor (vista previa + Va a); **sólo su pie** (alerta/estado + Va a abreviado + el único botón Enviar) se vuelve `position: sticky; bottom: 0`. Un botón, un `role=alert`, un `role=status` en el DOM a cualquier ancho". Prueba: `getAllByRole("button", {name:/Enviar aviso/})` tiene longitud 1 en los dos anchos.

### D8 · Media · "Va a" sin tope, y textos repetidos que confunden a las pruebas
- **Evidencia:** con 12 grupos elegidos, "Va a 2° Primaria · A, 2° Primaria · B, …" empuja el botón fuera de los 768 px de alto de un iPad horizontal, que es justo P5 de regreso. Además, si el resumen repite "N seleccionados", `getByText(/2 seleccionados/)` (behavior.test.tsx:127) encuentra dos nodos y truena. Si repite el nombre del alumno como botón, pasa lo mismo con `getByRole("button",{name:"Andrade Mateo"})`.
- **Enmienda:** §3.2 `ComposeSummary`: "«Va a» muestra hasta 3 nombres y «y N más» (texto, no botón); nunca repite la cadena «seleccionados» ni crea botones; «Toda la escuela · ciclo X» para `cycle`". El contador B6 (`· N seleccionados` sólo con N > 1) se queda **en un solo lugar**: debajo de la lista, como dice §4.

### D9 · Media · La pista "Elige a quién va dirigido" es el mismo texto que el error D3
- **Evidencia:** §3.2, copy: "«Elige a quién va dirigido» como pista muda". D3 es "Elige a quién va dirigido." en `role=alert`. Dos textos iguales con dos estados distintos (pista gris contra error rojo) no se distinguen al leer de reojo, y `getByText` encuentra dos.
- **Enmienda:** cambiar la pista a "Todavía no eliges a nadie." en el hueco de "Va a". El error D3 queda igual.

### D10 · Media · El buscador de alumno no tiene definidos sus estados
- **Evidencia:** §3.2 sólo dice "hasta 8 resultados". No dice qué hay con el campo vacío, sin coincidencias, con más de 8, ni cómo se evita que el elegido aparezca dos veces (arriba y en resultados, otra vez dos botones con el mismo nombre). La línea de grupo usa `gradeGroup`, que **sólo guarda la letra** ("A") según la memoria del QA del 25-sep: "Mateo Andrade · A" no ayuda.
- **Enmienda:** tabla de estados en §3.2:
  - vacío: no hay lista; "401 alumnos · escribe nombre o apellido";
  - con texto: hasta 8 en el orden de `sortStudents`, más "N de 401 alumnos" (si hay más de 8: "Sigue escribiendo para ver los demás");
  - sin coincidencias: "Ningún alumno coincide con «{texto}»";
  - elegido: un renglón fijo arriba con `aria-pressed=true`, **excluido** de los resultados;
  - segunda línea: `level · gradeGroup` (por ejemplo "Primaria · A"), nunca `gradeGroup` solo.

### D11 · Media · Faltan los estados de carga y vacío de las listas
- **Evidencia:** A2–A4 fallan en silencio (lista vacía). Hoy, mientras carga, la caja se ve vacía. En la vista nueva, "Grupos" sin secciones o "Niveles" sin fila se lee como "no hay nada", y si se agrega un texto de vacío va a parpadear en cada carga.
- **Enmienda:** agregar a §3 la tabla de estados:

```
  PIEZA            | CARGANDO                | VACÍO / FALLÓ (indistinguibles, A2–A4)           | ÉXITO
  -----------------|-------------------------|--------------------------------------------------|------------------
  Grupos/Niveles   | 3 renglones fantasma    | "No encontramos grupos en el ciclo {nombre}."   | secciones por nivel
  Un alumno        | "Cargando alumnos…"     | "No encontramos alumnos."                        | buscador (D10)
  Ciclo (A1)       | —                       | alerta actual "No pudimos leer los ciclos."      | —
  Vista previa     | —                       | título/cuerpo en blanco → "Título del aviso" /   | texto en vivo
                   |                         | "Aquí va el mensaje" en text-faint, aria-hidden  |
  Envío (D5–D8)    | botón 50 % (D7), mismo  | alerta (D6)                                      | status D5, se queda
                   | nombre                  |                                                  | hasta el siguiente éxito (D8)
```

  El hook expone `groupsLoaded`, `levelsLoaded` y `studentsLoaded`, que salen de las mismas promesas. No agregan llamadas, así que no rompen el congelamiento.

### D12 · Media · Los errores siguen lejos del campo y el foco no se mueve (P8 sin resolver)
- **Evidencia:** §4 pone los mensajes "en `ComposeSummary` junto al botón". Eso deja "Un aviso necesita título y mensaje." a unos 400 px de los campos, en otra columna, y el foco no se mueve. `ui-forms-and-modals.md` pide un resumen arriba con el foco en él. La redacción y el orden de D1–D3 están congelados; dónde se muestran y a dónde va el foco, no.
- **Enmienda (recomendada, sólo presentación):** la alerta queda junto al botón (ahí está la mirada del que acaba de oprimir), con `tabIndex={-1}`, y recibe el foco en cada envío fallido. Para D3, el hueco "Va a" se marca en `--bad-fg`. No se dividen mensajes ni se ponen errores por campo: eso es BC4. Prueba nueva: después del envío fallido, `document.activeElement` es la alerta. *Es presentación, pero queda señalado por si el dueño lo ve como comportamiento.*

### D13 · Media · Cumpleaños dentro de `<details>`: tarjeta dentro de tarjeta, encabezado doble y un segundo lima
- **Evidencia:** `BirthdayGreeting` pinta su propia tarjeta (`marginTop 24`, borde, sombra), su estrella y su `<h2>` "Felicitación de cumpleaños" (BirthdayGreeting.tsx:124-140) con estilos en línea, que ninguna clase puede vencer. Al abrir la fila salen dos estrellas, dos títulos iguales y una tarjeta anidada (*nested-cards*). Además vuelve el botón lima "Guardar felicitación", y el "**único** lima" de §3.2 deja de ser cierto. Si A5 falla, su `role=alert` queda escondido en la fila cerrada.
- **Enmienda:** permitir **una** prop de presentación en `BirthdayGreeting`, `embedded?: boolean`, que quite la tarjeta exterior, la estrella y el `<h2>` (la fila ya los pone), con `aria-labelledby` apuntando al título de la fila. Lógica, llamadas, textos y validación quedan intactos, y `BirthdayGreeting.test.tsx` sigue verde sin tocarlo. Cambiar en §3.2 "un solo botón lima" por "un solo lima **visible con la fila cerrada**; adentro, Guardar es el primario de esa sección". Lo de A5 escondido se acepta y se anota; abrir la fila sola al fallar pediría sacar el estado del componente.

### D14 · Media · `<button>` "Editar texto" dentro de `<summary>`
- **Evidencia:** §3.2 `AutomaticMessages`: "`<details>` … con botón fantasma «Editar texto»". Un elemento interactivo dentro de `<summary>` es HTML inválido: el lector de pantalla anuncia dos controles, y Safari/VoiceOver se pierde.
- **Enmienda:** "El `<summary>` es el control. «Editar texto ⌄» es un `<span>` con estilo de botón fantasma, sin `role=button`. El ⌄ gira con `[open]`, y con `prefers-reduced-motion` no gira".

### D15 · Media · Estilos en línea contra la capa `look refresh`
- **Evidencia:** DESIGN-SYSTEM.md:87-89: "nada de estilos en línea salvo propiedades personalizadas: un estilo en línea le gana a la capa". El plan dice "`globals.css` sólo si el grid sticky lo pide". La pantalla vieja está hecha toda con estilos en línea, y copiar ese patrón deja la vista nueva con el look viejo en dev, donde `ui_refresh` está ON.
- **Enmienda:** §6: "Toda pieza nueva se estiliza con clases `.announce-*` en `globals.css`; en línea sólo `--custom-props`". §5: las pruebas de la vista nueva corren en los cuatro cruces de `module_announcements_view` × `module_ui_refresh`, en claro y oscuro. Las claves de color: chip elegido = `color-mix(in oklab, var(--lime) 12%, var(--surface))` + borde `--lime` + texto `--text` (no `--forest` sobre oscuro: ese es el 3.84:1 de P7); audiencia activa = `--surface` + `--shadow-card` sobre la cápsula `--surface-3` (patrón 4 del sistema).

### D16 · Baja · La vista previa no se parece a la notificación real
- **Evidencia:** "Así lo leen los papás" promete fidelidad, pero el plan no dice cuánto se corta.
- **Enmienda:** título en 1 renglón y cuerpo en 2 (`-webkit-line-clamp`), igual que el push de la app de padres; el texto completo se ve en el campo. Si la app corta distinto, se copia su corte (verificarlo en `apps/mobile`).

### D17 · Baja · El estado "enviando" no está en el plan
- **Evidencia:** D7 congela la opacidad al 50 % y que el botón siga siendo clicable. El plan no lo repite.
- **Enmienda:** §4, renglón D7: "botón al 50 %, mismo nombre «Enviar aviso», `aria-busy` mientras envía; sin «Enviando…» (cambiaría el nombre que buscan las pruebas a mitad del envío)". El guardia de doble clic sigue siendo BC1.

### D18 · Baja · El nuevo orden de grupos contradice la letra de B3
- **Evidencia:** behavior.md B3 dice "labels (server order)". El rediseño los agrupa y los ordena en el cliente. Es presentación (el diagnóstico lo dice), pero el contrato congelado queda contradiciendo la vista nueva.
- **Enmienda:** anotar en behavior.md B3: "el orden visual es presentación; la vista nueva agrupa por nivel/ordinal/nombre (audience.ts); el conjunto y las etiquetas no cambian".

### D19 · Baja · Movimiento: abrir `<details>` a su altura natural
- **Evidencia:** §3.3 propone animar la altura del `<details>`. Para eso hace falta `interpolate-size: allow-keywords` o `::details-content`, que no son Baseline en octubre de 2026.
- **Enmienda:** mejora progresiva con `@supports (interpolate-size: allow-keywords)`; si no hay soporte, se abre al instante. Ninguna animación toca el texto de la vista previa.

### D20 · Baja · Nota para el dueño (preexistente, fuera del rediseño): "llega sola a las 7:00"
- **Evidencia:** el texto ya existe (BirthdayGreeting.tsx:138), pero el rediseño lo sube a la fila siempre visible. Según ADR 0028 y la memoria T11, `birthday_schedule_enabled` vale `false` por omisión y el programador no está aplicado en GCP: en dev, hoy nadie recibe la felicitación.
- **Enmienda:** no tocar el texto en el rediseño. Anotar en §7 como **BC7**: aplicar el programador o matizar la promesa.

## Pase 4: riesgo de verse "hecho por IA"

Clasificación: **OPERATE** (herramienta de oficina). Rechazos duros: ninguno. La pantalla deja de ser "tarjetas apiladas" (P12) y queda en un redactor más un resumen.

| Litmus | Respuesta | Evidencia |
|---|---|---|
| 1. ¿El producto se reconoce en la primera pantalla? | SÍ | Encabezado de módulo, ícono tintado, lima del sistema. |
| 2. ¿Hay un ancla visual? | SÍ | El botón lima del resumen; es el único primario con la fila cerrada. |
| 3. ¿Se entiende leyendo sólo los títulos? | SÍ | "¿A quién va?" → "¿Qué les dices?" → "Así lo leen los papás". |
| 4. ¿Cada sección tiene un solo trabajo? | SÍ | Con D13 se cumple también al abrir la fila. |
| 5. ¿Las tarjetas se ganan su lugar? | SÍ, salvo la anidada | Ver D13. |
| 6. ¿El movimiento ayuda? | SÍ (poco) | Abrir la fila y el ✓ del chip; nada decorativo. |
| 7. ¿Se ve bien sin sombras decorativas? | SÍ | `--shadow-card` vale `none` con el look refresh. |

Detector: `numbered-section-labels` coincide en forma con los círculos "1" y "2", pero aquí sí marcan pasos reales de una tarea. Se mantienen.

## Pase 3: recorrido

| PASO | QUÉ HACE | QUÉ SIENTE | ¿LO CUBRE EL PLAN? |
|---|---|---|---|
| 1 | Entra a /avisos desde "Nuevo aviso" | Prisa: "es la junta de 2°" | Sí: Grupos por omisión, secciones por nivel |
| 2 | Busca "2° Primaria B" | Antes perdido (P2/P3); ahora se ubica | Sí, si D3 resuelve el texto del chip |
| 3 | Escribe título y mensaje | Concentrado | Sí; ojo con D1 si eligió "Un alumno" |
| 4 | Revisa "Va a" y la vista previa | Seguridad: "le llega a quien debe" | Sí, si D8 pone tope y D9 separa la pista |
| 5 | Envía y le falta algo | Antes frustrado; ahora sabe qué falta | Sólo con D12 |
| 6 | Ve "Enviado a 56 alumnos" | Alivio | Sí (D5); el mensaje se queda hasta el siguiente éxito (D8) |
| 7 | Una vez al año, edita el cumpleaños | Indiferente | Sí, abajo y plegado; D13/D14 para que no se vea parchado |

## Decisiones para el dueño (no entran al rediseño)

- **BC6 (nuevo, D3):** nombre accesible del chip igual a su texto visible ("2° A, Primaria").
- **BC7 (nuevo, D20):** aplicar el programador de cumpleaños en GCP o cambiar la promesa "llega sola a las 7:00".
- **D12:** confirmar que mover el foco a la alerta cuenta como presentación (recomendado: sí).
- **D5:** confirmar columna de 960 px (recomendado) contra el ancho completo de la maqueta.
- Siguen igual: BC1–BC5. «Quitar todos» de la maqueta es BC5 (D4).

## GSTACK REVIEW REPORT

| Run | Skill | Status | Findings |
|---|---|---|---|
| 2026-10-12 | plan-design-review (no interactiva, un modelo) | issues_found | 20: 5 altas (D1–D5), 10 medias (D6–D15), 5 bajas (D16–D20) |

- Bloqueantes: D1 (Enter en el buscador envía), D2 (✓ en el nombre accesible), D3 (texto del chip contra `aria-label`, falta `gradeName`), D4 (la maqueta trae BC5 y `isDefault`), D5 (ancho de página y prueba e2e de 960).
- Hallazgos que romperían el congelamiento si se hacen como en la maqueta: D3 (cambiar el `aria-label`), D4 («Quitar todos», «texto predeterminado»).
- OUTSIDE COVERAGE: no se corrió (decidido automáticamente). CROSS-MODEL: n/a.
- VERDICT: **NO APROBAR TODAVÍA.** Aprobable en cuanto el plan incorpore D1–D5; D6–D15 antes de construir.

**UNRESOLVED DECISIONS:**
- D5: columna de 960 px (recomendado) o ancho completo de la maqueta con la prueba e2e reescrita.
- D12: confirmar que el foco a la alerta es presentación.
- BC6 y BC7 para la lista del dueño en §7.
