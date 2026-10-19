# QA-PROCESO — Guion de QA por entrega (García Lorca)

> **Para qué sirve.** Es el guion que sigue un QA (persona o agente) **antes de cada
> entrega** y para **levantar un ciclo escolar completo desde una base vacía**, con el
> sistema en modo producción, por rol: Director (owner, web), Cobranza (caja, web),
> Docente (web) y Familia (app móvil).
>
> **De dónde sale.** Del código en `dev` al 17-oct-2026 (`services/gateway/cmd/main.go`,
> `proto/`, `migrations/iam`, `apps/web/app`, `apps/mobile`, `deploy/terraform`), no de la
> memoria. El inventario completo está en el **Apéndice A**. Cuando el código cambie,
> el apéndice se regenera con los comandos del **Apéndice B**.
>
> **Antecedente.** El método sale de las corridas T14 (`.gstack/qa-reports/qa-report-t14-*.md`,
> 17-oct-2026): fase A director, fase B docente + familia + cruces, 4 verificaciones.
> Cada caso de borde marcado **[T14-nnn]** reproduce un defecto real que ya se corrigió:
> es una prueba de regresión, no una idea.

Reglas que este guion no repite y que aplican siempre: `~/.claude/rules/qa-browser.md`
(navegador visible, base local vaciada, datos creados por la UI), `tdd.md` (todo arreglo
lleva su test de regresión primero), `git-workflow.md` (nunca en `dev`/`master`).

---

## Índice

0. [Preparación](#0-preparación)
1. [Ciclo completo desde cero](#1-ciclo-completo-desde-cero) — 40 pasos
2. [Matriz de permisos](#2-matriz-de-permisos)
3. [Pruebas de dinero (conciliación SQL)](#3-pruebas-de-dinero)
4. [Pruebas no funcionales mínimas](#4-pruebas-no-funcionales-mínimas-por-entrega)
5. [Plantilla de reporte](#5-plantilla-de-reporte)
- [Apéndice A — Inventario](#apéndice-a--inventario-del-sistema)
- [Apéndice B — Cómo regenerar el inventario](#apéndice-b--cómo-regenerar-el-inventario)

---

## 0. Preparación

### 0.1 Levantar el stack en modo producción

La guía completa está en [`deploy/PROD-LOCAL.md`](../../deploy/PROD-LOCAL.md). En corto:

| Paso | Comando | Notas |
|---|---|---|
| Construir imágenes | `make prod-build` | las 10 imágenes de producción **una por una** (en paralelo se cae el daemon de Docker) |
| Arrancar | `make prod-up` | detiene los contenedores de la app de desarrollo (Postgres sigue) y levanta el proyecto `prod-local` |
| Salud | `make prod-status` | sondea el **puerto** de cada servicio, no `docker ps` |
| Bitácoras | `make prod-logs` | |
| Apagar | `make prod-down` | vuelve a arrancar el stack de desarrollo |
| APK de release | `scripts/mobile-release-apk.sh` | JDK 17 (`brew install openjdk@17`); app id local de marcador |

- Puertos: web **:13000**, gateway **:18080** (el emulador usa `10.0.2.2:18080`), servicios internos 18091–18101 sólo en 127.0.0.1.
- Firma de sesiones fija (`.env.prod-local`, ignorado por git): reiniciar iam ya no cierra las sesiones.
- `ENABLED_MODULES` = lista de `deploy/terraform/services.tf` + `ui_refresh,announcements_view` (encendidos en dev a mano; ver §0.2).
- Diferencias con producción real (sin escala a cero, login por contraseña en vez de Identity Platform, Pub/Sub emulado, push sólo en bitácora, sin SMTP, archivos subidos en memoria, Postgres sin TLS, sin Cloud Scheduler, APK con llave de depuración): tabla en `deploy/PROD-LOCAL.md`.

El stack de desarrollo sigue siendo `docker compose up -d` (web :3012 según `.env`,
gateway :8080, Postgres :5433 en el host, servicios internos :8091–:8101). Antes de
empezar, anotar en el reporte qué stack se usó.

### 0.2 Flags de la corrida

| Fuente | Lista |
|---|---|
| Local (`docker-compose.yml`, por omisión) | `iam,gateway,students,config,teachers,grades,finance,attendance,comms,uniforms,expenses,schedule,activities,scholarships,student_import` |
| Producción/dev (`deploy/terraform/services.tf`) | la anterior **+ `config_setup`** |
| Vistas nuevas (sólo presentación) | `ui_refresh`, `announcements_view` — encendidas a mano en dev, **no** están en terraform |

- La corrida de entrega usa **la lista de terraform** + las vistas que estén encendidas en dev.
  Arrancar el gateway con `ENABLED_MODULES=<lista> docker compose up -d gateway` y
  confirmar con `GET /api/v1/features`.
- Pasada **flag OFF** (obligatoria, ver 2.4): por cada módulo, apagarlo y comprobar 404
  en sus rutas y que la web oculta su tarjeta.
- La app móvil **no lee `/features`**: sus flags están fijos en
  `apps/mobile/src/flags/bootstrap.ts` (credencial, uniformes, becas y documentos en `false`).

### 0.3 Vaciar la base LOCAL

```sh
# Sólo contra el contenedor de compose. Sin PGHOST.
pg_dump ... > "$SCRATCH/antes-de-vaciar.sql"     # respaldo, fuera del repo
sh scripts/reset-demo-db.sh
```

- El guard del script **se niega** si `PGHOST` está definido (el proxy de Cloud SQL hace
  que una base remota parezca `127.0.0.1`). Nunca pasar `--i-know-this-wipes-remote` en QA.
- Conserva sólo `users roles user_roles user_invitations schema_migrations`. Verificar:
  `SELECT count(*) FROM students;` → 0, `SELECT count(*) FROM cycles;` → 0.
- **No** correr Playwright sin `E2E_SKIP_RESET=1` después de sembrar: su `globalSetup` vacía la base.
- Alternativa sin Excel reales: `pnpm demo:seed` (400 alumnos sintéticos). Sirve para volumen,
  **no** sustituye la creación por la UI del paso 1.

### 0.4 Navegador visible

- `$B connect` (Chromium de gstack, `Mode: headed`). Si falla: matar el daemon y el Chromium
  huérfano, borrar `~/.gstack/chromium-profile/SingletonLock`, reintentar. Sólo entonces
  headless, y decirlo en el reporte.
- Un perfil por rol: abrir una pestaña nueva y cerrar sesión entre roles, o usar `curl`
  con un archivo de cookies por rol para las pruebas de API.
- "Imprimir ticket" abre el diálogo de impresión y **congela** el daemon hasta `Esc` [T14 E2]:
  probar el ticket con su JSON/PDF y una sola vez a mano.
- `$B disconnect` al terminar.

### 0.5 Emulador Android + APK de release

- AVD de referencia: `Medium_Phone_API_36.1` (`emulator-5554`). El host se ve como `10.0.2.2`.
- **Expo Go no vale como evidencia de entrega**: congela el emulador con el bundle de
  desarrollo [T14 E3] y esconde lo que Metro sí compila. Construir release:
  ```sh
  cd apps/mobile
  EXPO_PUBLIC_API_URL=http://10.0.2.2:8080 pnpm exec expo prebuild --platform android --clean
  (cd android && ./gradlew assembleRelease)
  adb install -r android/app/build/outputs/apk/release/app-release.apk
  ```
  (Alternativa: `eas build --profile preview --platform android --local`.) El directorio
  `android/` generado **no** se commitea.
- Push: sin `EAS_PROJECT_ID` no hay token de Expo → la app sólo registra en desarrollo y
  comms no puede entregar. Registrar como **ENTORNO** mientras no exista el proyecto EAS.
- Para tocar con `adb`: volver a leer el `uiautomator dump` después de cada acción (las
  referencias cambian); no mandar `BACK` justo después de un envío que navega solo.

### 0.6 Credenciales

- Director: el owner sembrado local (`director@garcialorca.edu.mx`) con la contraseña de
  `OWNER_TEMP_PASSWORD` del `.env` local. La primera entrada obliga a cambiarla.
- Todos los demás usuarios se **crean en la corrida, por la UI**, con dominio
  `@garcialorca.test` y contraseñas de QA (`<Rol>QA<año>.`). Se listan en el §8 del reporte
  porque son sintéticas y sólo locales.
- **Nunca** contraseñas reales, nunca la cuenta Google/Firebase del dueño. Si el `.env`
  manda el login a Firebase (el director sembrado no existe ahí y sale "contraseña
  incorrecta"), apagar Firebase en el `.env` local antes de empezar.
- Un muro que el agente no puede cruzar (2FA, proveedor real): `$B handoff`, el dueño entra, `$B resume`.

### 0.7 Privacidad (datos reales de menores)

- Los archivos de `importaciones/*.xlsx` (5, uno por nivel) tienen **datos reales** de
  niñas y niños. Sólo entran a la base **LOCAL**. Nunca a dev/GCP, nunca a fixtures.
- En reportes, issues, commits y capturas: **sólo conteos y sumas agregadas**. Nada de
  nombres, CURP, matrícula, teléfono, domicilio ni montos que vengan del Excel.
- Toda operación de detalle (cobro, baja, calificación, aviso, acceso de familia) se hace
  sobre alumnos **sintéticos** creados en la corrida: "PRUEBA CALIDAD ALUMNO UNO",
  "… ALUMNA DOS", "… ALUMNO TRES".
- Capturas: recortar al resumen (`$B screenshot --clip`) y **abrir cada captura** antes de
  guardarla. Una lista de grupo, una vista previa de importación o un reporte de asistencia
  muestran alumnos reales: no se capturan. Si se tomó una por error, se borra en el acto y
  se dice en el reporte.
- `.gstack/` y `importaciones/` están en `.gitignore`; confirmar con `git status` antes de
  cualquier commit.
- El aviso "Ambiente de demostración · datos ficticios" queda en pantalla aunque haya datos
  reales en local: es configuración, no defecto; mencionarlo en ENTORNO.

### 0.8 Herramientas de verificación independiente

```sh
# SQL como superusuario (ignora RLS: es la vista "desde fuera" para cuadrar cifras).
PSQL='docker exec -i garcia-lorca-postgres-1 psql -U postgres -d garcia_lorca -At'
# API con cookie por rol (web).
curl -s -c dir.jar -H 'Content-Type: application/json' \
  -d '{"email":"director@garcialorca.edu.mx","password":"…"}' localhost:8080/api/v1/auth/login
curl -s -b dir.jar localhost:8080/api/v1/me | jq .permissions
# API con Bearer (familia).
TOKEN=$(curl -s -d '{"email":"familia.qa@garcialorca.test","password":"…"}' \
  -H 'Content-Type: application/json' localhost:8080/api/v1/auth/mobile/login | jq -r .accessToken)
```

- Dinero en la base: **centavos enteros** (`*_cents`). Pantalla ÷ 100.
- "Hoy" del colegio = fecha de **America/Mexico_City** (ADR 0035). En SQL:
  `(now() AT TIME ZONE 'America/Mexico_City')::date`, nunca `CURRENT_DATE`.
- Consola y red en cada pantalla: `$B console --errors` y `$B network` (filtrar 4xx/5xx).
  Un 4xx **provocado** por un caso de borde se anota como esperado.

---

## 1. Ciclo completo desde cero

Formato de cada paso:

- **Rol · Ruta** — quién y dónde.
- **Datos** — sintéticos sugeridos.
- **Pasos** — exactos.
- **Esperado** — lo que debe verse.
- **Verificación** — SQL o API independiente (no la misma pantalla).
- **Red/consola** — llamadas que deben aparecer y su código.
- **Bordes** — casos que hay que intentar. Cada uno lleva su resultado esperado.
- **Evidencia** — qué se guarda.

Un paso no se da por bueno sin la **verificación**. El orden es el de un colegio real; los
pasos dependen de los anteriores.

### Fase I — Configuración del colegio (Director)

#### 1.1 Datos del colegio
- **Rol · Ruta**: Director · `/configuracion` → Datos del colegio.
- **Datos**: nombre "Colegio QA", razón social, RFC y CCT ficticios, dirección, teléfono, leyenda de boleta.
- **Pasos**: llenar y guardar; recargar.
- **Esperado**: persiste; el encabezado de comprobantes y boletas lo usa después (pasos 1.24 y 1.32).
- **Verificación**: `GET /api/v1/config/school-profile` · `SELECT * FROM school_profile;`
- **Red/consola**: `PUT /config/school-profile` 200.
- **Bordes**: RFC con formato inválido → mensaje al salir del campo, no al teclear · sin `config:write` (Cobranza) → la ruta da 403.
- **Evidencia**: captura del formulario guardado.

#### 1.2 Ciclo escolar
- **Rol · Ruta**: Director · `/configuracion` (barra de ciclo).
- **Datos**: 2026-2027, 24-ago-2026 → 09-jul-2027.
- **Pasos**: crear y **activar**.
- **Esperado**: un solo ciclo activo. Lo eligen por omisión las demás pantallas.
- **Verificación**: `SELECT name, starts_on, ends_on, status FROM cycles;`
- **Red/consola**: `POST /config/cycles` 201 · `POST /config/cycles/{id}/activate` 200.
- **Bordes**: fin antes que inicio → error · segundo ciclo traslapado → rechazo claro · activar otro ciclo deja el primero inactivo (no dos activos).
- **Evidencia**: SQL del ciclo.

#### 1.3 Niveles
- **Rol · Ruta**: Director · `/configuracion` → Niveles (vista "Ruta de arranque" si `config_setup` está ON).
- **Datos**: PRIMARIA (clave PRI). Más adelante la importación crea Maternal, Preescolar, Secundaria y Preparatoria.
- **Pasos**: crear; elegir modo de asistencia (diaria o por clase), escala de calificación (enteros 5–10) y orden.
- **Esperado**: se guarda en MAYÚSCULAS. Con un solo nivel queda preseleccionado en Materias y Grupos [T14-023].
- **Verificación**: `SELECT name, code, position, attendance_mode FROM levels ORDER BY position;`
- **Red/consola**: `POST /config/levels`, `PATCH /config/levels/{id}`, `PUT /config/levels/order`.
- **Bordes**: clave duplicada → 409 con mensaje · no ofrece decimales en la calificación [T14-005] · cambiar la escala con calificaciones ya capturadas → rechazado (scale guard).
- **Evidencia**: captura de la lista de niveles.

#### 1.4 Grados
- **Rol · Ruta**: Director · `/configuracion` → Grados (fila en línea, Enter guarda).
- **Datos**: 1º a 6º de PRIMARIA.
- **Pasos**: capturar en línea con Enter; renombrar uno; borrar uno vacío y volver a crearlo.
- **Esperado**: ortografía "1º" igual que la importación (pendiente D-B: "1º" vs "1°").
- **Verificación**: `SELECT name FROM grade_levels WHERE level_id=… ORDER BY position;`
- **Bordes**: borrar un grado con grupos o alumnos → bloqueado con motivo · nombre vacío → error al guardar, no al teclear.
- **Evidencia**: SQL.

#### 1.5 Grupos
- **Rol · Ruta**: Director · `/configuracion` → Grupos.
- **Datos**: A y B con cupo 30 en 1º–3º.
- **Pasos**: crear (al guardar propone el siguiente), renombrar, borrar uno vacío.
- **Esperado**: el nombre que se ve incluye grado y nivel en todo el sistema ("3º PRIMARIA · A").
- **Verificación**: `SELECT g.name, g.capacity, gl.name FROM groups g JOIN grade_levels gl ON gl.id=g.grade_level_id;`
- **Bordes**: cupo 0 o negativo → error · borrar un grupo con alumnos → bloqueado (ADR 0036) · grupo duplicado → 409.
- **Evidencia**: SQL.

#### 1.6 Materias
- **Rol · Ruta**: Director · `/configuracion` → Materias.
- **Datos**: ESPAÑOL, MATEMÁTICAS y CIENCIAS en los 6 grados.
- **Pasos**: crear en un grado y aplicar a todo el nivel; renombrar; borrar una sin calificaciones.
- **Verificación**: `SELECT count(*) FROM subjects WHERE grade_level_id IN (…);` → 18.
- **Bordes**: borrar una materia con calificaciones o asignaciones → bloqueado con motivo.

#### 1.7 Trimestres y periodos de evaluación
- **Rol · Ruta**: Director · `/configuracion` → Trimestres · `/configuracion/niveles/{id}/periodos`.
- **Datos**: 3 trimestres con meses (ago–nov, dic–mar, abr–jul); periodos por grado.
- **Pasos**: crear trimestres, asignar meses, copiar meses a otro nivel; definir periodos por grado y copiarlos a todo el nivel.
- **Esperado**: los periodos tienen fechas de captura; el 2.º y 3.º salen "Aún no empieza" para el docente.
- **Verificación**: `SELECT * FROM terms; SELECT * FROM term_months; SELECT grade_level_id, starts_on, ends_on, term_id FROM evaluation_periods;`
- **Bordes**: periodos que se traslapan → rechazo · cambiar meses de un trimestre cerrado → bloqueado (term lock) · **conocido [T14-006]**: los periodos por grado proponen tercios del ciclo y quedan con `term_id` nulo; anotarlo si sigue igual.

#### 1.8 Calendario: días no laborables y eventos
- **Rol · Ruta**: Director · `/configuracion` → Calendario.
- **Datos**: feriado 16-nov-2026; vacaciones 21-dic → 06-ene; evento escolar "Junta QA".
- **Pasos**: crear, listar, borrar uno y volver a crearlo.
- **Esperado**: Horarios marca el día como no laborable; Asistencia **no** deja tomar lista ese día [T14-101]; las fechas se ven con el mes en letra [T14-012].
- **Verificación**: `SELECT * FROM non_working_periods; SELECT * FROM school_events;` · `GET /api/v1/teaching/schedule/day?date=2026-11-16` → `nonWorking`.
- **Bordes**: rango invertido → error · día ya ocupado → rechazo.

#### 1.9 Tabuladores (inscripción, colegiatura, recargos, pronto pago)
- **Rol · Ruta**: Director · `/configuracion` → Colegiaturas (página del tabulador).
- **Datos**: Primaria: inscripción $4,000, colegiatura $3,500 × 10 desde septiembre, límite día 10, recargo diario $10, pronto pago $200 antes del día 5. Un tramo de inscripción con precio que sube por fecha (ADR 0011). Otro nivel con otro recargo diario (el recargo mensual fijo ya no existe desde M16). Uno usando "copiar del ciclo anterior" si existe.
- **Pasos**: definir, guardar, reabrir.
- **Esperado**: el resumen "Año completo" suma la inscripción mientras se captura [T14-007]; "Vigente desde" viene prellenado.
- **Verificación**: `GET /api/v1/finance/tuition-plans?cycleId=…` · `SELECT * FROM tuition_plans;` (montos en centavos).
- **Bordes**: monto 0 o negativo → error · monto con o sin separadores ("3,500", "3500.00") → mismo valor · sin "Vigente desde" → un solo mensaje en el campo que falta y el foco ahí · Cobranza → 403.
- **Evidencia**: captura del resumen (sólo montos de QA).

#### 1.10 Conceptos de catálogo
- **Rol · Ruta**: Director · `/finanzas` → Conceptos.
- **Datos**: "CREDENCIAL QA" $80, "EXAMEN EXTRAORDINARIO QA" $350.
- **Pasos**: crear, editar el precio, desactivar uno.
- **Verificación**: `SELECT name, kind, amount_cents, active FROM charge_concepts;`
- **Bordes**: nombre duplicado → 409 · editar un concepto ya cargado no cambia los cargos emitidos.

#### 1.11 Becas
- **Rol · Ruta**: Director · `/becas` (flag `module_scholarships`).
- **Datos**: "BECA HERMANOS QA" 20 % sobre colegiatura; "APOYO FIJO QA" −$500 sobre inscripción; "PRECIO FIJO QA" pagar $2,000 por colegiatura.
- **Pasos**: crear las 3; asignar después de dar de alta a los sintéticos (paso 1.18) con el selector de dos paneles; editar una; quitar una.
- **Esperado**: la beca se calcula **al leer y al cobrar** (ADR 0022): base → descuento del nivel → beca (la más favorable, no se suman) → pronto pago. Se respeta si se paga a tiempo; pagada tarde, el cargo se cobra completo con recargo.
- **Verificación**: `SELECT * FROM scholarship_students;` · estado de cuenta `GET /finance/accounts/{id}/statement` con líneas `scholarship` · ver §3.4.
- **Bordes**: dos becas al mismo alumno → aplica sólo la más favorable · quitar la beca devuelve el precio a los cargos pendientes y **no toca** los ya pagados · 0 % o más de 100 % → error · apagar el flag oculta la pantalla pero **las becas siguen aplicando**.

#### 1.12 Actividades
- **Rol · Ruta**: Director · `/actividades` (flag `module_activities`).
- **Datos**: "AJEDREZ QA" martes 14–15 h, $600 al mes, recargo fijo $50; "NATACIÓN QA" con 2 opciones (2 o 3 días por semana) y periodicidad semanal.
- **Pasos**: crear; inscribir a un sintético (después de 1.18); cambiar su opción; darlo de baja de la actividad; desactivar y reactivar la actividad.
- **Esperado**: inscribir emite al instante lo ya facturable, con ref `activity:<act>:<alumno>:<periodo>`; entrar después de la fecha límite empieza en el siguiente periodo; dar de baja no toca cargos emitidos; reactivar **no** cobra los periodos apagados (`resumed_on`).
- **Verificación**: `SELECT external_ref, amount_cents, due_date, status FROM charges WHERE kind='activity';`
- **Bordes**: inscripción con fecha pasada; reactivar después del vencimiento (pendientes T6: anotar si cambian) · doble clic en "Inscribir" → un solo cargo.

#### 1.13 Inventario (uniformes): catálogo y existencias
- **Rol · Ruta**: Director · `/uniformes` (flag `module_uniforms`).
- **Datos**: "PLAYERA DEPORTIVA QA" $250, tallas 6/8/10/12, mínimo 3; "LIBRO QA" sin tallas, sólo PRIMARIA.
- **Pasos**: crear artículo; tallas por rango; entrada de 40 piezas; ajuste −2 con motivo; ajustes de la tienda (settings).
- **Esperado**: existencias por talla; alerta bajo el mínimo.
- **Verificación**: `SELECT i.name, s.size, s.on_hand FROM uniform_stock s JOIN uniform_items i ON i.id=s.item_id;` · `SELECT * FROM uniform_stock_moves ORDER BY created_at;` (cada movimiento con su motivo).
- **Bordes**: ajuste que deja existencia negativa → rechazo · entrada con cantidad 0 → error · rol sólo `uniforms:stock` ve inventario pero **no** pedidos con nombres (`uniforms:orders` es del owner).

### Fase II — Personas y accesos

#### 1.14 Roles y usuarios
- **Rol · Ruta**: Director · `/administracion` (flag `module_iam`).
- **Datos**: usuario "CAJA QA" con el rol sembrado `cobranza`; rol nuevo "SECRETARÍA QA" con `students:read`, `expenses:read`, `schedule:read`.
- **Pasos**: invitar al usuario (la liga se copia: no hay correo local [T14 E4]); abrir la liga en `/activar` en otra pestaña y fijar contraseña; crear el rol y darle permisos; suspender y reactivar un usuario; eliminar uno de prueba; reenviar invitación.
- **Esperado**: la cuenta suspendida no entra (401) y su sesión abierta muere; la eliminada desaparece de la lista (soft delete).
- **Verificación**: `SELECT email, status FROM users;` · `SELECT name, permissions FROM roles;` · `GET /api/v1/me` con la cookie del nuevo usuario.
- **Bordes (seguridad)**: dar a un rol no-owner un permiso owner-only (`finance:read`, `attendance:read`, `comms:read`, `guardians:*`, `uniforms:orders`, `medical:*`, `students:notes_void`, `grades:close`, `levels:delete`, `users:delete`, `students:import`) → **rechazo** · asignar el rol `guardian` a mano → rechazo · un usuario con `users:write` no puede dar permisos que él no tiene (caller-subset) · eliminar o suspender al último owner → rechazo (verificar el mensaje) · invitación usada dos veces → error · `/activar` con token alterado → error sin filtrar si el correo existe.
- **Evidencia**: captura de la lista de roles (sin datos de alumnos).

#### 1.15 Docentes y asignaciones
- **Rol · Ruta**: Director · `/docentes` y `/docentes/{id}/carga` (flag `module_teachers`).
- **Datos**: "DOCENTE PRUEBA UNO" y "DOCENTE PRUEBA DOS"; Uno: ESPAÑOL y MATEMÁTICAS en 3º A, MATEMÁTICAS en 3º B; Dos: CIENCIAS en 3º A.
- **Pasos**: crear; asignar por la carga académica (bulk); reasignar una materia a Dos; quitar una; **dar acceso al sistema** a Uno (`POST /teachers/{id}/access` crea usuario con rol `maestro`); activar con la liga.
- **Esperado**: la carga dice "2 asignaturas · 1 grupo" cuando ambas son del mismo grupo [T14-014].
- **Verificación**: `SELECT t.name, s.name, g.name FROM teaching_assignments ta JOIN teachers t … ;` · `SELECT user_id FROM teachers;`
- **Bordes**: asignación duplicada → 409 con mensaje · reasignar a un docente ocupado en ese horario → `SCHEDULE_TEACHER_BUSY` · quitar una asignación borra sus clases del horario y **conserva** las calificaciones · sin `config:read` la pantalla dice por qué no salen los grupos.

#### 1.16 Horarios
- **Rol · Ruta**: Director · `/horarios` (flags `module_config` + `module_schedule`).
- **Datos**: rejilla de PRIMARIA 07:00–14:00, pasos de 5 min; 2 clases lunes en 3º A; ausencia de Uno el próximo lunes; suplencia por Dos.
- **Pasos**: rejilla → bloques → ausencia → ver "No cubiertas" → cubrir con la sugerencia → "¿Qué está pasando ahora?" → historial.
- **Esperado**: sugerencias en orden: misma materia, mismo grado, cualquiera libre. Con el ciclo ya empezado se pide motivo, pero no al armar el horario por primera vez [T14-015]. El grado no arranca en "1°" sin nivel [T14-016].
- **Verificación**: `SELECT * FROM schedule_blocks; SELECT * FROM teacher_absences; SELECT * FROM schedule_overrides;` · `GET /api/v1/schedule/uncovered?date=…`.
- **Bordes**: docente en dos grupos a la vez → 409 (EXCLUDE) · grupo con dos clases a la vez → 409 · clase en sábado o fuera de la rejilla → rechazo · día feriado → `nonWorking`.

#### 1.17 Migración de alumnos (Excel)
- **Rol · Ruta**: Director · `/configuracion/migrar-alumnos` (flags `students` + `config` + `student_import`; permiso `students:import`, sólo owner).
- **Datos**: los 5 `importaciones/*.xlsx`, **del nivel más chico al más grande**. Sin Excel: no se puede ejecutar; usar `pnpm demo:seed` para el volumen y decirlo.
- **Pasos**: subir → vista previa (avisos y rechazos) → mapear nivel ("Bachillerato" → "Preparatoria") → confirmar → resultado. Repetir con los 5.
- **Esperado (referencia T14)**: 570 filas → 567 importados, 52 con avisos, 3 rechazados (CURP inválida, bajas en el sistema anterior). Crea los niveles, grados y grupos que faltan respetando el cupo. Inscritos **desde el primer día del ciclo**, sin cargo de inscripción, con `origin='import'` (ADR 0033, #137).
- **Verificación (sólo conteos)**: `SELECT l.name, count(*) FROM enrollments e JOIN groups g … WHERE e.origin='import' GROUP BY 1;` = pantalla · `SELECT min(enrolled_at), max(enrolled_at) FROM enrollments WHERE origin='import';` = 00:00 Morelia del inicio del ciclo.
- **Bordes**: volver a subir un archivo → "Ya estaban N, Nuevos 0" (no duplica) · archivo de otro formato o vacío → error claro · archivo de más del límite de filas/tamaño → rechazo · Cobranza o Docente → 403 · flag `student_import` OFF → 404 y la tarjeta desaparece.
- **Evidencia**: captura **recortada al resumen de conteos**. La tabla de filas muestra datos reales: no se captura.

#### 1.18 Alta manual de alumno y ficha
- **Rol · Ruta**: Director · `/alumnos/nuevo`, `/alumnos`, `/alumnos/{id}`, `/alumnos/{id}/editar`.
- **Datos**: PRUEBA CALIDAD ALUMNO UNO (3º A), ALUMNA DOS (3º B), ALUMNO TRES (3º A), con CURP sintética válida.
- **Pasos**: alta con "Crear y agregar otro" (conserva nivel y grado, limpia el resto, foco al primer campo); búsqueda sin acentos ("calidad"); filtros; abrir ficha por liga directa; editar domicilio; subir foto; datos médicos; nota (crear, editar, anular); exportar CSV.
- **Esperado**: el alumno queda inscrito y recibe su cargo de inscripción por evento (`StudentEnrolled` → finance). Matrícula consecutiva. `/alumnos/{id}` abre la ficha, no Inicio [T14-010].
- **Verificación**: `SELECT student_number FROM students WHERE last_name LIKE 'PRUEBA%';` · `SELECT external_ref, amount_cents FROM charges WHERE external_ref LIKE 'enrollment:%' AND student_id=…;` (aparece en ≤ 30 s: barrido del outbox).
- **Red/consola**: `POST /students` 201, `POST /enrollments/by-grade` 201.
- **Bordes**: CURP duplicada → 409 · guardar con 5 campos vacíos → resumen arriba, foco al primero, cada error ligado a su campo · grupo lleno → el siguiente con cupo · foto de 10 MB o no imagen → rechazo · datos médicos con Cobranza → 403 (`medical:*` owner-only) · doble clic en Guardar → un solo alumno · CSV abre bien en Excel (BOM UTF-8 [T14-011]; celdas que empiezan con `= + - @` neutralizadas — deuda conocida).

#### 1.19 Contactos y acceso a la app
- **Rol · Ruta**: Director · ficha del alumno → Contactos.
- **Datos**: madre de ALUMNA DOS, `familia.qa@garcialorca.test`.
- **Pasos**: agregar contacto; editarlo; **"Dar acceso a la app"** (`POST /students/{id}/guardians/{contactId}/invite`); copiar la liga; activar en `/activar`; quitar y volver a dar acceso a otro contacto de prueba.
- **Esperado**: se crea un usuario con rol `guardian` ligado **sólo** a esa alumna.
- **Verificación**: `SELECT * FROM guardian_students WHERE student_id=…;` · `GET /api/v1/family/students` con su token → **sólo** ALUMNA DOS.
- **Bordes**: dar acceso con un correo que ya es personal del colegio → rechazo claro · desligar → su token deja de ver a la alumna (404) · Cobranza → 403 (`guardians:write` owner-only).

### Fase III — Dinero

> Correr esta fase **entre 18:00 y 23:59 hora de Morelia** al menos una vez por entrega: en
> UTC ya es el día siguiente, y es donde salen los defectos de día de negocio (§4.2).

#### 1.20 Corrida de cargos (colegiaturas, recargos, actividades)
- **Rol · Ruta**: Director · `/configuracion` → Colegiaturas → "Generar cargos del mes" (`POST /finance/billing-runs`).
- **Pasos**: correr una vez; correr otra vez; revisar el mensaje.
- **Esperado (T14)**: con 567 importados + sintéticos, una sola corrida emite todas las colegiaturas del ciclo (567 × 10 incluyendo septiembre) y la segunda emite **0** (idempotente por `external_ref tuition:*`). Lo que venció antes de importar va **sin recargo**. Si movió una fecha límite (inscripción que caía en sábado), el mensaje lo dice [T14-022].
- **Verificación**: `SELECT kind, count(*), sum(amount_cents) FROM charges WHERE status<>'cancelled' GROUP BY kind;` · `SELECT external_ref, count(*) FROM charges GROUP BY 1 HAVING count(*)>1;` → 0 filas · `SELECT count(*) FROM charges WHERE kind='late_fee' AND student_id IN (importados) AND due_date < fecha_importación;` → 0.
- **Red/consola**: 200 en menos de 30 s; **nunca 500** [T14-001: más de 65,535 parámetros].
- **Bordes**: corrida con un tabulador desactivado → ese nivel no se cobra · corrida en un ciclo no activo → rechazo · el job programado (`monthly_billing`, diario 06:00 Morelia) **no** se detiene apagando `module_finance`: comprobar en `deploy/terraform/scheduler.tf` que `billing_cycle_id` apunta al ciclo correcto.

#### 1.21 Cobro en caja (Registro de pagos)
- **Rol · Ruta**: **Cobranza** (CAJA QA) y Director · `/pagos`.
- **Datos**: ALUMNO UNO: inscripción $4,000 + octubre $3,500 = $7,500 con efectivo $5,000 + débito $2,500. ALUMNA DOS: abono de $3,000 a la inscripción por transferencia con referencia. ALUMNO UNO: adelanto de noviembre $3,500 en efectivo.
- **Pasos**: buscar alumno → elegir cargos (o monto) → **vista previa** (`POST /finance/deposits/preview`) → formas de pago → registrar → ticket → abrir el comprobante.
- **Esperado**: folio `P-AAAA-NNNNNN` consecutivo; el ticket y el comprobante dicen grado y grupo ("3º PRIMARIA · A"), quién recibió, a qué cargos se aplicó y el **saldo restante** del abono [T14-020]. La familia recibe aviso "Pago recibido".
- **Verificación**: §3.2 y §3.3. `SELECT folio, amount_cents, method FROM deposits ORDER BY created_at DESC LIMIT 3;` y sus `deposit_tenders`.
- **Red/consola**: `POST /finance/deposits` 201.
- **Bordes**: monto mayor al adeudo → rechazo de sobrepago · suma de formas de pago distinta del total → rechazo · **doble clic / Enter dos veces** → un solo depósito (clave de idempotencia; el botón no se deshabilita, ignora el segundo clic) · tarjeta sin tipo (débito/crédito) → error · pagar después del día 10 → el recargo diario entra en la cotización · pagar antes del día 5 → pronto pago aplicado · alumna con beca pagando tarde → precio completo + recargo · cobro con el corte del día ya **cerrado** → aviso "Modificado después del cierre" (1.25) · Cobranza intenta `POST /finance/payments`, `/charges/{id}/discount`, `/charges/{id}/cancel`, `/deposits/{id}/void` → 403.
- **Evidencia**: captura del comprobante de un sintético.

#### 1.22 Cobro y ajustes desde Finanzas
- **Rol · Ruta**: Director · `/finanzas`.
- **Datos**: ALUMNA DOS: $1,000 con efectivo $600 + crédito $400. ALUMNO TRES: cargo manual "CREDENCIAL QA". Descuento de $100 a un cargo. Cancelar un cargo con motivo.
- **Pasos**: tarjetas de totales → adeudos (Todos / Pagados / Vencidos) → buscar → "Registrar pago" (con cotización previa) → cargo manual → descuento → cancelar → exportar CSV.
- **Esperado**: el pago desde Finanzas deja **folio y comprobante** como en caja [T14-003]. Los totales cubren **todos** los cargos, no 500 [T14-002]. Un cargo cancelado se ve con saldo $0 [T14-020].
- **Verificación**: "Por cobrar" de Inicio y Finanzas = §3.1 · CSV con N filas = `SELECT count(*) FROM charges WHERE cycle_id=…`.
- **Bordes**: descuento mayor al cargo → rechazo · cancelar un cargo con pagos → rechazo · `GET /finance/charges` sin filtro → 400 (lecturas acotadas) · paginación: con más de 500 cargos, la suma de las páginas = SQL.

#### 1.23 Correcciones, anulación y nota de crédito
- **Rol · Ruta**: Director (`payments:edit`, `finance:write`) · `/finanzas` → depósito.
- **Pasos**: corregir la forma de pago de un depósito (`POST /deposits/{id}/correct`) → ver historial (`GET /deposits/{id}/corrections`) → anular otro con motivo (`/void`) → usar una nota de crédito como forma de pago.
- **Esperado**: la corrección crea un depósito nuevo que reemplaza al anterior (`superseded_by`); el anulado deja sus cargos pendientes otra vez; el corte se recalcula y, si ya estaba cerrado, se marca "Modificado después del cierre". La nota de crédito va **aparte** de los ingresos (ADR 0025).
- **Verificación**: `SELECT id, folio, voided_at, superseded_by FROM deposits WHERE id IN (…);` · `SELECT * FROM deposit_corrections;` · §3.2 sin los anulados ni reemplazados.
- **Bordes**: anular dos veces → error · corregir un anulado → error · Cobranza → 403 en ambas rutas.

#### 1.24 Venta de mostrador (uniformes)
- **Rol · Ruta**: Director (`uniforms:write` + `finance:write`) · `/uniformes` → Venta.
- **Datos**: 1 playera talla 8 a ALUMNO UNO, efectivo $500 (cambio $250).
- **Pasos**: vender → comprobante de venta → Pedidos.
- **Esperado**: folio `U-AAAA-NNNNNN` **igual** en Pedidos y en el comprobante; dice quién cobró [T14-019]; existencia baja 1; el cargo de uniforme entra al corte del día.
- **Verificación**: `SELECT * FROM uniform_orders ORDER BY created_at DESC LIMIT 1;` · `SELECT on_hand FROM uniform_stock …;` · el cargo y su pago en `charges`/`payments`.
- **Bordes**: vender más de la existencia → rechazo · rol con sólo `uniforms:write` (sin `finance:write`) → la entrada se deshabilita **y dice por qué** · doble envío → una sola venta (`uniform_sale_keys`).

#### 1.25 Corte de caja: cerrar y reabrir
- **Rol · Ruta**: Director o un rol con `expenses:write` · `/finanzas` → Corte · reabrir: Director (`finance:write`).
- **Pasos**: revisar el cuadre por forma de pago y por concepto → registrar un egreso en efectivo (1.26) → cerrar con efectivo contado = esperado − $50 → registrar un cobro más → reabrir con motivo → cerrar otra vez.
- **Esperado (T14)**: "Efectivo esperado" = efectivo cobrado − egresos en efectivo; diferencia −$50 guardada; tras el cobro nuevo, "Modificado después del cierre"; la lista del corte va en orden de hora.
- **Verificación**: §3.5 · `SELECT day, expected_cash_cents, counted_cash_cents, difference_cents, reopened_at FROM cash_cut_closures;`
- **Bordes**: cerrar dos veces el mismo día → rechazo · reabrir sin motivo → error · Cobranza reabre → 403 · corte del día con más de 500 depósitos → deuda conocida (tope), anotar si se alcanza.

#### 1.26 Egresos
- **Rol · Ruta**: Director · `/egresos` (flags `finance` + `expenses`).
- **Datos**: categorías "PAPELERÍA QA" y "MANTENIMIENTO QA"; egreso $450 en efectivo con foto de comprobante; egreso $1,200 por transferencia que luego se cancela con motivo.
- **Pasos**: primera categoría (invitación) → egreso con comprobante → ver comprobante → cancelar → balance del periodo → reportes diario/semanal/mensual/anual en PDF y Excel.
- **Esperado**: los cancelados no suman; el historial guarda cada cambio (nunca se borra).
- **Verificación**: `SELECT method, sum(amount_cents) FROM expenses WHERE status='active' AND spent_on=hoy GROUP BY 1;` = balance · `SELECT kind FROM expense_events WHERE expense_id=…;`
- **Bordes**: comprobante de tipo no permitido o enorme → rechazo · editar un cancelado → error · filas de la tabla alcanzables con teclado (rol de botón) [T14-021].
- **Evidencia**: el PDF y el XLSX descargados (sólo cifras de QA).

### Fase IV — Vida académica diaria

#### 1.27 Asistencia diaria — Docente
- **Rol · Ruta**: DOCENTE PRUEBA UNO · `/asistencia` (rol `maestro`: `attendance_own:*`).
- **Pasos**: abrir hoy (día hábil) → todos presentes por omisión → marcar ausente a ALUMNO UNO con nota → guardar → cambiarlo a retardo → guardar.
- **Esperado**: "Lista guardada a las HH:MM por DOCENTE PRUEBA UNO"; segunda vez `revision=2` y la nota se conserva. La familia recibe "Ausencia registrada" y luego la corrección (evento `AbsenceRecorded`/`AttendanceMarked` → comms).
- **Verificación**: `SELECT date, taken_by_name FROM attendance_sessions WHERE group_id=…;` · `SELECT status, note FROM attendance WHERE date=…;` (sólo excepciones).
- **Bordes**: **sábado, domingo o feriado** → rechazo [T14-101] · fecha futura → "No puedes registrar una fecha futura" · más de 7 días atrás → "Solo puedes corregir hasta 7 días atrás" · fecha anterior a la inscripción del alumno → el alumno no aparece en esa lista [T14-105] · "hoy" a las 19:00 Morelia sigue siendo hoy, no mañana [#132] · grupo ajeno por API → 404 · estado inventado → 400.

#### 1.28 Asistencia — Director: correcciones, justificadas, reporte
- **Rol · Ruta**: Director · `/asistencia`.
- **Pasos**: abrir la lista que tomó el docente → justificar la falta (`excused`) con nota → tomar lista de 3º B → reporte del grupo por rango → PDF y Excel.
- **Esperado**: el reporte cuenta **todas** las listas en rango desde la inscripción: presentes + faltas + retardos + justificadas = alumnos × listas [T14-105].
- **Verificación**: `SELECT a.status, count(*) FROM attendance a JOIN attendance_sessions s ON … WHERE s.group_id=… AND s.date BETWEEN … GROUP BY 1;` = reporte.
- **Bordes**: QR (`module_student_card`) está OFF: `POST /attendance/scan` → 404 · justificar de nuevo no duplica el aviso a la familia (consumo idempotente, `processed_events`).

#### 1.29 Calificaciones — Docente
- **Rol · Ruta**: DOCENTE PRUEBA UNO · `/calificaciones`.
- **Pasos**: elegir asignación (sólo las suyas) → 1.er periodo → capturar 9 a ALUMNA DOS (3º B · MATEMÁTICAS) y 8 a ALUMNO UNO → salir del campo (guarda solo) → vaciar uno (borra).
- **Esperado**: `PUT /teaching/grades/period-grades` 200 al salir del campo; `DELETE` 204 al vaciar. Promedio y reprobados se actualizan.
- **Verificación**: `SELECT value, captured_by_name FROM period_grades WHERE enrollment_id=…;`
- **Bordes**: `8.5`, `8,5` → "Solo se permiten calificaciones enteras" · `11`, `4`, `100` → "entre 5 y 10" · `-3` → mensaje de rango, no "debe ser un número" [T14-108] · `abc` → error · por API `8.0` → se guarda 8; `"9"` en texto → 400 · asignación ajena por API (GET/PUT/DELETE) → 404 · alumno de otro grupo en su asignación → 400 · periodo que no ha empezado → deshabilitado · periodo cerrado → sólo lectura.

#### 1.30 Calificaciones — Director, cierre de periodo y boletas
- **Rol · Ruta**: Director · `/calificaciones` → Captura / Periodos.
- **Pasos**: capturar en una asignación de cualquier docente → avance del periodo (`/periods/{id}/progress`) → **cerrar** el periodo → ver boletas → PDF de la boleta de ALUMNA DOS → reabrir el periodo con motivo → cambiar una calificación → cerrar otra vez.
- **Esperado**: cerrar congela la boleta (ADR 0020) y emite `GradesPublished` → aviso a la familia. Reabrir y volver a cerrar publica **una versión nueva**; la familia ve la última.
- **Verificación**: `SELECT * FROM period_closures; SELECT student_id, version FROM report_cards;` · el PDF trae datos del colegio (1.1) y el promedio = `avg(value)` de SQL.
- **Bordes**: cerrar sin `grades:close` (cualquier rol que no sea owner) → 403 · capturar en periodo cerrado → rechazo · docente con periodo cerrado → sólo lectura.

#### 1.31 Avisos y notificaciones
- **Rol · Ruta**: Director · `/avisos` (flag `module_comms`; vista nueva con `announcements_view`).
- **Datos**: "Junta de padres 3º B (QA)" al grupo 3º B; "Simulacro QA" a PRIMARIA; "Suspensión QA" a toda la escuela; uno a ALUMNA DOS sola.
- **Pasos**: elegir destinatarios → redactar → enviar → revisar el conteo.
- **Esperado**: notificaciones = alumnos **activos** del destino (T14: 24 en 3º B = 24 inscritos activos). La familia lo ve en la app y por API.
- **Verificación**: `SELECT count(*) FROM notifications WHERE title='Junta de padres 3º B (QA)';` = `SELECT count(*) FROM enrollments WHERE group_id=… AND status='active';`
- **Bordes**: aviso sin texto → error en el envío · doble envío → un solo lote · alumnos dados de baja no reciben · Docente → 403 (no tiene `comms:*`) · la campana de la barra abre algo real o no existe [T14-104] · encabezado de nivel desplegable con teclado [T14-021].

#### 1.32 Cumpleaños
- **Rol · Ruta**: Director · `/avisos` → Mensajes automáticos → Cumpleaños.
- **Pasos**: editar la plantilla y guardar; poner a un sintético con fecha de nacimiento = hoy; disparar la corrida.
- **Disparo local**: `curl -X POST -H 'Content-Type: application/json' -d '{}' localhost:<puerto comms>/garcialorca.comms.v1.CommsService/RunBirthdays` (en GCP lo llama Cloud Scheduler a las 07:00 Morelia; `birthday_schedule_enabled` por omisión es `false`).
- **Esperado**: un aviso por alumno que cumple hoy (hoy en Morelia); correrlo dos veces no duplica (`birthday:<alumno>:<año>`).
- **Verificación**: `SELECT count(*) FROM notifications WHERE type LIKE '%birthday%' AND created_at::date = hoy;`
- **Bordes**: alumno nacido el 29-feb en año no bisiesto (documentar el comportamiento) · correr a las 23:30 Morelia (ya es mañana en UTC) → debe usar el día de Morelia.

### Fase V — Cambios de vida del alumno

#### 1.33 Cambio de grupo
- **Rol · Ruta**: Director · ficha → Cambiar de grupo (`POST /enrollments/transfer`).
- **Pasos**: mover ALUMNO TRES de 3º A a 3º B.
- **Esperado**: sólo ofrece grupos del **mismo grado**; confirma con un aviso [T14-013]; asistencia y calificaciones siguen ligadas a su historial; aparece en las listas del grupo nuevo desde hoy.
- **Verificación**: `SELECT * FROM enrollment_transfers WHERE …;` · `SELECT status FROM enrollments WHERE student_id=…;`
- **Bordes**: grupo lleno → rechazo o aviso de cupo · alumno de baja → no se puede mover.

#### 1.34 Baja con vista previa
- **Rol · Ruta**: Director · ficha → Dar de baja (necesita `students:enroll` + `finance:read` o `collections:read`).
- **Pasos**: vista previa (`GET /enrollments/{id}/withdrawal-preview`) → confirmar con fecha y motivo → filtro "Bajas".
- **Esperado (ADR 0034)**: "Se cancelarán N cargos por $X" = sólo lo **no vencido**; lo vencido queda debiéndose. La ficha queda en **sólo lectura**, también en el servidor [T14-009]. `StudentWithdrawn` → finance cancela.
- **Verificación**: `SELECT count(*), sum(amount_cents) FROM charges WHERE student_id=… AND status='cancelled' AND cancellation_reason ILIKE '%baja%';` = vista previa · `PUT /students/{id}` después de la baja → rechazo.
- **Bordes**: baja con fecha futura o anterior a la inscripción → rechazo · baja de un alumno con inscripción movida por la corrida [T14-022] · repetir la baja → error · la baja deja de mandarle avisos de grupo.

#### 1.35 Eliminar un nivel (papelera de 7 días)
- **Rol · Ruta**: Director (`levels:delete`, owner) · `/configuracion/niveles/{id}/eliminar`.
- **Datos**: un nivel **desechable** "NIVEL BORRAR QA" con 1 grado, 1 grupo, 1 alumno sintético y 1 cargo.
- **Pasos**: vista previa → confirmar con contraseña → papelera → restaurar → eliminar de nuevo.
- **Esperado (ADR 0021)**: triple validación; todo lo del nivel pasa a la papelera en cada servicio (saga); **las finanzas no se tocan**; restaurar devuelve todo.
- **Verificación**: conteos por tabla antes/después de eliminar y después de restaurar (snapshot, no sólo `count`: comparar ids) · `SELECT * FROM level_deletions;`
- **Bordes**: contraseña mal 3 veces → límite de intentos · token de confirmación vencido → rechazo · Cobranza → 403.

### Fase VI — Reportes y transversales

#### 1.36 Reportes y exportaciones
| Salida | Ruta | Verificar contra |
|---|---|---|
| CSV de adeudos | `/finanzas` → Exportar | filas = `count(*) FROM charges` del ciclo; BOM; acentos |
| CSV de alumnos | `/alumnos` → Exportar | filas = alumnos activos |
| Estado de cuenta PDF | ficha / Finanzas → `/finance/accounts/{id}/statement/pdf` | §3.4 |
| Comprobante de pago | `/comprobantes/payment/{id}` (ticket 80 mm + PDF) | `deposits` + `deposit_tenders` |
| Comprobante de venta | `/comprobantes/sale/{id}` | `uniform_orders` |
| Reporte de asistencia PDF/XLSX | `/asistencia` → Reporte | §1.28 |
| Reporte de egresos PDF/XLSX | `/egresos` → Reportes | §3.5 |
| Boleta PDF | `/calificaciones` → Boletas | `period_grades` |

En cada uno: abre, trae datos del colegio, fechas con el mes en letra, grado + grupo, montos con 2 decimales, sin datos de otro alumno.

#### 1.37 Sesión, cuenta y buscador
- **Pasos**: menú de cuenta → cambiar contraseña por voluntad propia [T14-103] → la sesión vieja muere y la nueva entra → "¿Olvidaste tu contraseña?" (`/auth/password-reset`) → ⌘K "cobrar" → Registrar pago → cerrar sesión.
- **Bordes**: 6+ intentos de login fallidos → límite (429) igual para cuenta inexistente · contraseña nueva de menos de 8 → 400 · clic en el propio nombre no cierra sesión sin preguntar · liga directa estando sin sesión → login y luego regresa a esa liga (`?next=` sólo rutas propias).

### Fase VII — App de familia (release APK)

#### 1.38 Entrada y sesión
- **Rol**: FAMILIA (madre de ALUMNA DOS) · app release en el emulador.
- **Pasos**: abrir → Entrar → cerrar la app a la fuerza y reabrir (la sesión sigue) → cambiar su contraseña por API → esperar a que venza el access token → cambiar de pestaña.
- **Esperado**: con la sesión revocada, la app manda a **Entrar** con un mensaje, no a "Revisa tu conexión" ni a un Inicio con guiones [T14-102].
- **Verificación**: `POST /auth/mobile/refresh` con el refresh viejo → 401.

#### 1.39 Pantallas
| Pantalla | Qué revisar | Contra |
|---|---|---|
| Inicio | nombre, "3º PRIMARIA · B" (con grado), saldo, asistencia de hoy con el ícono correcto [T14-108], avisos nuevos | API `/family/students`, §3.4 |
| Asistencia | mes con faltas, retardos y justificadas; día corregido dice "Asistió" | `/family/students/{id}/attendance?month=` |
| Calificaciones | periodos, materias, promedio; boletas publicadas y su PDF | `/grades/periods`, `/report-cards` |
| Cuenta | total, "A pagar hoy" (sólo lo vencido a hoy en Morelia), movimientos en orden útil, becas, saldo corrido sin negativos falsos [T14-106]; PDF del estado de cuenta; **comprobante de cada pago** [T14-107] | `/statement`, `/receipts/{depositId}/pdf` |
| Avisos | lista, abrir marca leído (contador −1), aviso de grupo, de pago, de asistencia, de boleta | `/family/notifications`, `read_at` |
| Más | perfil del alumno; tarjetas ocultas por flag (Documentos, Uniformes, Facturas, Becas, Credencial) **no aparecen** | `bootstrap.ts` |
| Cerrar sesión | vuelve a Entrar, sin congelarse | — |

#### 1.40 Alcance y fugas (API de familia)
- Para cada ruta por alumno (`profile`, `statement`, `statement/pdf`, `account`, `grades`, `grades/periods`, `report-cards`, `report-cards/{id}`, `report-cards/{id}/pdf`, `attendance`, `receipts/{depositId}`, `card`, `photo`) probar 4 ids: su hija (200; `card`/`photo` 404 por flag), otro sintético (404), un alumno real **sin leer su respuesta** (404), UUID inexistente (404). Los 404 deben ser **idénticos** (mismo cuerpo).
- Recibo de un depósito de otro alumno con el id de su hija en la ruta → 404.
- Token de familia contra rutas de personal (`/students`, `/finance/*`, `/comms/*`) → 403. Cookie de docente contra `/family/*` → 403.
- Red lenta / sin red: ver §4.4.

---

## 2. Matriz de permisos

### 2.1 Roles sembrados (`migrations/iam`)

| Rol | Permisos | Superficie |
|---|---|---|
| `owner` (Director) | todos, incluidos los owner-only y `family:read` (ADR 0012) | web completa |
| `cobranza` | `students:read`, `students:list`, `collections:read`, `payments:write` | web: Registro de pagos, Estudiantes (lectura) |
| `maestro` | `teaching:read`, `teaching:write`, `attendance_own:read`, `attendance_own:write` | web: Asistencia, Calificaciones, Horarios (lo propio) |
| `guardian` | `family:read`, `uniforms_own:read`, `uniforms_own:write` | app móvil (`/family/*`); en web ve "No tienes módulos asignados" |
| delegables (rol a la medida) | `expenses:*`, `schedule:*`, `activities:*`, `scholarships:*`, `teachers:*`, `uniforms:stock`, `uniforms:write`, `payments:edit`, `students:*` no owner-only | lo que se arme en 1.14 |

**Owner-only** (el guard de iam rechaza dárselos a otro rol): `finance:read`, `attendance:read`,
`comms:read`, `guardians:read`, `guardians:write`, `uniforms:orders`, `medical:read`, `medical:write`,
`students:notes_void`, `grades:close`, `levels:delete`, `users:delete`, `students:import`.

### 2.2 Permitido / negado por rol

| Módulo (ruta web) | Director | Cobranza | Docente | Familia |
|---|---|---|---|---|
| Inicio `/` | todas las tarjetas | Pagos, Estudiantes | Asistencia, Calificaciones, Horarios | "sin módulos" |
| Configuración `/configuracion` | ✅ | ❌ 403 | ❌ 403 | ❌ |
| Migrar alumnos | ✅ | ❌ | ❌ | ❌ |
| Administración `/administracion` | ✅ | ❌ | ❌ | ❌ |
| Estudiantes `/alumnos` | ✅ alta, edición, baja, médica, notas | 👁 lectura (sin médica) | ❌ | ❌ |
| Contactos / acceso app | ✅ | ❌ | ❌ | ❌ |
| Docentes `/docentes` | ✅ | ❌ | ❌ | ❌ |
| Horarios `/horarios` | ✅ todo | ❌ | 👁 sólo su semana (`/teaching/schedule`) | ❌ |
| Registro de pagos `/pagos` | ✅ | ✅ cobrar, ver saldo de un alumno, vencidos, corte (lectura) | ❌ | ❌ |
| Finanzas `/finanzas` | ✅ | ❌ (ledger completo, descuentos, cancelar, anular, ingresos) | ❌ | ❌ |
| Corregir pago | ✅ (`payments:edit`) | ❌ | ❌ | ❌ |
| Egresos / cerrar corte | ✅ | ❌ (salvo `expenses:write` delegado) | ❌ | ❌ |
| Reabrir corte | ✅ (`finance:write`) | ❌ | ❌ | ❌ |
| Becas, Actividades | ✅ | ❌ | ❌ | ❌ |
| Inventario | ✅ (stock + pedidos + venta) | ❌ | ❌ | ❌ |
| Asistencia | ✅ todos los grupos + reportes | ❌ | ✅ sólo sus grupos, ±7 días, días hábiles | 👁 su hija (app) |
| Calificaciones | ✅ todas + cierre + boletas | ❌ | ✅ sólo sus asignaciones, periodo abierto | 👁 su hija (app) |
| Avisos | ✅ | ❌ | ❌ | 👁 los suyos (app) |
| `/family/*` | ✅ (tiene `family:read`, pero sin hijos ligados) | ❌ 403 | ❌ 403 | ✅ sólo sus hijos |

### 2.3 Cómo probar la negación

1. **Barrido de API por rol** (T14: 109 GET con la cookie del docente → 6 × 200).
   Extraer las rutas del gateway (Apéndice B), sustituir `{id}` por un UUID válido del
   colegio y llamar cada una con el archivo de cookies del rol:
   ```sh
   while read m p; do
     printf '%s %s %s\n' "$(curl -s -o /dev/null -w '%{http_code}' -X "$m" -b doc.jar "localhost:8080$p")" "$m" "$p"
   done < rutas-get.txt | sort | uniq -c -w3
   ```
   Esperado: 200 sólo en lo que la tabla 2.2 permite; **403** por permiso; **404** por flag
   OFF o id ajeno (nunca 403 por un id ajeno: no filtra si existe). Las escrituras (POST/PUT/DELETE)
   se prueban con cuerpo vacío: deben dar 403 **antes** de validar el cuerpo.
2. **Web**: escribir la URL de un módulo prohibido → regresa a Inicio (UX); la tarjeta no aparece.
3. **Escalada**: con un usuario que tenga `users:write` y `roles:write` pero no owner,
   intentar darse `finance:read` o el rol owner → rechazo.
4. **Cambio en caliente**: el Director da o quita una asignación al docente con la sesión
   del docente abierta → se refleja **sin volver a entrar** (T14: quitar → 404 al instante).
5. **Suspensión**: suspender a CAJA QA con sesión abierta → la siguiente llamada da 401.

### 2.4 Flag OFF (por módulo)

Para cada flag de la lista de terraform: arrancar el gateway sin él y comprobar:
- sus rutas → **404 antes de autenticar** (también sin cookie);
- las rutas compuestas que lo usan → 404 (p. ej. sin `students`, todo `/family/*`; sin `finance`, `/expenses*`, `/activities*`, `/scholarships*`, `/receipts/*`);
- la web oculta la tarjeta y el buscador no la ofrece;
- los demás módulos siguen funcionando (un módulo apagado no tumba a otro).
- **Lo que un flag NO apaga** (verificar que el reporte lo diga): las corridas programadas
  (`monthly_billing`, `daily_birthdays`), los consumidores de Pub/Sub y las becas ya asignadas.

---

## 3. Pruebas de dinero

Todo en centavos. Cada cifra de pantalla se cuadra con una consulta escrita **por el QA**,
no con otra pantalla. Los nombres de columna salen de `migrations/finance`; si una consulta
falla por una columna, corregirla y anotar el cambio de esquema.

### 3.1 Cargos (Por cobrar)
```sql
-- Saldo pendiente bruto por cargo = monto − descuento − beca liquidada − pagado.
WITH paid AS (
  SELECT p.charge_id, sum(p.amount_cents) AS paid
  FROM payments p JOIN deposits d ON d.id = p.deposit_id
  WHERE d.voided_at IS NULL AND d.superseded_at IS NULL
  GROUP BY 1)
SELECT count(*) FILTER (WHERE c.amount_cents - c.discount_cents - coalesce(c.scholarship_cents,0) - coalesce(paid,0) > 0) AS cargos_con_saldo,
       sum(c.amount_cents - c.discount_cents - coalesce(c.scholarship_cents,0) - coalesce(paid,0)) AS saldo_bruto
FROM charges c LEFT JOIN paid ON paid.charge_id = c.id
WHERE c.cycle_id = :ciclo AND c.status <> 'cancelled';
```
- Pantalla "Por cobrar" = `saldo_bruto` − **becas dinámicas** de los cargos pendientes
  (la beca de un cargo no pagado se calcula al leer; T14: 9 × $700). Calcularlas a mano por
  alumno becado: `% × monto` (o fijo) por cargo pendiente del concepto cubierto.
- Pagos de la ruta vieja `POST /finance/payments` no tienen `deposit_id`: incluirlos aparte
  (`deposit_id IS NULL`) si los hubo.
- Duplicados: `SELECT external_ref, count(*) FROM charges WHERE external_ref <> '' GROUP BY 1 HAVING count(*) > 1;` → 0.
- Estados: `SELECT status, count(*) FROM charges GROUP BY 1;` = filtros Todos / Pagados / Vencidos.

### 3.2 Pagos y depósitos (Cobrado)
```sql
SELECT sum(d.amount_cents) FROM deposits d
WHERE d.voided_at IS NULL AND d.superseded_at IS NULL
  AND (d.paid_at AT TIME ZONE 'America/Mexico_City')::date = :dia;
-- = suma de sus aplicaciones a cargos:
SELECT sum(p.amount_cents) FROM payments p JOIN deposits d ON d.id = p.deposit_id
WHERE d.voided_at IS NULL AND d.superseded_at IS NULL
  AND (d.paid_at AT TIME ZONE 'America/Mexico_City')::date = :dia;
```
- Depósito = Σ sus `payments` (ningún centavo sin aplicar, salvo adelanto explícito).
- Folios: `SELECT folio FROM deposits ORDER BY folio;` sin huecos ni repetidos.
- Idempotencia: `SELECT idempotency_key, count(*) FROM deposits WHERE idempotency_key <> '' GROUP BY 1 HAVING count(*)>1;` → 0.

### 3.3 Formas de pago (tenders)
```sql
SELECT t.method, t.card_type, sum(t.amount_cents)
FROM deposit_tenders t JOIN deposits d ON d.id = t.deposit_id
WHERE d.voided_at IS NULL AND d.superseded_at IS NULL
  AND (d.paid_at AT TIME ZONE 'America/Mexico_City')::date = :dia
GROUP BY 1, 2;
```
- Por depósito: `Σ tenders = deposits.amount_cents` (consulta con `HAVING` ≠ → 0 filas).
- `credit_note` va **aparte** del ingreso (no suma a Efectivo/Tarjeta/Transferencia).
- Cuadra con el corte "por forma de pago" y con el balance de Egresos.

### 3.4 Becas, recargos, pronto pago y estado de cuenta (por alumno sintético)
- Cargado = Σ `charges.amount_cents` no cancelados del alumno.
- Becas = por cargo cubierto y **pagado a tiempo o pendiente y no vencido**: % o fijo; pagado tarde → 0 y recargo.
- Recargos: `SELECT external_ref, amount_cents FROM charges WHERE kind='late_fee' AND student_id=:a;` — diario: `días hábiles/naturales de atraso × tarifa` según el tabulador; actividad: fijo, una vez (`latefee:<cargo>:actividad`).
- Pronto pago: aplicado sólo si `paid_at` (Morelia) ≤ fecha de pronto pago.
- Saldo = cargado − descuentos − becas − pagado. Debe ser igual en: caja, Finanzas, estado de
  cuenta web, PDF, app de familia (T14: $35,500 − $6,300 − $4,000 = $25,200 en las 5 vistas).
- Redondeo: un 20 % sobre montos con centavos impares (p. ej. $3,333.33) → documentar si
  redondea a favor de la familia y que la suma de las líneas = total (sin ±1 centavo).

### 3.5 Corte de caja y egresos
```sql
-- Efectivo esperado del día (Morelia)
SELECT
 (SELECT coalesce(sum(t.amount_cents),0) FROM deposit_tenders t JOIN deposits d ON d.id=t.deposit_id
   WHERE t.method='cash' AND d.voided_at IS NULL AND d.superseded_at IS NULL
     AND (d.paid_at AT TIME ZONE 'America/Mexico_City')::date=:dia)
 -
 (SELECT coalesce(sum(amount_cents),0) FROM expenses WHERE method='cash' AND status='active' AND spent_on=:dia)
 AS esperado;
SELECT expected_cash_cents, counted_cash_cents, difference_cents, snapshot FROM cash_cut_closures WHERE day=:dia;
```
- `difference = counted − expected`; el `snapshot` del cierre no cambia al cobrar después; la pantalla
  marca "Modificado después del cierre".
- Ingresos por concepto (inscripción / colegiatura / uniformes / actividades / otros) = Σ `payments` por `charges.kind`.

### 3.6 Cancelaciones (baja) y anulaciones
- Baja: cancelados = vista previa (conteo y monto); ningún cargo vencido cancelado.
- Anulado: sus `payments` ya no cuentan en 3.1–3.5; sus cargos vuelven a `pending`/`partially_paid`.
- Corrección: el depósito viejo tiene `superseded_at`; sólo el nuevo cuenta.

---

## 4. Pruebas no funcionales mínimas por entrega

### 4.1 Volumen real
- Base con **≥ 567 alumnos** (los 5 Excel en local, o `SEED_STUDENTS_PER_LEVEL` para sintéticos) y **≥ 5,000 cargos** (una corrida completa).
- Medir: corrida de cargos (< 30 s, sin 500), Inicio y Finanzas (< 3 s), CSV de adeudos completo, búsqueda de alumno (< 1 s), aviso a toda la escuela, reporte de asistencia de un mes.
- Toda lista o total: comparar con SQL. Un número redondo (500, 1000) es sospechoso: buscar un tope o una página perdida [T14-002].

### 4.2 Zona horaria de Morelia
- Correr cobros, asistencia y avisos entre **18:00 y 23:59 Morelia**: "hoy" = día de Morelia en el corte, la lista del día, "A pagar hoy", recargos y cumpleaños.
- Fecha límite el día 10: pagar el 10 a las 23:30 Morelia → sin recargo; el 11 a las 00:05 → con recargo.
- Fines de semana y feriados: asistencia rechaza, horarios marca `nonWorking`, la corrida mueve fechas límite que caen en sábado/domingo y lo dice.

### 4.3 Sesiones
- Sesión de personal: dura la jornada (ADR 0013); refresh después de vencer el access token sin perder lo capturado.
- Cambiar la contraseña revoca refresh tokens (ADR 0005) en web y móvil.
- Reinicio de iam con `IAM_JWT_SECRET` fijo **no** cierra sesiones; sin él, sí (anotar como ENTORNO).
- Límite de intentos de login y de reautenticación (429).

### 4.4 App: sin red y red lenta
- Modo avión con la app abierta → mensaje de "sin conexión" con Reintentar; al volver la red, recupera sin reiniciar.
- Red lenta (emulador: `adb emu network delay gprs` / `network speed edge`) → estados de carga, sin pantallas en blanco, sin dobles envíos; la Cuenta carga en un tiempo razonable (T14: ~10 s en Expo Go; medir en release).
- Jalar para actualizar en cada pestaña.

### 4.5 Accesibilidad básica
- Todo control alcanzable y operable con teclado; foco visible; `Esc` cierra diálogos (pregunta si hay cambios); el foco vuelve al botón que abrió.
- Etiqueta visible sobre cada campo; errores ligados con `aria-describedby`; resumen de errores al enviar.
- Contraste ≥ 4.5:1 en etiquetas; objetivos ≥ 44 × 44 px; texto de captura ≥ 16 px.
- App: TalkBack lee cada fila una sola vez y los botones son botones.
- Radix: ningún `DialogContent` sin `Description` (advertencia en consola) [T14-021].

### 4.6 Consola y red limpias
- Cero errores de la app en consola en todas las pantallas recorridas; cero 5xx; 4xx sólo los provocados.
- Build web: `find apps/web/.next -name '*.map'` y `grep -r sourceMappingURL` vacíos.

### 4.7 Eventos y trabajos en segundo plano
- Outbox de finance, students y grades: tras cada acción, el evento sale en ≤ 30 s. `SELECT count(*) FROM finance_outbox WHERE …pendiente…;` → 0 después de 1 min (igual `students_outbox`, `grades_outbox`).
- Comms idempotente: re-entregar un evento no duplica la notificación (`processed_events`).
- DLQ vacías al final (`payment-events-dlq`, `attendance-events-dlq`, `student-events-dlq`, `grades-events-dlq`) en el emulador.
- Papelera de niveles: la purga del gateway borra lo que pasó de 7 días (adelantar con un lote de prueba).

---

## 5. Plantilla de reporte

Archivo: `.gstack/qa-reports/qa-report-<objetivo>-<AAAA-MM-DD>.md`. Capturas en
`.gstack/qa-reports/screenshots/<objetivo>/`. Nada de esto se commitea.

```markdown
# QA <objetivo> · <fecha>

- Alcance: … · Rama/commit: … · Stack: producción local / desarrollo (cuál)
- Flags: <lista ENABLED_MODULES> · App: APK release <versión> en <AVD>
- Privacidad: sólo conteos de los Excel; detalle sobre sintéticos.

## 1. Qué se vació y qué se creó por la UI
## 2. Migración (sólo conteos)
## 3. Resultado por paso (tabla: paso · rol · resultado · issues)
## 4. Permisos (barrido por rol: 200/403/404) y flags OFF
## 5. Dinero: pantalla vs SQL (tabla con ✅/❌)
## 6. No funcionales
## 7. ENTORNO (no son defectos de la app)
## 8. Defectos
## 9. Credenciales sintéticas creadas · estado en que queda el entorno
## 10. Evidencia (lista de archivos; ninguno con datos reales)
```

### 5.1 Formato de cada defecto

```markdown
### ISSUE-<objetivo>-NNN · <Severidad> · <Módulo> — <qué pasa, en una línea>
- Rol / ruta / datos: …
- Repro: pasos numerados, mínimos.
- Esperado: … (cita la regla, ADR o decisión del dueño si existe)
- Real: … (código HTTP, mensaje, log del servicio, consulta SQL que lo prueba)
- Alcance: ¿a cuántos alumnos/cargos afecta? ¿hay datos ya dañados?
- Evidencia: captura / archivo.
- Regresión propuesta: el test que debería fallar hoy (unit / integración / e2e).
```

### 5.2 Escala de severidad

| Severidad | Criterio | Ejemplos T14 | ¿Bloquea la entrega? |
|---|---|---|---|
| **Critical** | dinero incorrecto guardado, fuga de datos entre familias o roles, pérdida de datos, el colegio no puede cobrar o entrar | — | Sí |
| **High** | una función principal falla con el tamaño real, o una cifra de dinero mostrada es falsa | 001 corrida 500 · 002 totales truncados a 500 | Sí |
| **Medium** | flujo correcto pero un rol se queda sin salida, datos inconsistentes entre vistas, regla de negocio violada sin daño de dinero | 003 pago sin folio · 101 lista en sábado · 102 sesión muerta · 105 listas no cuentan | Decide el dueño |
| **Low** | texto, formato, orden, accesibilidad puntual, clic de más | 011 BOM · 012 fechas ISO · 021 teclado | No |

### 5.3 ENTORNO aparte

Va en su propia sección y **no** cuenta como defecto: reinicios de `air`/iam que cierran
sesiones, Expo Go congelado, push sin EAS, sin correo local, diálogo de impresión que
bloquea la automatización, puertos ocupados, Metro viejo sirviendo un bundle en caché,
errores de mis propios `curl`. Si un problema de entorno se sospecha que también pasa en
producción, se reproduce en el stack de modo producción antes de subirlo a defecto.

---

## Apéndice A — Inventario del sistema

### A.1 Módulos

| Módulo | Pantallas web (ruta) | Rutas del gateway | Permiso | Flag(s) | Servicios | Eventos |
|---|---|---|---|---|---|---|
| Sesión / cuenta | `/entrar`, `/cambiar-contrasena`, `/activar`, menú de cuenta | `/auth/login`, `/auth/refresh`, `/auth/logout`, `/auth/google`, `/auth/change-password`, `/auth/password-reset`, `/auth/invitations/accept`, `/auth/mobile/{login,refresh,google}`, `/me`, `/features` | público / autenticado | — | gateway, iam | — |
| Administración | `/administracion` (usuarios, roles, liga docente↔usuario) | `/users*` (crear, invitar, reenviar, estado, roles, eliminar), `/roles*`, `/teachers/{id}/user` | `users:read/write/delete`, `roles:read/write` | `iam` | iam, config | — |
| Configuración | `/configuracion` (datos, ciclo, niveles, grados, trimestres, periodos, materias, grupos, calendario, eventos), `/configuracion/niveles/{id}/periodos`, `/configuracion/niveles/{id}/eliminar` | `/config/*` (≈35 rutas) | `config:read/write`; `levels:delete` | `config` (+ vista `config_setup`) | config (+ grades: scale/term guard; saga de borrado con students, grades, attendance, finance, uniforms) | — |
| Colegiaturas | `/configuracion` → Colegiaturas (tabulador) | `/finance/tuition-plans`, `/finance/billing-runs` | `finance:read/write` | `finance` | finance | `ChargeSettled`, `PaymentRegistered` (vía outbox) |
| Migración de alumnos | `/configuracion/migrar-alumnos` | `/imports/students/{preview,commit}` | `students:import` (owner) | `students`+`config`+`student_import` | students, config, finance | `StudentEnrolled` |
| Estudiantes | `/alumnos`, `/alumnos/nuevo`, `/alumnos/{id}`, `/alumnos/{id}/editar` | `/students*` (ficha, foto, médica, notas), `/enrollments*` (alta, por grado, cambio, baja, vista previa de baja) | `students:create/read/update/enroll`; `medical:*`, `students:notes_void` (owner) | `students` | students (+ finance para la vista previa) | `StudentEnrolled`, `StudentWithdrawn` |
| Contactos / acceso app | ficha → Contactos | `/contacts`, `/guardians`, `/guardians/unlink`, `/students/{id}/guardians`, `/students/{id}/contacts/{cid}`, `/students/{id}/guardians/{cid}/invite` | `guardians:read/write` (owner) | `students` | students, iam | — |
| Credencial virtual | ficha → Verificar credencial | `/students/card/verify`, `/students/{id}/card/{revoke,reissue}`, `/attendance/scan`, `/teaching/attendance/scan`, `/family/students/{id}/{card,photo}` | `students:read/update`, `attendance:read`, `attendance_own:read`, `family:read` | `student_card` (**OFF**) | students, attendance | — |
| Docentes | `/docentes`, `/docentes/{id}/carga` | `/teachers`, `/teachers/{id}/access`, `/teaching-assignments*` (incl. `bulk`) | `teachers:read/write` (+`users:write` para acceso) | `teachers` (+`iam`) | config, iam | — |
| Horarios | `/horarios` | `/schedule*` (tablero, día, libres, eventos, bloques, cambios, ajustes, ausencias, no cubiertas), `/teaching/schedule{,/day}` | `schedule:read/write`; `teaching:read` | `config`+`schedule` | config | — |
| Registro de pagos | `/pagos` | `/finance/deposits{,/preview}`, `/finance/accounts/{id}{,/statement,/statement/pdf}`, `/finance/expected`, `/finance/overdue`, `/finance/cash-up`, `/finance/charges` (un alumno) | `collections:read`, `payments:write` (o `finance:*`) | `finance` | finance, iam (nombres) | `PaymentRegistered`, `ChargeSettled` |
| Finanzas | `/finanzas` (totales, adeudos, conceptos, corte, ingresos) | `/finance/concepts*`, `/finance/charges*` (crear, generar, descuento, cancelar), `/finance/payments`, `/finance/deposits/{id}/{void,correct,corrections}`, `/finance/income` | `finance:read/write`, `payments:edit` | `finance` | finance | ídem |
| Comprobantes | `/comprobantes/{payment\|sale}/{id}` | `/receipts/payment/{id}{,/pdf}`, `/receipts/sale/{id}{,/pdf}` | `finance:read`/`collections:read`; `uniforms:orders` | `finance` / `uniforms` | finance, uniforms, students, config, iam | — |
| Egresos y corte | `/egresos` | `/expense-categories*`, `/expenses*` (resumen, reportes JSON/PDF/XLSX, comprobante), `/finance/cash-up/{close,reopen}` | `expenses:read/write`; reabrir `finance:write` | `finance`+`expenses` | finance, config | — |
| Becas | `/becas` | `/scholarships*` | `scholarships:read/write` | `finance`+`scholarships` | finance | — |
| Actividades | `/actividades` | `/activities*` | `activities:read/write` | `finance`+`activities` | finance | — |
| Asistencia | `/asistencia` (docente: pase de lista; director: listas, reporte) | `/attendance{,/roster,/reports{,/pdf,/xlsx}}`, `/teaching/attendance{,/classes,/roster}` | `attendance:read/write` (owner lectura); `attendance_own:*` | `attendance` (+`config`) | attendance, config, students, iam | `AttendanceMarked`, `AbsenceRecorded` |
| Calificaciones y boletas | `/calificaciones` (captura docente / director, periodos, boletas) | `/grades*` (period-sheet, period-grades, periods, close/reopen, report-cards, pdf; legado roster/capture), `/teaching/{assignments,terms,roster,grades*}` | `grades:read/write`, `grades:close` (owner); `teaching:*` | `grades` (+`config`) | grades, config, students, iam | `GradesPublished` |
| Avisos y cumpleaños | `/avisos` (+ vista `announcements_view`) | `/comms/announcements`, `/comms/notifications{,/{id}/read}`, `/comms/devices`, `/comms/birthday-template` | `comms:read` (owner), `comms:write` | `comms` | comms, students, config | consume payment-, attendance-, grades-events |
| Inventario (uniformes) | `/uniformes` (catálogo, inventario, venta, pedidos) | `/uniforms/{levels,items*,stock*,counter-sale,orders*,settings}` | `uniforms:stock`, `uniforms:write`, `uniforms:orders` (owner); venta + `finance:write` | `uniforms` | uniforms, finance, config | cargo vía `external_ref` |
| Familia (app) | app: Inicio, Asistencia, Calificaciones, Boleta, Cuenta, Avisos, Más, Credencial, Eventos | `/family/notifications*`, `/family/devices*`, `/family/students` y `/family/students/{id}/{account,statement,statement/pdf,receipts/{dep}{,/pdf},profile,grades,grades/periods,report-cards*,attendance,card,photo}` | `family:read` | `students` + el del dato (`comms`, `finance`, `grades`, `attendance`, `student_card`) | students, comms, finance, grades, attendance, config | — |
| Superbuscador | ⌘K en toda la web | — | por tarjeta | — | — | — |
| Documentos | tarjeta oculta | — | `documents:read` | `docs` | docs (stub: sólo `Health`) | — |
| Facturación CFDI | tarjeta oculta | — | `billing:read` | `billing` | billing (stub) | — |
| Reportes | tarjeta oculta | — | `reports:read` | `reports` | — | — |
| Vista nueva general | Inicio por áreas + estilo | — | — | `ui_refresh` | — | — |

### A.2 Servicios y RPC (de `proto/`)

| Servicio | RPC | Notables |
|---|---|---|
| iam | 23 | Login, LoginWithIDToken, RefreshToken, ChangePassword, Invite/AcceptInvitation, RequestPasswordReset, users/roles CRUD, VerifyPassword |
| config | 86 | catálogo, calendario, perfil, docentes, asignaciones, horarios, ausencias, saga de borrado de nivel (Begin/Cancel/Fail/Sweep/Purge/Restore) |
| students | 42 | alta, ficha, foto, médica, notas, credencial, inscripción/cambio/baja, **ImportStudents**, contactos/acceso, archivo de nivel |
| finance | 65 | conceptos, cargos, pagos, depósitos, corte, egresos, actividades, becas, tabulador, **RunBilling**, **DrainOutbox**, ExportContpaqi (**stub**) |
| grades | 27 | captura por periodo, cierre/reapertura, boletas, outbox; mensuales (retiradas, sin ruta) |
| attendance | 12 | lista diaria, sesiones, resumen, archivo de nivel |
| comms | 9 | dispositivos, notificaciones, Announce, **RunBirthdays**, plantilla |
| uniforms | 27 | catálogo, stock, venta; **etapa 2 sin ruta**: CreateOrder, PickLines, Dispatch/CancelFulfillment, ExtendDeadline, ExpireDueFulfillments, ResumeStuckPicking, DrainOutbox |
| docs, billing | 1 c/u | stubs |

### A.3 Trabajos programados y en segundo plano

| Trabajo | Dónde | Frecuencia | Lo detiene |
|---|---|---|---|
| Corrida de cargos `RunBilling` | Cloud Scheduler `monthly_billing` → finance | `billing_schedule_cron` (diario 06:00 Morelia desde T17) | `billing_schedule_enabled=false` o `billing_cycle_id=""` + apply (**no** el flag) |
| Cumpleaños `RunBirthdays` | Cloud Scheduler `daily_birthdays` → comms | 07:00 Morelia | `birthday_schedule_enabled` (por omisión `false`) |
| Barrido de outbox | finance, students, grades (goroutine) | cada 30 s | — |
| Recibos de push Expo | comms (`expo/receipts.go`) | intervalo fijo | — |
| Purga de papelera de niveles | gateway (`levelarchive.StartPurger`) | cada hora | — |
| Vencimiento de apartados de uniformes | `ExpireDueFulfillments` | **sin llamador** | — |

### A.4 Pub/Sub

| Tópico | Productor | Suscripción → consumidor | Efecto |
|---|---|---|---|
| `payment-events` | finance (`PaymentRegistered`, `ChargeSettled`) | `comms-payment-events` → comms · `students-payment-events` → students | aviso "Pago recibido" · inscripción pagada ⇒ alumno "inscrito" |
| `attendance-events` | attendance (`AttendanceMarked`, `AbsenceRecorded`) | `comms-attendance-events` → comms | aviso de falta / corrección / justificada |
| `student-events` | students (`StudentEnrolled`, `StudentWithdrawn`) | `finance-student-events` → finance | cargo de inscripción · cancelar lo no vencido |
| `grades-events` | grades (`GradesPublished`) | `comms-grades-events` → comms | aviso de boleta |
| `*-dlq` | — | `*-dlq-drain` | mensajes agotados |

### A.5 Documentos generados

Estado de cuenta PDF (personal y familia) · comprobante de pago JSON/ticket 80 mm/PDF (personal
y familia) · comprobante de venta JSON/PDF · boleta PDF (personal y familia) · reporte de
asistencia PDF/XLSX · reporte de egresos PDF/XLSX · CSV de adeudos y de alumnos (web) ·
plantilla Excel de importación (lectura).

### A.6 Huecos encontrados en el inventario (17-oct-2026)

1. **Flags fuera de terraform**: `ui_refresh` y `announcements_view` están encendidos en dev a mano
   (`gcloud run services update`), pero no en `services.tf`: el próximo `terraform apply` los apaga.
   `config_setup` está en terraform pero no en la lista local de compose.
2. **`module_student_card` OFF en todas partes**; la app no lee `/features` (flags fijos al compilar):
   encender la credencial pide un build nuevo de la app.
3. **Uniformes etapa 2 sin superficie**: los RPC de pedidos de familia, surtido, entrega, prórroga y
   vencimiento existen, pero no hay rutas en el gateway, ni pantallas, ni llamador de
   `ExpireDueFulfillments`/`DrainOutbox`. El rol `guardian` tiene `uniforms_own:*` sin ninguna ruta que lo use.
   La tarjeta "Uniformes" de la app está oculta.
4. **App sin backend en dos pantallas**: Eventos/agenda (los `school_events` de config no tienen ruta
   `/family`) y la hoja para justificar una falta (`ExcuseSheet`, sin uso ni ruta).
5. **Stubs**: `docs`, `billing` (CFDI), `reports` y `ExportContpaqi`: sin implementación; tarjetas ocultas.
6. **Push sin EAS**: sin `EAS_PROJECT_ID` no hay entrega real de push; los avisos sólo se ven en la bandeja.
7. **Reporte de asistencia del docente** de su propio grupo: diferido (`attendance_own` no lo abre).
8. **Deuda de API**: ruta vieja `POST /finance/payments` (sin folio por diseño), rutas de comprobantes del
   personal sin documentar en OpenAPI, tipos de la app escritos a mano, mezcla de formatos de error
   (sobre JSON vs texto) [T14-108].
9. **Calificaciones mensuales retiradas**: los RPC `*Monthly*` siguen en grades sin ruta.
10. **Corridas que ningún flag apaga** (corrida de cargos, cumpleaños, consumidores, becas): correcto por
    diseño, pero hay que decirlo en cada reporte de flags OFF.
11. Pendientes del dueño vigentes: ortografía del grado (D-B), dos esquemas de periodos (006), corte con
    tope de 500 depósitos, CSV sin neutralizar fórmulas.

---

## Apéndice B — Cómo regenerar el inventario

```sh
# Rutas del gateway con su envoltura (permiso/flag)
grep -nE '"(GET|POST|PUT|PATCH|DELETE) /api' services/gateway/cmd/main.go services/gateway/internal/handler/*.go
# Sólo GET, para el barrido de permisos (sustituir {params} a mano)
grep -oE '"GET /api[^"]+"' services/gateway/cmd/main.go | tr -d '"' > rutas-get.txt
# RPC por servicio
for s in proto/garcialorca/*/; do echo "$s $(grep -c 'rpc ' $s/v1/*.proto)"; done
# Roles sembrados y owner-only
grep -n -A6 'INSERT INTO roles' migrations/iam/*.sql
grep -n '":' services/iam/internal/domain/permission.go
# Pantallas web y su permiso/flag
sed -n '/BY_SCREEN = {/,/} as const/p' apps/web/app/routing/routes.ts
grep -n 'flag: "module_' apps/web/app/_design/data.ts
# App móvil
find apps/mobile/app -name '*.tsx'; cat apps/mobile/src/flags/bootstrap.ts
# Flags por ambiente
grep -n ENABLED_MODULES docker-compose.yml deploy/terraform/services.tf
# Trabajos y eventos
grep -nE 'google_cloud_scheduler_job|uri ' deploy/terraform/scheduler.tf
grep -nE 'google_pubsub_(topic|subscription)"|name +=' deploy/terraform/pubsub.tf
```
