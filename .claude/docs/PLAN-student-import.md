# Plan — Migración de alumnos desde el sistema anterior (Configuración → Migrar alumnos)

> v2 (2026-10-05). Este plan ya pasó por /autoplan con tres revisiones: estrategia, ingeniería y diseño. La v1 traía
> una premisa falsa que la v2 corrige: no publicar `StudentEnrolled` **no** evita el cargo de inscripción, porque la
> corrida diaria de finanzas inscribe a toda inscripción activa. Ver "Review record".

## Implementation plan

### Problema

El colegio viene de otro sistema y tiene a sus ~570 alumnos en cinco Excel, uno por nivel
(Maternal 9, Preescolar 73, Primaria 297, Secundaria 161, Bachillerato 30). Hoy cada alumno se
da de alta a mano, y el nivel, el grado y el grupo tienen que existir antes. Migrar así lleva
días y se presta a errores.

Objetivo: en **Configuración** agregar **"Migrar alumnos"**. El director:
1. crea el ciclo si todavía no hay uno;
2. sube un Excel del sistema anterior;
3. ve una vista previa (qué se crea, qué se omite y por qué; todavía no se guarda nada);
4. confirma.

Al confirmar, el sistema crea **lo que falte** del catálogo (nivel → grado → grupo) **sin
duplicar**, inscribe a los alumnos en el ciclo activo **sin cargo de inscripción** y entrega un
reporte por fila.

### Decisiones del dueño (no se reabren)

| # | Decisión |
|---|---|
| O1 | "Matrícula Interna" = **referencia bancaria BBVA**. Campo nuevo `bank_reference`, **único por alumno**. La matrícula del sistema (`YYYY-NNNNN`) no cambia. |
| O2 | Se importan: nombre (nombre / ap. paterno / ap. materno), CURP, nivel/grado/grupo, nacimiento, **teléfonos, correo, domicilio** y **sexo tomado de la CURP**. Se descartan "Programa" y "Dirección 2". |
| O3 | Las filas en **"Baja" no se importan**; se listan en el reporte. |
| O4 | CURP **ya existente** → la fila se omite con aviso y no se toca nada. Subir el mismo archivo otra vez no duplica. |
| O5 | Los migrados entran **como "inscrito" y SIN cargo de inscripción** del ciclo actual. Las colegiaturas mensuales sí se cobran. |
| O6 | Nivel, grado y grupo **se buscan antes de crearse** y se reutilizan; nunca se duplican. |
| O7 | Para más correos o contactos se usan los **contactos** que ya existen (`AddContact`). La importación no crea contactos porque el Excel no trae nombre de tutor. |
| O9 | Si **no hay ciclo activo** (por ejemplo, la primera carga), la pantalla **pide crear el ciclo primero**, en el mismo flujo (paso 0). |
| O8 | *(pendiente en la compuerta)* Cuotas extra del ciclo que vencieron antes de la importación: se cobran desde el mes de importación en adelante, o todas. |

### Formato del archivo (verificado sobre los 5 archivos reales)

Una hoja con la fila 1 de encabezados: Matrícula Interna, Nombre ("APELLIDOS, NOMBRES"), Escolaridad,
[Programa], Grado / Nivel, Grupo, Estado (alumno), Cumpleaños, CURP, Email, Telefono de Casa, Número de
Celular, Dirección, Dirección 2, Estado (domicilio), Ciudad/Municipio, Código Postal.

- Las columnas se reconocen **por encabezado**, no por posición, porque Preescolar no trae "Programa". "Estado" viene dos veces: la primera es el estado del alumno y la segunda, la entidad federativa.
- Las fechas pueden llegar como texto `dd/mm/aaaa` **o** como número de serie de Excel; se aceptan las dos formas.
- Datos sucios que ya vimos: espacios al final, `" 25910"`, `S/R`, `S/R CASA HOGAR`, una CURP vacía, municipio como texto libre, CP vacío y "Preparatoria" donde debería decir "Bachillerato".

### Flujo del usuario (página propia, `Page variant="wide"`, `Stepper` reutilizado de levelDeletion en color forest/lime)

**Entrada.** Un botón fantasma "Migrar alumnos" en el encabezado de Configuración, más una tarjeta de entrada debajo de `CycleBar` (patrón `LevelPeriodsEntry`). En el estado vacío de Estudiantes, un enlace "¿Vienes de otro sistema?". Las tres entradas aparecen solo con el flag encendido y el permiso `students:import`. Hay que agregar el ícono `upload` a `Icon.tsx`.

**Paso 0 — Ciclo (O9), solo si no hay ciclo activo.** Lleva título, el texto "Para migrar alumnos primero crea el ciclo escolar al que van a entrar" y un formulario de 3 campos: nombre (sugerido "2026–2027"), inicio y fin, con fechas sugeridas de ago a jul. Botón: "Crear y activar ciclo". Se reutilizan `CreateCycle` + `ActivateCycle`; no hace falta backend nuevo. Si existe un ciclo pero ninguno está activo, se ofrece "Activar «X»" en lugar de crear otro. Cuando sí hay ciclo activo, el paso no aparece y en la parte de arriba queda visible "Destino: Ciclo 2026–2027".

**Paso 1 — Subir.** Una zona para soltar el archivo, usable con teclado. Botón "Elegir archivo .xlsx". Debajo: "Un archivo por nivel. Empieza con el más chico." Mientras se lee el archivo se muestra "Leyendo «Primaria.xlsx»…" con `aria-live`. Si sueltan varios archivos: "Sube un archivo a la vez", y se toma solo el primero.

**Paso 2 — Vista previa.** Aviso fijo: "Vista previa: todavía no se ha guardado nada."
- **Tarjetas** que cuadran con el total de filas del archivo: Nuevos · Con avisos (es un subconjunto: "280 se importan · 12 con avisos") · Ya estaban · No se importan.
- **Mapeo de nivel.** Cada "Escolaridad" del archivo:
  - Si coincide con un nivel existente (sin importar mayúsculas, acentos ni espacios) → se asigna ese nivel y se puede cambiar.
  - Si **no coincide** → **no hay opción por defecto**. Sale una tarjeta "Elige a dónde van los alumnos de «Preparatoria»" con chips de los niveles existentes y "+ Crear nivel nuevo «Preparatoria»".
  - Si al pulsar Importar falta resolver alguna tarjeta, el foco salta a ella (el botón principal nunca se desactiva).
  - Cada cambio de mapeo vuelve a pedir la vista previa.
- **Avisos por nivel.** "Sin tabulador en el ciclo activo: no se cobrará colegiatura hasta que lo captures", con enlace a Colegiaturas. Lo mismo para periodos de evaluación y asignaturas.
- **Catálogo a crear.** Un árbol plegado ("Se crearán 1 nivel, 6 grados, 12 grupos" ▸ "Primaria › 1° › A, B"). Los sobrecupos se avisan una sola vez por grupo. El **cupo** de cada grupo nuevo se puede editar y por defecto vale `max(alumnos que entran, 30)`.
- **Tabla.**
  - Pestañas: Todos · Nuevos · Con avisos · Ya estaban · No se importan. Por defecto abre la primera que no esté vacía en el orden No se importan → Con avisos → Todos.
  - Búsqueda por nombre o CURP, 50 por página.
  - Columnas: **# fila de Excel**, Alumno, CURP, Destino ("Primaria · 3° · A"), estado (píldora con ícono y texto), Motivo (el primero, más "+2" desplegable).
  - Por debajo de 640 px se muestra como tarjetas.
- **Caja de confirmación** encima del botón: "Se inscribirán en el ciclo 2026–2027 sin cargo de inscripción. Las colegiaturas mensuales sí se les cobrarán como a todos. Una importación no se deshace de golpe: cada alumno se da de baja uno por uno."
- **Botón:** "Importar 297 alumnos a Primaria" (nunca dice solo "Confirmar"). Atrás · Cancelar.

**Paso 3 — Resultado.**
- Mientras importa: "Importando 297 alumnos… no cierres esta ventana", con `beforeunload` activo solo durante la importación. Después: los mismos conteos y "Primaria quedó migrada."
- **Descargar reporte CSV** con **todas** las filas: # fila, resultado, motivo y matrícula asignada. Incluye una columna "posible hermano de" que agrupa por teléfono compartido.
- Lista de verificación "Niveles migrados: Maternal ✓ · Preescolar ✓ · …", leída del servidor.
- Acciones: **Subir otro archivo** (principal) · Ver alumnos importados (Estudiantes filtrado por nivel) · un recordatorio de capturar contactos (O7).
- El foco pasa al `h2` de cada paso cada vez que cambia.

**Estados con su texto.** Archivo que no es .xlsx: "Este archivo no es un Excel (.xlsx). Si viene como .xls, ábrelo en Excel y guárdalo como .xlsx." · Falta columna: "Al archivo le falta la columna «CURP». Usa el reporte tal como lo exporta el sistema anterior." · Hoja vacía · Archivo demasiado grande: "El archivo pasa de 1,000 filas o 2 MB. Sepáralo por nivel." · Todos ya estaban: "Estos 297 alumnos ya estaban registrados (misma CURP). No se duplicó nada." (sin botón Importar) · Corte a medias: "Se importaron 180 de 297 y la conexión se cortó. Sube el mismo archivo otra vez: los que ya entraron aparecerán como «Ya estaba» y sólo se agregará el resto." · La vista previa cambió: "Algo cambió desde la vista previa; revísala de nuevo." · Sin permiso: el apartado no se muestra.

### Reglas de transformación (por fila)

| Campo | Regla |
|---|---|
| Nombre | Algoritmo de separación validado con la CURP (ver abajo). Se recortan espacios; las mayúsculas se dejan como vienen. |
| CURP | Obligatoria (`NOT NULL`). Vacía, inválida o repetida en el archivo → **No se importa** (motivo, con la fila de la primera aparición). Ya existe en la escuela → **Ya estaba**, salvo que su inscripción ya esté en el grupo destino → "Ya importado". |
| Sexo | Posición 11 de la CURP: `H`→`M`, `M`→`F`. |
| Nacimiento | `dd/mm/aaaa` o número de serie → ISO. Si es ilegible o no coincide con la CURP (posiciones 5–10, siglo según la 17) → se usa la de la CURP y se avisa. |
| Ref. BBVA | Se quitan espacios; de 1 a 20 dígitos → `bank_reference`. Cualquier otra cosa (`S/R…`) → NULL con aviso "Sin referencia BBVA (S/R)". Si ya la tiene otro alumno → NULL con aviso; la fila se importa igual. |
| Teléfonos | Se dejan solo los dígitos y se quita el prefijo 52/521/044/045. Con 10 dígitos se guarda: casa=`casa`, celular=`celular`. Si los dos son iguales se guarda uno. Si no son válidos, se descartan con aviso. |
| Correo | `email.Normalize` + `Validate`; si no es válido se descarta con aviso. |
| Domicilio | Dirección → `Street` (≤120). Municipio → `Municipality` (≤120; si es más largo se recorta y se avisa). El estado se compara sin acentos ni mayúsculas y con alias (MICHOACAN DE OCAMPO, CDMX/DISTRITO FEDERAL, MEXICO→Estado de México, VERACRUZ DE IGNACIO…). CP: 5 dígitos. Cada subcampo inválido se descarta solo, con aviso (perfil **tolerante**: un dato malo no tira el domicilio entero). |
| Estado alumno | `Activo` → se importa. `Baja` → No se importa ("Baja en el sistema anterior"). Cualquier otro valor → No se importa. |
| Destino | Nivel según el mapeo. Grado: ordinal = "Grado / Nivel", nombre `"<n>°"`. Grupo: la letra recortada y en mayúscula, dentro del ciclo activo. |

**Separación de apellidos** (las letras 1–4 de la CURP siempre se validan):
```
fold(s): mayúsculas, sin acentos, Ñ→X, quita . ' -.   Partículas P={DA DAS DE DEL DER DI DIE DD EL LA LAS LE LES LOS MAC MC VAN VON Y}
sig(ws) = primera palabra ∉ P (si no, la última). L1=sig(pat)[0]; L2=1ª vocal de sig(pat)[1:] o 'X'; L3=sig(mat)[0] o 'X' si no hay materno
apellidos,nombres = corte en la PRIMERA coma (sin coma → probar ambos órdenes contra la CURP; si ninguno cuadra → No se importa)
para k en 1..n: pat=T[:k], mat=T[k:]; ilegal si T[k-1]∈P (la partícula se une a la siguiente palabra)
  cuadra si c[0]==L1 && (c[1]==L2 || c[1]=='X') && c[2]==L3
1 cuadra → se usa; >1 → la k más chica + aviso; 0 → la k legal más chica + aviso "apellidos no coinciden con la CURP"
nombres: si hay ≥2 y el primero ∈ {MARIA, MA, M, JOSE, J} → c[3] se valida contra el 2º nombre; si no cuadra → aviso
```
Ejemplo: DE LA CRUZ HERNANDEZ con CURP CUH… → solo cuadra k=3.

### Arquitectura

```
web /config (Migrar alumnos) ── JSON {fileBase64, mapping, capacities, planHash?} ──▶ gateway
   [flags module_students AND module_config AND module_student_import; permiso owner-only students:import; requireAuthChanged]
   legacyimport (puro): excelize (UnzipSizeLimit 20MB, UnzipXMLSizeLimit 10MB, Rows() en streaming, ≤1,000 filas, ≤2 MB)
        → columnas por encabezado → separación de nombre / chequeos de CURP → RawRow (sin reglas de dominio)
   preview: config.EnsurePlacements(dry_run) + students.ImportStudents(validate_only) → plan + planHash
   commit : re-parsea → ¿hash == planHash? si no, 409 → config.EnsurePlacements (tx)
            → students.ImportStudents(filas + group_id)   [ctx desligado con context.WithoutCancel + timeout propio]
config: EnsurePlacements: un RPC, una tx, índice único sobre el nombre plegado, INSERT … ON CONFLICT, excluye
        niveles 'deleting' (55000 → error por destino). Sirve para preview y commit.
students: por grupo distinto (~40) valida por RPC y DERIVA ciclo y nivel (se mantiene requireSameLevel);
        por fila, 1 tx: matrícula + student (ENROLLED, level_id/level_name, perfil tolerante, bank_ref pre-chequeada,
        import_batch_id) + enrollment (origin='import', enrolled_at=now()) + PlaceIn. NO publica eventos.
finance: RunBilling / EnsureEnrollmentCharge / inscriptionMoves: origin='import' ⇒ sin inscripción;
        colegiatura desde el mes de importación (ya pasa con InstallmentsFrom(StartsOn, EnrolledAt));
        cuotas extra según O8; sin recargos el día 1.
```

¿Por qué RPCs nuevos y no reutilizar CreateLevel/CreateGroup/Enroll? (a) El nombre de nivel es único con distinción de mayúsculas, así que list-then-create duplica "PREPARATORIA"/"Preparatoria" y tiene carrera. (b) `Enroll` publica `StudentEnrolled`. (c) No habría atomicidad por fila ni modo de validación para la vista previa.

### Cambios por capa (orden del flujo de módulo; TDD en cada paso: test rojo → verde → refactor)

1. **ADR 0029**: importación de alumnos y `enrollments.origin`. Es una frontera entre servicios (students ↔ finance).
2. **Contratos**:
   - `students.v1`: `Student.bank_reference = 17`, `CreateApplicantRequest.bank_reference = 10`, `UpdateStudentRequest` con `optional string bank_reference = 7` (si no viene, se conserva); `Enrollment.origin = 10`; RPC `ImportStudents{group rows, validate_only, batch_id}` con resultado por fila.
   - `config.v1`: `EnsurePlacements{cycle_id, placements[{level_id|new_level_name, ordinal, group_name, capacity}], dry_run}`.
   - `docs/api/openapi.yaml`: `POST /api/v1/imports/students/preview` y `/commit`, con el sobre de error único.
   - `buf lint` + `buf generate`.
3. **Migraciones**:
   - `students`: `bank_reference text NULL` + `uq_students_bank_reference` parcial `(tenant_id, bank_reference) WHERE NOT NULL`; `import_batch_id uuid NULL`; `enrollments.origin text NOT NULL DEFAULT 'manual' CHECK (origin IN ('manual','import'))`. Tests de aislamiento RLS A≠B.
   - `config`: índice único sobre el nombre de nivel plegado (`lower` + `translate` de acentos + espacios colapsados). Antes de crearlo, la migración revisa choques ya existentes, como hace la 0006.
   - `iam`: `students:import`, otorgado al owner y agregado a `IsOwnerOnly`.
   - Flag `module_student_import` (default OFF): FLAGS.md, `packages/ui/src/flags/types.ts`, `ENABLED_MODULES` de docker-compose y terraform. Tiene una tarea de retiro en FLAGS.md.
4. **finance (primero, con test rojo)**: `ports.EnrollmentRef.Origin`. `wantedInscriptions`, `EnsureEnrollmentCharge` e `inscriptionMoves` saltan `import`. Las cuotas extra se filtran por fecha de ingreso (según O8). Regresión: una corrida sobre un alumno importado da 0 inscripciones, colegiatura desde el mes de importación y 0 recargos.
5. **students**:
   - VO `BankReference`; `domain.NewProfileLenient` (descarta subcampos inválidos y devuelve avisos); alias de estados.
   - `ImportStudents` y la consulta de CURP para `validate_only`.
   - El repo distingue la violación de `uq_students_bank_reference` de la de la CURP (`violates(err, name)`). Hoy toda violación única se reporta como "CURP ya existe".
6. **config**: caso de uso `EnsurePlacements`. Mapea el error 55000 (nivel en borrado) a un error por destino.
7. **gateway**: paquete `internal/legacyimport` (parser y separación de nombres, puros), handlers preview/commit con `planHash`, rutas protegidas por los 3 flags y el permiso.
8. **web**:
   - `bank_reference` visible y editable en la ficha y buscable en la lista y en el superbuscador.
   - Página Migrar alumnos con los pasos 0–3, con todos los estados y textos descritos arriba.
   - Ícono `upload`, `Stepper` generalizado, entradas desde Configuración y Estudiantes.
9. **QA** (qa-browser.md): base LOCAL vacía y navegador visible. Crear el ciclo desde el paso 0, importar los 5 archivos reales empezando por Maternal, mapear Preparatoria→Bachillerato. Volver a subir Primaria → 0 nuevos. Correr la cobranza y verificar 0 inscripciones y colegiatura solo desde el mes actual. Usar `E2E_SKIP_RESET=1`.

### Runbook del día de migración (va en `.claude/docs/` con el módulo)

1. Ensayar en local o en una copia. **Nunca** en dev con `demo:verify` o Playwright activos, porque vacían la base. Decidir con el dueño en qué entorno viven los datos reales (CURPs y domicilios de menores).
2. Pausar la corrida automática de cobranza.
3. Capturar los tabuladores de cada nivel en el ciclo activo.
4. Importar, empezando por el nivel más chico.
5. Capturar becas y la lista de alumnos prepagados o con saldo a favor que entregue el colegio. El sistema anterior no se migra en dinero.
6. Reanudar la cobranza y revisar los estados de cuenta de una muestra.

El gateway nunca registra filas, CURPs ni domicilios en los logs; solo conteos.

### Fuera de alcance (con motivo)

- Crear contactos o tutores e invitar familias desde el Excel (O7). → TODO.
- Conciliación de pagos BBVA con la referencia; el campo queda listo. → TODO.
- Saldos, adeudos, pagos y calificaciones del sistema anterior. Los periodos anteriores a la importación quedan sin calificaciones; se avisa en la vista previa y en QA.
- Deshacer una importación por lote. Queda la base para hacerlo: `import_batch_id` (auditoría). → TODO.
- CSV, `.xls` viejo y plantillas de otros sistemas.
- Varios archivos en una sola carga: se sube uno a la vez, por diseño.

## Review record

### Decision Audit Trail
<!-- AUTONOMOUS DECISION LOG -->
| # | Fase | Decisión | Clasificación | Principio | Motivo | Rechazado |
|---|---|---|---|---|---|---|
| 1 | 0 | Saltar /office-hours | Mecánica | P6 | El dueño ya respondió 6 preguntas de alcance | Correr office-hours |
| 2 | 0 | Hook de autoplan falló (journal sin evidencia, 6 reintentos) → revisiones por agentes independientes | Mecánica | P6 | El propio hook sugiere el fallback manual | Abortar |
| 3 | CEO+Eng | `enrollments.origin='import'` y finance lo respeta | Mecánica (P0) | P1 | `billing.go:244-260,353` inscribe toda inscripción activa | "No publicar el evento basta" |
| 4 | Eng | `enrolled_at = now()` explícito | Mecánica | P1 | `InstallmentsFrom` cobra desde el ingreso | Fecharlo al inicio del ciclo |
| 5 | CEO+Eng | Cupo del grupo nuevo = max(conteo, 30), editable | Mecánica | P1 | `enrollment_repository.go:119,272` rechaza al llenarse | Cupo = conteo |
| 6 | Eng | Un solo `EnsurePlacements(dry_run)` en lugar de Resolve+Ensure | Mecánica | P4 | Un solo camino de código para preview y commit | 2 RPCs |
| 7 | Eng | `ImportStudents(validate_only)` para "Ya estaba" en la vista previa | Mecánica | P1 | students no tiene consulta por CURP | Validar en el gateway |
| 8 | Eng | Ciclo y nivel derivados del grupo dentro de students | Mecánica | P5 | Mantiene `requireSameLevel` | ciclo enviado por quien llama |
| 9 | Eng | Perfil tolerante en el dominio de students; el gateway solo parsea | Mecánica | P4 | `NewProfile` es todo-o-nada | Duplicar validación |
| 10 | Eng | Índice único sobre el nombre de nivel plegado | Mecánica | P1 | UNIQUE con distinción de mayúsculas en config/0001:32 | Comparar solo en memoria |
| 11 | Eng | `optional` en UpdateStudent.bank_reference | Mecánica | P1 | proto3 borraría la referencia | campo plano |
| 12 | Eng | Commit re-envía archivo + planHash (409 si cambió) | Mecánica | P5 | Cloud Run con varias instancias: sin token en memoria | Token en memoria |
| 13 | Eng | excelize con límites y ≤1,000 filas | Mecánica | P1 | UnzipSizeLimit por defecto 16 GB; WriteTimeout 30 s | 2,000 filas |
| 14 | Eng | Separación de apellidos validada con la CURP (algoritmo arriba) | Mecánica | P1 | Apellidos compuestos | Primer token = paterno |
| 15 | Diseño | Nivel sin coincidencia SIN destino por defecto | Mecánica | P1 | Crear "Preparatoria" en silencio duplica Bachillerato | Crear por defecto |
| 16 | Diseño | 4 estados (Nuevo/Con avisos/Ya estaba/No se importa) | Mecánica | P5 | Volver a subir no debe parecer error | ✓/⚠/✗ |
| 17 | Diseño | Un archivo a la vez | Gusto | P5 | Lotes chicos y mapeo por archivo | Los 5 juntos |
| 18 | CEO+Diseño | Aviso por nivel sin tabulador/periodos | Mecánica | P1 | `billing.go:378-383` lo salta sin avisar | — |
| 19 | CEO | CSV con "posible hermano de" (teléfono compartido) | Gusto (S) | P2 | 196 teléfonos compartidos | — |
| 20 | CEO+Eng | `import_batch_id` (auditoría); deshacer por lote → TODO | Gusto | P3 | S ahora, M después | Deshacer ya |
| 21 | CEO | Runbook del día de migración | Mecánica | P1 | Becas, prepagados, cobranza automática, base de demo | — |
| 22 | Usuario | Paso 0: crear/activar ciclo si no hay activo (O9) | Usuario | — | Pedido explícito del dueño | Bloquear sin ciclo |

### Compuerta final (2026-10-06)
- O8 → desde el mes de importación. O10 → ensayo local, entorno real se decide después.
- **Plan v2 APROBADO** por el dueño.

### Cobertura de revisión
| Fase | Voz nativa | Voz externa (Codex) | Hallazgos |
|---|---|---|---|
| CEO | agente independiente, completo | no instalado (N/A) | 1 P0, 6 P1, 4 P2 |
| Diseño | agente independiente, completo | no instalado (N/A) | 6 P1, 5 P2; puntajes 3–6/10 → objetivo 9 |
| DX | omitida: falso positivo de términos ("API", "flag", "import"); no es producto para desarrolladores | — | — |
| Eng | agente independiente, completo | no instalado (N/A) | 2 P0, 12 P1, 3 P2 |
| Temas cruzados | inscripción cobrada por la corrida diaria (CEO+Eng), grupo lleno por cupo (CEO+Eng), nivel sin tabulador (CEO+Eng+Diseño) | | |
