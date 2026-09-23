# Campos vacíos con ayuda al lado

**Fecha:** 2026-09-23 · **Rama:** `feature/campos-vacios-con-ayuda` · **Alcance:** `apps/web` (+ un placeholder en `apps/mobile`)

## El problema, en las palabras del dueño

> "Necesito que los campos estén vacíos sin ejemplos para que al momento de capturar no se
> confundan y piense el director o cualquiera que capture que ya tienen información."

Hoy el sistema pinta **valores de ejemplo** dentro de los campos: `ADA`, `LOVELACE`,
`3200.00`, `XXXX000000XXXXXX00`, `1° A`, `PRIMARIA`, `director@example.com`. En gris claro,
dentro del recuadro, indistinguibles de un dato capturado para quien no diseñó la pantalla.
Quien captura duda, y en el peor caso **guarda sin escribir**, creyendo que el campo ya
traía algo.

La ayuda no desaparece: se muda a un `(?)` chiquito junto a la etiqueta, que se abre al
pasar el cursor, al llegar con Tab o al tocarlo en tableta.

## Inventario verificado

De los ~95 `placeholder=` de `apps/web`:

| Clase | Cuántos | Qué se hace |
|---|---|---|
| Valor de ejemplo dentro de un campo de captura | ~57 | **Se borra.** Su contenido se reescribe como texto del `(?)` |
| Texto de la `<option>` vacía de un `<select>` | 28 | **Se borra el texto.** La opción vacía se queda, sin letras |
| Buscadores (no capturan nada, no guardan nada) | 8 | **Se quedan** |
| Plomería de la propia capa (`fields.tsx`, `MoneyField`) | resto | Se elimina con la prop |

Dos hechos que cambian el diseño y que se verificaron en el código, no se supusieron:

1. **Ninguna pantalla pasa `hint` a un campo.** Los 8 `hint=` que existen en `_screens/`
   son de estados vacíos (`AllClear` / `EmptyHint`) y no se tocan. `Field.hint` es hoy
   código muerto con una sola prueba: convertirlo en `help` no obliga a migrar nada.
2. **`InlineCreateRow` no tiene encabezado de tabla.** Sus celdas sólo llevan `aria-label`
   y el placeholder. Borrarlo a secas dejaría la celda **muda**.

## Decisiones del dueño

1. Se borran los valores de ejemplo **y** el texto de la `<option>` vacía del select.
2. El `(?)` responde a **hover + foco (Tab) + tap**, no sólo a hover.
3. Los `hint` de la capa de captura desaparecen: todo pasa al `(?)`.
4. Los textos los redacta el equipo, campo por campo, según lo que cada campo *es*.

---

## Diseño

### 1. `HelpTip` — primitiva nueva (`app/_design/form/HelpTip.tsx`)

Un `<button type="button">` de 16 px con un `?` en círculo. Área táctil de 44×44 px por
padding transparente (regla global de formas: objetivos ≥ 44×44).

- **Abre** con `mouseenter`, `focus` y `click`/tap.
- **Cierra** con `mouseleave`, `blur`, `Escape` o clic fuera.
- El puntero **puede entrar a la burbuja** sin que se desvanezca.
  (WCAG 2.2 · 1.4.13 *Content on Hover or Focus*: descartable, señalable, persistente.)
- Posición `fixed`, calculada desde el rect del disparador, para que el `overflow` de un
  cajón no la recorte. Se voltea hacia arriba si no cabe abajo.
- La burbuja visible es `aria-hidden`.

**Lo que oye un lector de pantalla no depende de ningún gesto.** El texto vive **siempre**
en el DOM, en un `<span>` oculto visualmente, y ese `id` es el `aria-describedby` **del
input** — no del botón. Al llegar al campo con Tab la ayuda se lee sola, esté la burbuja
abierta o cerrada.

```
Nombre del nivel *  (?)
                     └── hover · Tab · tap
        ┌──────────────────────────────────────┐
        │ El nivel educativo: preescolar,      │
        │ primaria, secundaria.                │
        └──────────────────────────────────────┘
```

### 2. `Field` — `hint` muere, nace `help`

- `FieldProps.hint?: string` → `FieldProps.help?: string`.
- El `(?)` va en el renglón de la etiqueta, después del asterisco de obligatorio.
- Desaparece el párrafo gris de abajo. `Field` ya no pinta texto bajo el campo salvo el error.
- Prioridad de `aria-describedby`: **error > help**. Cuando algo está mal, lo que hay que
  leer es qué está mal.

### 3. `placeholder` deja de existir en la capa de captura

No se vacía: **se elimina de `BaseFieldProps`**. Eso convierte al compilador en el
barrendero — TypeScript revienta en los ~57 puntos de llamada y se corrigen uno por uno, en
vez de confiar en un `grep`. Y ninguna pantalla futura puede reintroducir un ejemplo, porque
la prop ya no está.

- `MoneyField`: se va su `placeholder ?? "0.00"` por omisión.
- `DateField`: ya lo excluía (`Omit<BaseFieldProps, "placeholder">`); el `Omit` se retira por
  innecesario.

### 4. `SelectField` — la opción vacía se queda, sin letras

`SelectField` deja de recibir `placeholder` y **siempre** emite `<option value="" />` como
primera opción, sin texto. Es lo que impide el defecto que el dueño rechazó explícitamente:
un `<select>` sin opción vacía queda preseleccionado en la primera opción y **guarda un dato
que nadie eligió**.

Hoy 28 de los 30 `SelectField` traen `placeholder`. Los **dos** que no —el ciclo de
`Attendance.tsx:65` y el parentesco de `ContactsPanel.tsx:186`— ganan la opción vacía; en
ambos es correcto (el parentesco es una elección obligatoria; el ciclo trae valor por
omisión y sólo mostraría el hueco si alguien lo vacía).

### 5. Selects dependientes: apagados, no vacíos

Borrar el texto pierde avisos como `"Elige un nivel primero"`
(`StudentsScreen.tsx:353`). En su lugar: **el select queda `disabled` mientras su campo
padre esté vacío**, y su `(?)` dice de qué depende. Un desplegable apagado comunica más que
uno encendido y vacío.

Afectados: Grado (depende de Nivel), Grupo (depende de Ciclo) en `EnrollPanel`,
`RosterPanel`, `EnrollmentRow`, `AssignmentsPanel`.

### 6. `InlineCreateRow` — etiqueta visible por celda

`InlineColumn.placeholder?: string` → `InlineColumn.help?: string`.

Al abrirse el renglón, cada celda pinta **su etiqueta visible arriba** (12 px, `--text-muted`)
con su `(?)` al lado. El renglón crece ~18 px sólo mientras está abierto. Además de resolver
la celda muda, esto cumple la regla global que el renglón incumplía hoy: *"Visible label
above every field. A placeholder is never a label."*

### 7. Los textos van por i18n donde la pantalla ya usa i18n

Las pantallas de `finance/` y `attendance/` rotulan con `t(...)`. Sus textos de ayuda entran
como claves nuevas `help.<campo>` en `packages/i18n/src/messages/{es,en}/`, y la prueba de
paridad del paquete obliga a que existan en los dos idiomas. Las claves
`option.choose` (finance) y `option.chooseGroup` (attendance) quedan sin uso y **se retiran**.
El resto de las pantallas llevan el texto en línea, como ya llevan su etiqueta.

---

## Fuera de alcance, y por qué

- **Buscadores (8).** `"Buscar alumno, CURP o tutor…"` no es un dato falso: es una
  instrucción, el campo no guarda nada y no hay confusión posible con un registro.
  Se quedan: `AdminScreen:219`, `FinanceScreen:369`, `StudentsScreen:628`,
  `PaymentsScreen:119`, `OrdersPanel:65`, `StudentPicker:85` (autocompletado de alumno),
  y el buscador de `TopBar` / paleta de comandos.
- **App móvil de padres.** Sólo captura en Login. Se le quita el `"tu@correo.com"` de
  `LoginScreen.tsx:92` y nada más; el `"••••••••"` de la contraseña no es un ejemplo de dato.
- **`Dashboard.tsx:171`** es un comentario con la palabra "placeholder". No se toca.

## Consecuencias que hay que aceptar

1. **`StudentDirectoryPanel:56`** usa un `TextField` con etiqueta "Buscar" como buscador.
   Al perder el placeholder, su instrucción (`"Nombre, CURP, grupo o tutor…"`) pasa al `(?)`.
2. **`ChangePassword.tsx`** usa hoy el placeholder *como* etiqueta (no hay etiqueta visible).
   Se le ponen etiquetas visibles. Es una corrección de accesibilidad que entra con este
   trabajo porque el trabajo la destapa.
3. **Un select del que ya se eligió algo puede volver a vaciarse** con la opción en blanco.
   Es el comportamiento de hoy en 28 de 30 selects; se vuelve uniforme.
4. **`CounterSalePanel:309`** usa hoy el placeholder como etiqueta y además la cambia sola
   (`"Quién compró (requerido)"` sin alumno, `"Nota (opcional)"` con alumno), mientras su
   `aria-label` dice siempre "Nota" — o sea que un lector de pantalla oye lo contrario de lo
   que se ve. Gana **etiqueta visible dinámica**, y el `aria-label` se va con ella.

---

## Los textos, campo por campo

Todo lo de esta tabla es propuesta: táchese o reescríbase antes de tocar código.

### Alumnos

| Archivo:línea | Campo | Se borra | Texto del `(?)` |
|---|---|---|---|
| `StudentCreatePanel:80`, `StudentsScreen:312` | CURP | `XXXX000000XXXXXX00` | Los 18 caracteres de la CURP del alumno, tal como vienen en el acta. Es lo que impide que el mismo alumno se registre dos veces. |
| `StudentCreatePanel:81`, `StudentsScreen:323` | Nombre | `ADA` / `ANA SOFÍA` | Sólo los nombres de pila, sin apellidos. |
| `StudentCreatePanel:82`, `StudentsScreen:333` | Apellidos | `LOVELACE` / `RUIZ MÁRQUEZ` | Los dos apellidos: primero el paterno, luego el materno. |
| `StudentCreatePanel:83`, `StudentsScreen:344` | Nivel | `Selecciona un nivel…` | El nivel educativo donde va a cursar: preescolar, primaria o secundaria. |
| `StudentsScreen:353` | Grado | `Elige un nivel primero` | El grado dentro del nivel. Se enciende en cuanto eliges el nivel. |
| `StudentCreatePanel:84` | Grado y grupo | `1° A` | El grado y la letra del salón, juntos. |
| `StudentCreatePanel:85`, `StudentsScreen:365` | Tutor | `NOMBRE DEL TUTOR` | Quién responde por el alumno ante el colegio: nombre completo de la madre, el padre o el tutor legal. |
| `StudentDirectoryPanel:56` | Buscar | `Nombre, CURP, grupo o tutor…` | Puedes escribir el nombre, la CURP, el grupo o el nombre del tutor. |
| `ContactsPanel:174` | Nombre del contacto | `NOMBRE` | El nombre completo de a quién se le marca en una urgencia. |
| `ContactsPanel:180` | Teléfono del contacto | `Teléfono` | El número al que se marca, a diez dígitos. |
| `ContactsPanel:183` | Correo del contacto | `correo@ejemplo.mx` | El correo de este contacto. Puede quedar vacío. |
| `ContactsPanel:186` | Parentesco | — (gana opción vacía) | Qué es esta persona del alumno: madre, padre, abuela, tutor legal. |
| `EnrollPanel:86` | Alumno | `Selecciona un alumno…` | El alumno que vas a inscribir. |
| `EnrollPanel:87`, `RosterPanel:103` | Ciclo | `Selecciona un ciclo…` | El ciclo escolar sobre el que trabajas. |
| `EnrollPanel:93`, `RosterPanel:109`, `EnrollmentRow:80` | Grupo | `Selecciona un grupo…` | El salón. Se enciende en cuanto eliges el ciclo, y sólo muestra los grupos de ese ciclo. |
| `EnrollmentRow:83` | Motivo del traslado | `Motivo` | Por qué cambia de grupo. Queda escrito en el historial del alumno. |

### Docentes y asignaciones

| Archivo:línea | Campo | Se borra | Texto del `(?)` |
|---|---|---|---|
| `TeachersPanel:66` | Nombre | `ADA` | Sólo los nombres de pila del docente, sin apellidos. |
| `TeachersPanel:67` | Apellidos | `LOVELACE` | Los dos apellidos: primero el paterno, luego el materno. |
| `TeachersPanel:70` | Correo | `ada@example.com` | El correo donde el docente recibirá su acceso. Puede quedar vacío y agregarse después. |
| `AssignmentsPanel:167` | Ciclo | `Selecciona un ciclo…` | El ciclo escolar al que pertenece esta asignación. |
| `AssignmentsPanel:174` | Grupo | `Selecciona un grupo…` | El salón que le toca. Se enciende en cuanto eliges el ciclo. |
| `AssignmentsPanel:182` | Materia | `Selecciona una materia…` | La materia que va a impartir en ese grupo. |
| `AssignmentsPanel:190` | Profesor | `Selecciona un profesor…` | Quién la imparte. Sólo aparecen los docentes dados de alta. |
| `TeacherLinkPanel:98` | Profesor | `Selecciona un profesor…` | El docente del expediente, el que da clases. |
| `TeacherLinkPanel:99` | Usuario | `Selecciona un usuario…` | La cuenta con la que ese docente entra al sistema. Enlazarlos es lo que le deja capturar sus calificaciones. |

### Configuración del ciclo

| Archivo:línea | Campo | Se borra | Texto del `(?)` |
|---|---|---|---|
| `CycleBar:146` | Nombre del ciclo | `2026-2027` | Cómo se llama el ciclo escolar. Es lo que verás en cada filtro del sistema. |
| `LevelsPanel:107` | Nombre del nivel | `PRIMARIA` | El nivel educativo: preescolar, primaria, secundaria. |
| `LevelsPanel:108` | Clave SEP | `PRI` | La clave corta con la que la SEP identifica este nivel. Puede quedar vacía y ponerse después. |
| `GradeLevelsPanel:74` | Nombre | `1° PRIMARIA` | El grado dentro del nivel, como lo nombra el colegio. |
| `GroupsPanel:72` | Nombre del grupo | `A` | La letra o el nombre que distingue este salón de los otros del mismo grado. |
| `GroupsPanel` (Cupo) | Cupo | — | Cuántos alumnos caben. La asignación automática no mete a nadie de más. |
| `SubjectsPanel:45` | Nueva materia | `MATEMÁTICAS` | La materia como aparecerá en la boleta del alumno. |
| `TermsPanel:74` | Nombre | `T1` | Cómo se llama el periodo de evaluación. Es lo que verán los docentes al capturar. |
| `CalendarPanel:195` | Nombre (día cerrado) | `INDEPENDENCIA` | Por qué no hay clases ese día. Aparecerá en el calendario de las familias. |
| `CalendarPanel:240` | Actividad | `FESTIVAL DE PRIMAVERA` | Qué pasa ese día. Aparecerá en el calendario de las familias. |

### Colegiaturas y cuotas

| Archivo:línea | Campo | Se borra | Texto del `(?)` |
|---|---|---|---|
| `TuitionPage:284` | Colegiatura mensual | `3200.00` | Lo que se cobra cada mes por alumno en este nivel, antes de descuentos y recargos. |
| `TuitionPage:285` | Mensualidades | `10` | Cuántos meses se cobran a lo largo del ciclo. |
| `TuitionPage:286` | Mes de inicio | `9` | El mes en que arranca el cobro, con número: 1 es enero y 12 es diciembre. |
| `TuitionPage:292` | Día límite | `10` | El día del mes hasta el que se puede pagar sin recargo. |
| `TuitionPage:331` | Recargo por día | `10.00` | Lo que se suma por cada día de atraso, pasado el día límite. |
| `TuitionPage:332` | Pronto pago (monto) | `100.00` | El descuento para quien paga dentro del plazo de abajo. |
| `TuitionPage:333` | Pronto pago (días hábiles) | `5` | Cuántos días hábiles antes del día límite hay que pagar para ganarlo. Sábados y domingos no cuentan. |
| `tuition-sections:145` | Nuevo concepto | `GASTOS ADM Y SEGURO` | Cómo se llamará este cobro en el estado de cuenta de la familia. |
| `tuition-sections:278` | Monto | `2200.00` | Lo que se cobra por este concepto, por alumno. |
| `tuition-sections:438` | Nombre de la cuota | `CUOTA DE JULIO` | Cómo se llamará esta cuota en el estado de cuenta de la familia. |
| `tuition-sections:439` | Monto total | `1600.00` | El total de la cuota. Abajo eliges en cuántas partes se parte. |

### Finanzas (textos por i18n, `es` + `en`)

| Archivo:línea | Campo | Se borra | Texto del `(?)` |
|---|---|---|---|
| `ConceptsPanel:82` | Monto | `0.00` | Lo que se cobra por este concepto. |
| `GeneratePanel:84` | Monto (excepción) | `0.00` | Déjalo vacío para cobrar el monto del concepto. Escribe una cantidad sólo si este cobro es distinto. |
| `PaymentPanel:110` | Monto | `0.00` | Lo que la familia está entregando ahora. Puede ser menos que el adeudo: queda como abono. |
| `ChargeActions:93` | Descuento | `0.00` | Lo que se le rebaja a este cargo. El adeudo queda en el monto menos este descuento. |
| `ChargesPanel:77,86`, `AccountPanel:62`, `GeneratePanel:66,73,80`, `PaymentPanel:107` | filtros | `Selecciona…` | *(por filtro: ciclo, concepto, alumno)* El ciclo / el concepto / el alumno sobre el que estás trabajando. |
| `CollectDrawer:311`, `FinanceScreen:141` | Referencia | `Folio, transferencia…` | El folio del voucher o el número de la transferencia: lo que te permita reconocer este pago después. En efectivo puede quedar vacío. |

### Uniformes

| Archivo:línea | Campo | Se borra | Texto del `(?)` |
|---|---|---|---|
| `CatalogPanel:244` | Nombre | `SUÉTER VERDE` / `LIBRO DE MATEMÁTICAS 3°` | Cómo se llama la prenda o el libro. Es lo que verá la familia. |
| `CatalogPanel:248` | Clave | `SUE-VER` | Un código corto y único para encontrarla rápido en el mostrador. |
| `CatalogPanel:278` | Precio | `650.00` | Lo que cuesta una pieza. |
| `CatalogPanel:345` | Tallas | `4-16 · CH, M, G · 4, 6, 8` | Las tallas separadas por coma. Acepta números (4, 6, 8), rangos con guion (4-16) y letras (CH, M, G). |
| `CatalogPanel:379` | Mínimo | `0` | Cuántas piezas quieres tener siempre. Cuando el inventario baje de aquí, la talla aparece en el aviso de existencias bajas. |
| `InventoryPanel:317` | Piezas (por talla) | `0` | Cuántas piezas de esta talla están entrando. |
| `InventoryPanel:333` | Nota | `Factura, proveedor…` | De dónde vino la mercancía: el proveedor, el número de factura. Queda en el historial. |
| `InventoryPanel:396` | Ajuste | `-2 para restar, 3 para sumar` | Cuánto cambia la existencia, con signo: un número negativo resta piezas y uno positivo las suma. |
| `InventoryPanel:406` | Nota | `Merma, conteo físico…` | Por qué se ajusta. Un ajuste sin explicación no se puede auditar después. |
| `CounterSalePanel:309` | Nota / Quién compró | dinámico | *(sin alumno)* Anota quién se llevó la mercancía: sin eso la venta no se puede reclamar después. · *(con alumno)* Lo que quieras recordar de esta venta. |
| `CounterSalePanel:342` | Referencia | `Referencia o folio del voucher` | El folio del voucher o el número de la transferencia. |

### Calificaciones y asistencia (textos por i18n donde aplica)

| Archivo:línea | Campo | Se borra | Texto del `(?)` |
|---|---|---|---|
| `CapturePanel:104`, `TeacherCapture:157` | Asignación | `Selecciona una asignación…` | La materia y el grupo que vas a calificar. Sólo salen las que tienes asignadas. |
| `CapturePanel:111`, `TeacherCapture:164` | Periodo | `Selecciona un periodo…` | El periodo de evaluación que estás capturando. |
| `Attendance:71`, `TeacherAttendance:183` | Grupo | `Selecciona un grupo…` | El salón cuya lista vas a pasar. |
| `Attendance:65` | Ciclo | — (gana opción vacía) | El ciclo escolar sobre el que trabajas. |

### Usuarios, roles y acceso

| Archivo:línea | Campo | Se borra | Texto del `(?)` |
|---|---|---|---|
| `AdminScreen:306,346` | Correo | `nombre@garcialorca.edu.mx` | El correo con el que esta persona entrará al sistema. |
| `AdminScreen:307,347` | Nombre para mostrar | `NOMBRE DEL USUARIO` | El nombre que verán los demás en el sistema. |
| `AdminScreen:310` | Contraseña temporal | `La cambiará en el primer ingreso` | Con esto entra la primera vez. El sistema le pedirá cambiarla en ese primer ingreso. Mínimo 8 caracteres. |
| `AdminScreen:370` | Nombre del rol | `COORDINACIÓN, DOCENTE…` | Cómo se llama el puesto dentro del colegio. Los permisos se eligen abajo. |
| `UsersPanel:158` | Correo | `director@example.com` | El correo con el que esta persona entrará al sistema. |
| `UsersPanel:159` | Nombre para mostrar | `Directora` | El nombre que verán los demás en el sistema. |
| `UsersPanel:166` | Contraseña temporal | `El usuario la cambia…` | Con esto entra la primera vez. El sistema le pedirá cambiarla en ese primer ingreso. Mínimo 8 caracteres. |
| `RolesPanel:128` | Nombre del rol | `Coordinación académica` | Cómo se llama el puesto dentro del colegio. Los permisos se eligen abajo. |
| `AnnouncementsScreen:197` | Título | `Junta de padres` | Lo que las familias verán en la lista de avisos, antes de abrirlo. |
| `AnnouncementsScreen:206` | Mensaje | `Jueves 8:00 en el auditorio.` | El aviso completo. Llega tal cual a la app de los padres. |

### Entrada al sistema (fuera de la capa)

| Archivo:línea | Campo | Se borra | Qué se hace |
|---|---|---|---|
| `Login:157` | Correo | `correo@colegio.mx` | Sólo se borra. La etiqueta visible "Correo" ya existe; no lleva `(?)`. |
| `Login:296` | Correo (recuperación) | `tucorreo@garcialorca.edu.mx` | Sólo se borra. El párrafo de arriba ya explica. |
| `ChangePassword:49,50,51` | 3 contraseñas | los tres | Se borran **y se les pone etiqueta visible**, que hoy no tienen. `(?)` en "Nueva contraseña": Mínimo 8 caracteres. |
| `mobile/LoginScreen:92` | Correo | `tu@correo.com` | Sólo se borra. |

---

## Pruebas — primero las rojas (TDD)

### `HelpTip.test.tsx` (nueva)
1. No hay burbuja al montar.
2. `mouseenter` en el `(?)` la abre; `mouseleave` la cierra.
3. Tab hasta el `(?)` la abre; `blur` la cierra.
4. `click` la abre y la deja fija; `Escape` la cierra.
5. El puntero puede moverse del `(?)` a la burbuja sin que se cierre.
6. **El texto de ayuda está en el DOM con la burbuja cerrada**, y el `aria-describedby`
   del *input* apunta a él.

### `Field.test.tsx` (se reescribe)
7. Con `help`, aparece el botón `(?)`; sin `help`, no.
8. `Field` ya no pinta párrafo de ayuda debajo del campo.
9. Con `error` **y** `help`, el `aria-describedby` apunta al error.

### `fields.test.tsx` / `MoneyField.test.tsx`
10. Ningún campo de la capa emite atributo `placeholder`.
11. `SelectField` con `value=""` no deja preseleccionada la primera opción real.
12. `SelectField` emite una primera `<option value="">` de texto vacío.

### `InlineCreateRow.test.tsx`
13. Con el renglón abierto, cada celda muestra su etiqueta visible.
14. Una columna con `help` muestra su `(?)`; sin `help`, no.

### `adoption.test.ts` (regla nueva, sobre los 60 archivos)
15. **Ningún `placeholder=` en `app/_screens/**`**, salvo un input de búsqueda
    (`type="search"`, o `aria-label` que empiece con "Buscar", o el autocompletado de
    `StudentPicker`). Una pantalla nueva que ponga un ejemplo falla el día que se escribe.

### Paridad de i18n
16. Toda clave `help.*` nueva existe en `es` y en `en` (lo obliga `parity.test.ts`).

### E2E
17. El *spec* de alta de alumno sigue verde: es la prueba de que quitar los placeholders no
    rompió ningún selector que dependiera de ellos.

## Criterios de aceptación

- [ ] `grep -rn 'placeholder' apps/web/app/_screens` sólo devuelve buscadores.
- [ ] `placeholder` no existe como prop en `app/_design/form/`.
- [ ] Los 30 `SelectField` arrancan en una opción vacía sin texto.
- [ ] Todo campo con `(?)` funciona con hover, con Tab y con tap, y cierra con Esc.
- [ ] `pnpm --filter @garcia-lorca/web test` y `lint` y `build` en verde.
- [ ] Los E2E de alta de alumno, cobro y captura de calificaciones en verde.

## Lo que cambió al construirlo (2026-09-23)

El diseño resistió, salvo en tres puntos que sólo se ven con el navegador abierto:

1. **La burbuja se pinta en el `body`, por portal — no basta `position: fixed`.**
   El cajón anima su entrada con `transform`, y un ancestro con `transform` se
   vuelve el bloque contenedor: "fijo respecto a la ventana" pasa a ser "fijo
   respecto al cajón". En el alta de alumno la ayuda de CURP salía desplazada
   **encima de Apellidos**. Verificado antes y después en `localhost:3002`.
2. **Hay un respiro de 120 ms al cerrar por cursor.** Sin él, WCAG 2.2 · 1.4.13
   "señalable" es inalcanzable: al salir del `(?)` la burbuja se desmonta en el
   acto y el `mouseover` que venía en camino aterriza en un nodo ya despegado del
   DOM. El cursor nunca alcanza la burbuja que iba a leer.
3. **El `blur` apaga sólo lo pasajero, nunca lo fijado con clic.** Cerrar todo al
   perder el foco apagaba la burbuja justo cuando se iba a leer o a copiar.

Además: en `InlineCreateRow` la celda pasó de `aria-label` a `<label htmlFor>`
real, y en uniformes los párrafos grises que ya explicaban un campo se
consolidaron en su `(?)` en vez de quedar duplicados.

**Consecuencia en las pruebas:** `getByLabelText(/Nombre/)` ahora encuentra dos
cosas, porque el `(?)` es un control que se llama "Ayuda sobre Nombre". Las 26
consultas laxas piden el campo explícitamente
(`{ selector: "input, select, textarea" }`).

## Encontrado de paso, y NO corregido aquí

Al revisar el alta de alumno en el navegador: **al guardar vacío salen 5 errores
y sólo se ven 4**. El de "Tutor" queda en `y=629` mientras el cuerpo visible del
cajón termina en `y=576`, donde empieza el pie pegajoso. El formulario no se
desplaza (`scrollTop` sigue en 0) y el foco se queda en el botón, así que el
teclado tampoco lleva allí. El director corrige los cuatro que ve, vuelve a
guardar y sigue fallando por una razón invisible.

Es WCAG 2.2 · 2.4.11 y la regla propia del proyecto ("un pie pegajoso nunca debe
tapar el campo enfocado"), pero es un defecto **anterior** a este trabajo y
ajeno a los placeholders. Queda anotado para decidirlo aparte.

## Lo que este trabajo NO hace

- No toca el backend, ni un `.proto`, ni una migración.
- No cambia la validación: lo obligatorio se sigue reclamando al guardar (`useRequired`).
- No reorganiza ninguna forma ni cambia el envase de ningún alta (eso fue el rediseño de
  modales de 2026-09 y ya está decidido aparte).
