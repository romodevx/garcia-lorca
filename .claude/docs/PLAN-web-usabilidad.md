# PLAN — Usabilidad web: responsive, navegación y captura

Rama: `feature/responsive-navegacion-formularios` (de `dev`) · Origen: petición del dueño
(11-sep-2026), con captura de pantalla de Configuración académica en un monitor de 2560px.

Alcance: **`apps/web` únicamente**. No toca backend, `proto/`, `gen/`, migraciones,
feature flags, ni `apps/mobile`. **No nace ningún flag nuevo.**

---

## 0. La petición, tal como llegó

1. El sistema debe ser **100% responsive**: si la pantalla es muy grande, que se ajuste
   para que se visualice todo dentro de la pantalla y sus funciones; si baja a tablet,
   igual. **Los celulares no importan** — no es un sistema para teléfono.
2. **La navegación está mal**: el botón "atrás" del navegador saca del sitio. Debería
   funcionar dentro del sitio, como en cualquier otro lado.
3. Toda **fecha** que se capture debe ser un **calendario**; el **texto en mayúsculas**; y
   los **números, si son monto, deben verse en pesos**.
4. Los **campos requeridos** marcados con `*`, y si faltan, **circularlos en rojo** para
   que sea fácil encontrar el error.
5. El **correo electrónico, sólo en minúsculas**.
6. (Añadido al revisar) Configuración **parece anidada en un componente y no aprovecha
   toda la pantalla**; revisar que ningún otro componente se comporte igual.

---

## 1. Decisiones del dueño (checkpoint, 11-sep-2026)

| # | Decisión | Elegida |
|---|---|---|
| D1 | Alcance del botón "atrás" | **URLs reales por pantalla.** Atrás/adelante, F5 conserva el lugar, el link se puede compartir. (Descartado: historial interno sin URL.) |
| D2 | Qué pasa en un monitor grande | **Ensanchar el contenido**: el tope sube de 1320 a 1800px y las rejillas meten más columnas. La tipografía **no** se escala. (Descartado: ancho completo sin tope; y escalar todo tipo zoom.) |
| D3 | Alcance de "texto en mayúsculas" | **Nombres y datos de catálogo.** NO aplica a contraseñas, ni al correo (minúsculas), ni al **cuerpo de avisos** — la mayúscula sostenida se lee como gritar. |
| D4 | Ancho mínimo que debe verse perfecto | **1024px — tablet horizontal.** Es como se usa una tablet para pasar lista o capturar calificaciones, de pie en el salón. Teléfono queda fuera. |
| D5 | Cuándo se marca el error en rojo | **Al guardar.** Todos los faltantes a la vez, la pantalla salta al primero, y el rojo se limpia en cuanto se escribe en el campo. (Descartado: al salir de cada campo; y apagar el botón Guardar.) |
| D6 | Cómo se entrega | **Un solo PR con todo.** Se le advirtió al dueño que `dev` pasa ~6 días sin mejoras y que una revisión de ese tamaño deja pasar defectos; lo aceptó. Se mitiga con commits separados por bloque. |
| D7 | Avisos y ventanilla en monitor grande | **Columna de lectura acotada (~960px), centrada**, con la página alrededor a todo lo ancho. Un párrafo de 1800px hace que el ojo pierda el renglón al regresar. |

---

## 2. Hallazgos verificados en el código

Todo lo de abajo se leyó en el repo, no se supone.

### H1 — Toda la app vive en una sola ruta

`apps/web/app/page.tsx` renderiza `<AppShell/>`, y `AppShell.tsx:89` guarda la pantalla
actual en `useState<View>`. El navegador nunca ve una segunda página, así que "atrás"
sólo puede salir del sitio. **Es la causa raíz de la petición 2**, no un síntoma.

Consecuencias hoy: F5 en cualquier módulo devuelve al tablero; ningún link es
compartible; `/finanzas` no existe como dirección.

### H2 — `app/(auth)/login/page.tsx` es un stub muerto de FASE 0

Renderiza literalmente *"Stub — la autenticación llega con el módulo IAM"*. `/login` **ya
es una ruta viva**: cualquiera que la teclee ve esa página. Nadie enlaza a ella, y por eso
nadie la ha encontrado — pero el login real vive dentro de `AppShell`, no ahí. Se borra.

### H3 — El cajón de Configuración está mal anclado (el de la captura)

`_design/SlideOver.tsx:63` declara `position: fixed; inset: 0`. Debería anclarse al
viewport. No lo hace: en `Catalog.tsx:221` los paneles —y por lo tanto sus cajones— nacen
dentro de `<div className="fade-up">`, y en `globals.css`:

```css
@keyframes fadeUp { from { transform: translateY(12px); } to { transform: none; } }
.fade-up { animation: fadeUp .5s var(--ease) both; }
```

Un ancestro con `transform` animado **crea un bloque contenedor** para sus descendientes
`fixed`. `inset: 0` deja de significar "el viewport" y pasa a significar "ese div de
1320px".

**Tres defectos, no uno:**
1. El cajón no llega al borde derecho del monitor ni al fondo de la pantalla.
2. El velo no cubre la barra superior → **con el cajón abierto se puede cerrar sesión o
   abrir el buscador**. No es modal, aunque lo aparente.
3. El velo tampoco cubre las tarjetas de estadísticas de arriba.

**Config es el único módulo afectado**, y por casualidad:

| Módulo | Dónde se monta el cajón | Resultado |
|---|---|---|
| **Config** | `Catalog.tsx:221` — **dentro** de un `.fade-up` envolvente | 🔴 atrapado en 1320px |
| Finanzas | `FinanceScreen.tsx:395/397/403/427` — *hermanos* de los `.fade-up` | ✅ viewport |
| Alumnos | `StudentsScreen.tsx:648/649` — hermanos | ✅ |
| Administración | `AdminScreen.tsx:276-282` — hermanos | ✅ |
| Uniformes | sin `.fade-up` en esa pantalla | ✅ |

Config es el único que envuelve sus paneles en un contenedor `.fade-up`; en los demás el
`.fade-up` está en hojas sueltas y el cajón queda afuera. **Nadie usa `createPortal` en
todo el repo** (verificado). Que ocho cajones se anclen bien depende de dónde alguien puso
un `<div>`: el día que a Finanzas le agreguen un `.fade-up` envolvente, un `filter` o un
`contain`, se rompe igual y en silencio.

### H4 — Nueve cajones, ocho copiados a mano, cinco anchos distintos

`SlideOver` existe y **sólo config lo usa**. Los demás lo re-implementaron línea por línea:

| Dónde | Ancho |
|---|---|
| `_design/SlideOver.tsx:69` — el único compartido (8 paneles de config) | 520 |
| `admin/AdminScreen.tsx:63` | 520 |
| `students/StudentsScreen.tsx:257` — Nuevo alumno | 520 |
| `students/StudentsScreen.tsx:337` — Ficha del alumno | 460 |
| `finance/CollectDrawer.tsx:191` — Cobrar | 520 |
| `finance/FinanceScreen.tsx:104` — Registrar pago | 480 |
| `finance/FinanceScreen.tsx:412` — Corte de caja | 520 |
| `uniforms/InventoryPanel.tsx:456` | 460 |
| `finance/StudentPicker.tsx:12` — modal centrado | 560 |

Por eso el ancho del cajón no se puede arreglar en un lugar: no hay un lugar. Además el de
uniformes usa `zIndex: 50` mientras los otros ocho usan `70`, y `SlideOver` no declara
`aria-modal` (el de uniformes sí).

El tabulador de colegiaturas —la forma más grande del sistema: inscripción multi-concepto,
tramos de precio, colegiatura, cobranza y cuotas— se captura por un túnel de 520px, tenga
el monitor 1280 o 2560.

### H5 — Cinco anchos de página distintos, sin regla

| Pantalla | Tope | % de 2560px |
|---|---:|---:|
| Avisos `AnnouncementsScreen.tsx:129` | 820 | 32% |
| Ventanilla `PaymentsScreen.tsx:99` | 900 | 35% |
| Asistencia y Calificaciones (4 archivos) | 1160 | 45% |
| Uniformes `UniformsScreen.tsx:134` | 1180 | 46% |
| Finanzas, Alumnos, Config, Admin, Docentes, Tablero | 1320 | 52% |

### H6 — Topes *dentro* de una página ya topada

| Dónde | Tope interno | Vive dentro de |
|---|---:|---|
| `finance/AccountPanel.tsx:53` | 820 | página de 1320 |
| `finance/GeneratePanel.tsx:58` | 760 | página de 1320 |
| `finance/PaymentPanel.tsx:97` | 620 | página de 1320 |
| `students/StudentDirectoryPanel.tsx:54` | 420 | página de 1320 |

Los `maxWidth: 360` de los buscadores y los 120/180 de campos numéricos **sí están bien**
— un buscador de 1800px sería absurdo. Esos se quedan.

### H7 — Los anchos están en estilos en línea, y por eso hoy no hay responsive posible

Un estilo en línea le gana a cualquier `@media`. Este repo ya lo aprendió una vez y lo
dejó documentado en `globals.css` a propósito de `.topbar-search`:

> *"These were inline on the element until the phone media query needed to override them
> and could not: an inline style beats an author rule, so the box kept its desktop width
> on a 412px screen and crushed its own placeholder to 0px."*

El paso uno del bloque responsive **no es agregar media queries; es sacar los anchos del
JSX**.

### H8 — No hay una capa de formulario

~50 inputs con estilos en línea repartidos en 23 archivos. Existen primitivas decentes
(`TextField`, `NumberField`, `DateField`, `SelectField`) pero **escondidas en
`_screens/config/parts.tsx`**, sin `required` y sin estado de error.

- Sólo **3** inputs usan `type="date"`. El resto se teclea como texto — p. ej.
  `CalendarPanel.tsx:244` (`placeholder="2026-09-20"`) y `TuitionPanel.tsx:578`
  (*"Fecha límite de inscripción"*, `placeholder="2026-06-30"`). Hay incluso un mensaje de
  error que dice *"se escribe aaaa-mm-dd"* (`TuitionPanel.tsx:507`).
- Sólo **2** ocurrencias de `required` en toda la app web.
- El formateo de dinero está **triplicado**: `finance/money.ts:28` (bueno, probado),
  `Dashboard.tsx:14` (redondea a pesos enteros) y `uniforms/parts.tsx:17`.

### H9 — Deuda anotada, fuera de alcance

El toast está copiado a mano **7 veces** aunque `_design/Toast.tsx` existe. No se toca en
este PR; queda registrado.

---

## 3. Diseño

### Bloque A — Navegación por URL

**La URL pasa a ser el estado.** Se borra `app/page.tsx` y nace
`app/[[...slug]]/page.tsx` (catch-all opcional) que renderiza `<AppShell/>`. Las rutas más
específicas que ya existen (`/activar`, `/healthz`, `/api/*`) ganan sobre el catch-all, que
es como Next resuelve; no hay conflicto. `output: "standalone"` (no export estático), así
que el catch-all no necesita `generateStaticParams`.

Un módulo **puro y probable**, `app/routing/routes.ts`, traduce en ambos sentidos:

| Ruta | Pantalla |
|---|---|
| `/` | tablero |
| `/entrar` | login |
| `/cambiar-contrasena` | cambio forzado de contraseña |
| `/finanzas` | adeudos |
| `/pagos` | ventanilla |
| `/alumnos` | alumnos |
| `/calificaciones` | calificaciones |
| `/asistencia` | asistencia |
| `/docentes` | docentes |
| `/uniformes` | uniformes |
| `/avisos` | avisos |
| `/configuracion` | catálogo académico |
| `/administracion` | usuarios y roles |
| `/modulo/<id>` | módulo sin backend aún (`ModulePlaceholder`) |

Next App Router ya habla con el historial del navegador, así que **atrás, adelante y F5
salen gratis** una vez que la URL manda. No se escribe `pushState` a mano.

Tres consecuencias que caen de aquí y hay que implementar explícitamente:

1. **F5 en `/finanzas`** se queda en Finanzas. Hoy devuelve al tablero.
2. **Entrar a `/finanzas` sin sesión** → login, y al autenticarse **aterriza en Finanzas**,
   no en el tablero. El destino pendiente se guarda en memoria del cliente (no en la URL,
   para no ofrecer un `?next=` como vector de redirección abierta).
3. **Módulo con flag OFF** sigue mostrando `ModulePlaceholder` **en su propia URL**. El
   404 del gateway sigue siendo la frontera de seguridad; esto es sólo UX.

**Fuera de alcance (explícito):** cajones y pestañas **no** entran al historial. El dueño
eligió la opción sin ellos. Si más adelante los quiere, es un cambio aditivo sobre
`routes.ts`.

**Se borra** `app/(auth)/login/page.tsx` (H2).

### Bloque B — Una capa de formulario

Las primitivas se promueven de `_screens/config/parts.tsx` a **`app/_design/form/`** y
**todos** los inputs del sistema pasan por ahí:

| Primitiva | Qué garantiza |
|---|---|
| `TextField` | `uppercase` opcional → nombres, conceptos, grados, materias, referencias |
| `EmailField` | **siempre minúsculas**, `autoCapitalize="none"`, `spellCheck={false}`, `inputMode="email"` |
| `DateField` | `type="date"` → calendario nativo; se ve `dd/mm/aaaa`, guarda `aaaa-mm-dd` |
| `MoneyField` | prefijo `$`, al salir del campo formatea `1,850.00`, entrega **centavos enteros** |
| `TextArea` | **sin** mayúsculas — el cuerpo de un aviso a los papás no se grita (D3) |
| `SelectField`, `NumberField` | la misma disciplina de `*` y de error |

**La transformación ocurre en `onChange`, no al pintar.** Lo que se ve es lo que se guarda.
Guardar tal cual y sólo pintar en mayúsculas dejaría *"Ana Ruiz"* y *"ana ruiz"* idénticos
en pantalla y distintos en la base.

**Requeridos.** La etiqueta pinta `Nombre del alumno *` con el asterisco en `var(--red)` y
el input lleva `aria-required`. El `*` es decorativo para el lector de pantalla
(`aria-hidden`), que ya recibe la obligatoriedad por `aria-required`.

**Error (D5).** Al **Guardar**: se marcan todos los faltantes a la vez con borde
`var(--red)` + halo, `aria-invalid`, `aria-describedby` apuntando al mensaje
(`⚠ Falta la fecha de nacimiento`), la vista salta al primero
(`scrollIntoView({block:"center"})` + `focus()`), y arriba aparece un resumen
`role="alert"` — *"Faltan 2 datos"*. El rojo de cada campo se limpia en su `onChange`.

Un hook pequeño y puro, `app/_design/form/useRequired.ts`, concentra esa lógica para que
no se re-implemente 23 veces. Su contrato: recibe los valores y la lista de obligatorios,
devuelve `{ errors, check(), clear(key) }`. Sin React fuera de `useState`, unitariamente
probable.

**Dinero.** `finance/money.ts` ya es sólido y está bien probado: se **mueve** a
`app/_lib/money.ts` sin cambiar su lógica, y **mueren los dos formateadores duplicados**
(`Dashboard.tsx:14`, `uniforms/parts.tsx:17`). Todo monto se pinta `$1,850.00`. Los
centavos enteros siguen siendo lo único que cruza al API — ningún float regresa.

**Muere también** el mensaje *"la fecha límite se escribe aaaa-mm-dd"*: con calendario, ese
error deja de poder existir.

### Bloque C — Responsive (1024 → 2560) y el cajón

**C1. Los anchos salen del JSX (H7).** Tokens en `globals.css`:

```css
:root                       { --page-max: 1320px; --page-pad: 28px; }
@media (min-width: 1600px)  { :root { --page-max: 1800px } }
@media (max-width: 1180px)  { :root { --page-pad: 18px  } }
```

Un componente `<Page>` en `app/_design/Page.tsx` reemplaza las 12 copias de
`maxWidth: …, margin: "0 auto"`, con dos variantes:

- `wide` (predeterminada): tableros, tablas, catálogos. Usa `--page-max`.
- `reading` (D7): Avisos y Ventanilla. La **página** va a todo lo ancho; sólo la columna
  de captura se acota a ~960px y se centra.

Los cuatro topes anidados de H6 se borran.

**C2. Rejillas.** Las de tarjetas (`auto-fit minmax(340px,1fr)`) ya sirven: a 1800px dan 5
columnas solas, sin tocarlas. Las de formulario `1fr 1fr` pasan a
`auto-fit minmax(240px,1fr)` para que se apilen a 1024. Las **tablas de columnas fijas**
(`2.4fr 1.1fr 1.6fr 1fr auto` en finanzas/alumnos/administración) pasan a clases CSS y a
≤1100px sueltan las columnas secundarias en lugar de aplastarse.

**C3. El cajón, arreglado de raíz (H3 + H4).** Un solo `SlideOver`:

- Se renderiza con **`createPortal` a `document.body`**. Desde ahí ningún ancestro puede
  volver a capturarlo — mata H3 y lo vuelve irrepetible. No es un patrón inventado: es el
  que la paleta ⌘K ya usa, y por eso ella sí funciona.
- Recibe `size` en vez de un número pegado: `sm` (fichas, lectura) · `md` (altas simples)
  · `lg` (formularios grandes).
- **`lg` responde al monitor**: `min(920px, 100%)`, y a partir de 1600px la forma se
  acomoda **en dos columnas**. El tabulador de colegiaturas deja de ser un túnel.
- Declara `aria-modal="true"`, atrapa el foco, y cierra con `Esc`.
- Los **8 clones se borran** y todos entran por él. `zIndex` se normaliza a 70.

---

## 4. Pruebas (TDD — primero en rojo)

**Unitarias (Vitest)**
- `routes.ts` en ambos sentidos, incluida una ruta desconocida.
- `useRequired`: detecta faltantes, limpia al escribir, no reporta opcionales vacíos.
- `money.ts`: la batería que ya existe, movida intacta.
- Cada primitiva: `TextField uppercase` transforma en `onChange`; `EmailField` baja a
  minúsculas; el `*` se pinta y el input lleva `aria-required`; al fallar aparece
  `aria-invalid` y el mensaje queda ligado por `aria-describedby`; `DateField` es
  `type="date"`.

**e2e (Playwright)** — jsdom no aplica CSS, así que todo lo visual **sólo** se puede
defender en un navegador real.
- **Navegación:** atrás, adelante, F5 sobre un módulo, link directo, y link directo sin
  sesión → login → aterriza en el módulo pedido.
- **Cajón:** el velo cubre el viewport completo **incluida la barra superior** (la prueba
  que habría atrapado H3), y el cajón toca el borde derecho de la pantalla.
- **Dos proyectos nuevos**, `tablet` (1024×768) y `wide` (2560×1440), que recorren cada
  módulo afirmando **cero scroll horizontal** — reusando la afirmación de overflow que ya
  existe en `e2e/narrow.mobile.spec.ts` — y, en `wide`, que el ancho **de verdad se
  aprovecha**.
- **Requeridos:** guardar una forma vacía marca los faltantes, salta al primero, y el rojo
  se limpia al escribir.

El proyecto `mobile` (Pixel 7) que ya existe **se conserva**: el almacén de uniformes se
trabaja desde el teléfono entre los estantes. El teléfono queda fuera del *alcance nuevo*,
no de la suite.

---

## 5. Lo que este PR NO toca

Backend · contratos `proto/` · `gen/` · migraciones · feature flags · `apps/mobile` ·
el toast duplicado 7 veces (H9, deuda anotada) · cajones y pestañas en el historial.

---

## 6. Riesgos

| Riesgo | Mitigación |
|---|---|
| El catch-all choca con `/activar`, `/healthz` o `/api/*` | Next prefiere la ruta más específica; se verifica con e2e sobre las tres antes de seguir |
| Migrar 50 inputs en 23 archivos rompe algo silenciosamente | Cada archivo migrado corre su suite; las primitivas se prueban antes de migrar |
| Un solo PR grande deja pasar defectos (D6, advertido) | Commits separados por bloque; los tres bloques de prueba corren en cada uno |
| `e2e` de ancho es frágil frente a la barra de scroll | Se afirma `scrollWidth - clientWidth <= 1`, el margen que la suite existente ya usa |
