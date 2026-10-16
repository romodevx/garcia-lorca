# PLAN — Remediación del QA completo (oct 2026)

- **Origen:** QA completo local del 2026-10-15 (`.gstack/qa-reports/run-20261015T204242Z/qa-report-localhost-2026-10-15.md`): 28 hallazgos (4 High · 9 Medium · 15 Low).
- **Estado:** BORRADOR — pendiente de aprobación del dueño (checkpoint humano).
- **Fuera de alcance:** pendientes de infraestructura de producción (`context/codex-4-prod.md`: deploy prod, DLQ, alertas, limitador compartido, backups). Van en un plan aparte, porque master se despliega en otra cuenta de GCP.

## Decisiones del dueño (2026-10-15)

| # | Decisión |
|---|---|
| D1 | Alcance: los 28 hallazgos. |
| D2 | **Baja:** se cancelan los cargos **no vencidos** a la fecha de baja. Lo vencido o pagado se queda como adeudo real. El director ve el resumen antes de confirmar. |
| D3 | **Actividad a mitad de mes:** paga el mes completo, con vencimiento el día que se inscribe (la misma regla que la colegiatura). |
| D4 | **Cumpleaños:** todo alumno con inscripción activa en el ciclo, aunque esté "en trámite". |
| D5 | **Integración:** un PR por tarea → CI + GitGuardian verdes → squash a `dev` → limpieza de ramas. Autorización permanente para empujar dentro de este plan. |
| D6 | **Push real:** dejar todo listo (eas.json, projectId configurable, `PUSH_PROVIDER=expo` configurable, recibos de Expo, guía). El dueño conecta su cuenta de Expo/EAS cuando quiera. |
| D8 | **Día del cobro (16-oct):** finanzas deja el día UTC y toma "hoy" en America/Mexico_City (recargos, vencimientos, pronto pago, corridas, "Al {fecha}"). |
| D7 | **Datos del QA final:** los Excel de `importaciones/` (ignorada por git). Solo en la base LOCAL; nunca en reportes, capturas ni commits. |

## Ciclo por tarea (definición de terminado)

Cada tarea recorre este ciclo completo antes de empezar la siguiente:

1. **Rama** desde `dev` actualizado (`bugfix/…` o `feature/…`).
2. **Brief → Codex** (`codex-task impl`, TDD: primero un test que falle). Claude planea, revisa el diff y corre los tests (unitarios, integración con testcontainers y e2e que apliquen).
3. **Verify** (`superpowers:verification-before-completion`): evidencia de que los tests y el build pasan, nunca "debería funcionar".
4. **Security review** (`/security-review`) → corregir en la misma rama. Equivale a la compuerta de seguridad.
5. **Code review** (`/code-review`) → corregir en la misma rama. Equivale a la compuerta de tech-lead.
6. **QA** (`/qa` acotado a la tarea, con navegador visible y base local) → si sale un issue: se corrige en la misma rama y **se repiten los pasos 3–6** hasta que todo quede verde.
7. **PR → CI + GitGuardian verdes → squash a `dev` → `git-prune-merged --apply`** → siguiente tarea.

**Roles:** `backend-sr` (Go) · `frontend-sr` (web y móvil) · `db-sr` (migraciones y RLS) · `devops-sr` (EAS, Terraform y env) · `security` / `tech-lead` (compuertas) · `qa-sr` (QA) · `doc-writer` (guías y ADR). La implementación pasa por Codex. Las compuertas, las decisiones y los commits los hace Claude.

**Flags:** son correcciones dentro de módulos que ya existen, así que no se crean flags nuevos. Cada QA corre con los flags locales encendidos.

## Tareas (en orden)

### Ola 1 — Bajas (bloqueantes)

**T1 · La baja cancela los cargos no vencidos** — ISSUE-002 · High · backend-sr, db-sr
- Al retirar una inscripción (`POST /api/v1/enrollments/withdraw`), finance cancela los cargos de ese alumno y ciclo con `due_date` **posterior** a la fecha de baja y sin pagos (`cancelled`, con motivo "baja"). Los vencidos y los pagados no se tocan. Las inscripciones a actividades se cierran en esa fecha.
- Mecanismo idempotente y reintentable: un evento `StudentWithdrawn` hacia finance (outbox, o una saga en el gateway, como ya se hace en la eliminación de niveles). Con tests de reentrega.
- Nuevo `GET` de vista previa: qué cargos se cancelarían y cuánto queda de adeudo real.
- La corrida diaria no vuelve a crear cargos para una inscripción `withdrawn` (test).
- ADR con la regla D2.
- **Aceptación:** Regina dada de baja el 15-oct → los cargos de nov–jun quedan cancelados, el adeudo de octubre se queda, sale de "Por cobrar" de los meses futuros y una segunda entrega del evento no cambia nada.

**T2 · Pantalla de baja** — ISSUE-001 · High · frontend-sr
- Acción "Dar de baja" en la ficha del alumno (permiso `students:write`): pide motivo y fecha (hoy por defecto), muestra el resumen de la vista previa de T1 (cargos a cancelar y adeudo que queda) y una confirmación, porque no es reversible desde la UI.
- El filtro "Bajas" muestra a los alumnos dados de baja y su ficha queda en solo lectura.
- Se conecta o se borra el código huérfano (`_screens/students/Enrollment.tsx`, `EnrollmentRow.tsx`). Nada de código muerto.
- **Aceptación:** e2e: director da de baja → aparece en "Bajas" → su estado de cuenta ya no tiene cargos futuros.

### Ola 2 — App de padres

**T3 · App alineada con Expo SDK 56 y sin errores** — ISSUE-003, ISSUE-022 · High · frontend-sr (móvil)
- `expo install --fix` (10 paquetes) con pnpm; revertir el bump de la directiva `go` si se toca el workspace (trampa conocida).
- Cero `Uncaught Error` en todas las pestañas (Inicio, Asistencia, Notas, Cuenta, Avisos, Más) en el emulador Android; `bundling.test.ts` verde y el bundle real responde 200.
- Contenido: "Face ID" solo en iOS (en Android, "huella"/biometría o se oculta); saludo con el apellido de la familia; el nombre del encabezado no se corta.
- **Aceptación:** recorrido completo de la app en el emulador sin un solo error, con capturas.

**T4 · Push listo para conectar** — ISSUE-004 · High · devops-sr, backend-sr, doc-writer
- `eas.json` (perfiles development/preview/production) y `projectId` leído de la configuración (`app.config.ts` / env), sin secretos en git.
- Comms: `PUSH_PROVIDER=expo` configurable por entorno (en local se queda en `log`); consulta de recibos de Expo y baja de tokens muertos (`DeviceNotRegistered`).
- Guía `docs/push-setup.md`: `eas init`, credenciales APNs/FCM, variables y cómo probar con un teléfono real.
- **Aceptación:** tests del emisor con recibos simulados; en local el aviso llega a `target_count` correcto y un token muerto se borra.

### Ola 3 — Finanzas: consistencia

**T5 · Una sola cifra de saldo** — ISSUE-006 · Medium · backend-sr, frontend-sr
- Definir "saldo del año" como cargos efectivos + recargo acumulado a hoy − pagos. La misma cifra en caja ("Del año debe"), `/family/.../account.totalBalance`, estado de cuenta (web y app) y tarjetas de Finanzas.
- El monto que supone pronto pago se etiqueta como proyección ("Si paga a tiempo: $X"), no como un segundo "total".
- **Aceptación:** Valentina muestra el mismo saldo en las 4 superficies (test de contrato del gateway + test de componente).

**T5b · El día del cobro es el de Morelia** — decisión D8 · backend-sr
- finance toma "hoy" con un reloj de America/Mexico_City inyectado en la raíz de composición (como gateway y comms; tzdata embebido; falla al arrancar si la zona es inválida). Aplica a recargos diarios y mensuales, vencimientos, pronto pago, `ListOverdue`, corridas sin `asOfDate` y el `asOf` de cuenta y estado.
- Las fechas que ya están guardadas no cambian. Solo cambia qué día se considera "hoy".
- **Aceptación:** a las 19:00 de Morelia (01:00 UTC del día siguiente) no se acumula el recargo de mañana y la app dice "Al 15 oct 2026".

**T6 · Actividad a mitad de mes** — ISSUE-007 · Medium · backend-sr
- Regla D3: al inscribirse después del límite, se crea el cargo del periodo en curso con vencimiento el día de la inscripción. El texto del diálogo describe exactamente lo que pasa.
- **Aceptación:** fútbol el 15-oct → cargo de octubre por $600 que vence el 15-oct; la corrida no lo duplica.

**T7 · Captura de precios y formato de dinero** — ISSUE-010, 013, 016 · Medium · frontend-sr, backend-sr (comms)
- Tabulador: con el recargo diario apagado, la etiqueta dice "sin recargo".
- Al guardar el tabulador, el renglón de monto en borrador se confirma solo; lo mismo para "Agregar precio" en Actividades.
- Separador de miles en todo lado (tabla de colegiaturas, textos de notificaciones de pago), con un solo formateador compartido.
- **Aceptación:** guardar con el borrador lleno funciona a la primera; "$4,000.00" y "$7,000.00 MXN".

### Ola 4 — Académico y configuración

**T8 · Renombrar y borrar grados y grupos; ordenar niveles** — ISSUE-008, 021 · Medium · db-sr, backend-sr, frontend-sr
- API + UI para renombrar grados y grupos y borrarlos cuando no tengan alumnos, calificaciones ni asignaciones. Con dependencias, se explica qué lo impide.
- Niveles con orden (`sort_order`, migración versionada con RLS intacto) y la forma de reordenarlos.
- **Aceptación:** un grado mal capturado se corrige sin borrar el nivel; los niveles se ven en orden escolar.

**T9 · Alta de alumno con grupo y fecha de ingreso** — ISSUE-009 · Medium · backend-sr, frontend-sr
- El alta permite elegir grupo (por defecto, el primero con cupo) y fecha de ingreso (hoy por defecto; se acepta una fecha pasada dentro del ciclo). La facturación arranca en ese mes.
- Antes de escribir código, revisar si "inscripción con fecha propia" (M16) ya existe en otra pantalla, para reutilizarla.
- **Aceptación:** un alumno dado de alta con ingreso 24-ago recibe la colegiatura de septiembre; Valentina se puede dar de alta directo en 1º B.

**T10 · Calificaciones y periodos** — ISSUE-005, 011, 014, 019, 020 · Medium · backend-sr (grades), frontend-sr
- Nivel nuevo con "Sin decimales" por defecto.
- Al cerrar un periodo no se publica la boleta a un alumno sin calificaciones en ese periodo, y el diálogo de cierre lo dice.
- Los niveles sin calificación numérica (Kínder) no aparecen en Calificaciones › Periodos.
- Un solo nombre: los periodos de evaluación toman el nombre del trimestre ("1.er trimestre"), también en la boleta.
- Las fechas prellenadas cubren hasta el fin del ciclo.
- **Aceptación:** cerrar Primaria P1 no notifica a Emiliano; la boleta dice "1.er trimestre".

### Ola 5 — Avisos, cumpleaños y detalles de UX

**T11 · Cumpleaños y ruido de comms** — ISSUE-023, 027 · Low · backend-sr
- Regla D4: felicitar a todo alumno con inscripción activa. `RunBirthdays` devuelve conteos (felicitados, ya felicitados, sin tutor).
- `ChargeSettled` se ignora a nivel debug (sin WARN por cada pago).
- **Aceptación:** Camila (en trámite) recibe la felicitación; la respuesta trae los conteos.

**T12 · Formularios, errores y mensajes** — ISSUE-012, 015, 018, 025, 026 · Medium/Low · frontend-sr
- Mostrar el motivo que da el servidor (409 docente duplicado, asignación ocupada) y limpiar el error anterior al acertar.
- El botón principal nunca se deshabilita (activar cuenta, tallas, entrada de almacén, cobrar y entregar): siempre muestra estado ocupado o error al enviar.
- Lista de grupos en orden de nivel y grado en todas las pantallas.
- Egresos sin categorías: invitación a crear la primera, en vez de una lista vacía.
- Cobrar con el corte del día cerrado: avisar antes de confirmar.
- **Aceptación:** tests de componente de cada caso.

**T13 · Contenido y consola** — ISSUE-017, 024, 028 + consola · Low · frontend-sr
- Comprobante con grado y grupo ("1º Primaria · A").
- La pista de "Migrar alumnos" refleja el estado real.
- El selector de hora de Horarios en el rango escolar, con escritura directa.
- Sin errores de consola esperables: foto ausente (404) y sondeos de sesión antes del login (401) no se registran como errores.
- **Aceptación:** consola limpia en el recorrido de login → alumnos → caja.

### Cierre

**T14 · QA general final** · qa-sr
- Vaciar la base LOCAL (`scripts/reset-demo-db.sh`, verificando que apunta a localhost). Configurar desde cero por la UI.
- **Importar los 5 Excel de `importaciones/`** (Maternal, Preescolar, Primaria, Secundaria, Bachillerato) con "Migrar alumnos del sistema anterior" (`module_student_import`).
- Recorrer todos los módulos como director, docente y familia (app incluida), con navegador visible, consola y red. Comparar el dinero contra SQL independiente.
- Cada issue: corregir en su rama con el mismo ciclo. Reporte en `.gstack/qa-reports/` sin datos personales de los Excel.

## Estimación

| Tarea | CC (implementación + ciclo) | Notas |
|---|---|---|
| T1 | ~3 h | evento + vista previa + integración |
| T2 | ~1.5 h | |
| T3 | ~2 h | riesgo: el bump puede romper otras cosas de RN |
| T4 | ~2 h | |
| T5 | ~2 h | toca contrato del gateway |
| T6 | ~1 h | |
| T7 | ~1.5 h | |
| T8 | ~2.5 h | migración |
| T9 | ~2 h | |
| T10 | ~2 h | |
| T11 | ~1 h | |
| T12 | ~2 h | |
| T13 | ~1.5 h | |
| T14 | ~3 h + fixes | |
| **Total** | **~27 h de CC** | humano equivalente: ~3–4 semanas |

## Riesgos

- **Minutos de Actions:** 13 PRs × (CI de PR ~24 + push ~15 + deploy ~6) ≈ **585 min**. La cuota ya se agotó una vez (sep). Si se acaba a mitad del plan, el CI se detiene y la tarea queda sin fusionar hasta que se reponga.
- **`dev` es la demo en vivo:** cada merge despliega a dev. Solo se fusiona con el ciclo completo en verde.
- **T3 (bump de React Native):** puede romper cosas de la app que Jest no ve. Se verifica con el bundle real y con el emulador, no solo con tests.
- **Datos reales en los Excel:** solo en la base local. Ningún reporte ni captura muestra nombres, CURPs ni montos de esos archivos.
