# PLAN — Familias (oct 2026)

- **Origen:** pedido del dueño, 2026-10-23: papá y mamá (o tutor) entran a la app con
  su propia cuenta y ven a todos sus hijos. La oficina necesita **identificar familias**,
  filtrar alumnos por familia y mandar avisos por familia.
- **Estado:** APROBADO por el dueño el 2026-10-23 (las 4 decisiones de abajo).
- **ADR:** `adr/0041-families.md`.

## Lo que ya existía

- `guardian_students` (students, RLS) son los contactos **por alumno**: un contacto con
  `guardian_user_id` puede abrir la app. La invitación es idempotente por correo, así que
  una mamá con 3 hijos ya tiene una sola cuenta, y la app ya cambia entre hijos.
- Lo que faltaba: la familia no existía como cosa. La oficina capturaba y daba acceso
  **una vez por hijo**, nada limitaba las cuentas por familia, no había filtro, y un aviso
  a toda la escuela sonaba **una vez por hijo** en el celular del papá.

## Decisiones del dueño (2026-10-23)

| # | Decisión |
|---|---|
| D1 | **Pestaña "Familias" dentro de Alumnos.** Sin módulo nuevo en el menú. Columna y filtro en el directorio, tarjeta "Familia" en la ficha. |
| D2 | **Sugerencias a confirmar.** Mismo apellido paterno + materno (sin acentos ni mayúsculas); compartir correo, teléfono o cuenta de un contacto sube la confianza y también une medios hermanos. La oficina acepta, ajusta o descarta; lo descartado no vuelve. |
| D3 | **Máximo 2 cuentas de app por familia, estricto** (servidor). Contactos sin app, ilimitados. |
| D4 | **Un solo push por papá por aviso**, aunque tenga varios hijos. El aviso sigue en el feed de cada hijo. |

## Modelo (híbrido, ADR 0041)

- `families` (id, tenant, nombre) + `students.family_id` (un alumno, una familia).
- `guardian_students` **sigue siendo la autorización** de la app de padres: no se toca el
  camino de lectura familiar. La familia es la capa que **escribe** esos vínculos:
  dar acceso a un adulto en la familia lo liga a cada hijo (copiando su contacto), y un
  hijo que entra a una familia recibe las cuentas que la familia ya tiene.
- El tope de 2 cuentas se cuenta como cuentas distintas ligadas a los hijos de la familia,
  con la fila de la familia bloqueada (`FOR UPDATE`) en la misma transacción.
- Descartes de sugerencias: firma = hash de los ids ordenados del grupo. Si entra un
  hermano nuevo, la firma cambia y la sugerencia vuelve (con él).
- Permisos: leer `students:read`; armar/editar familias `students:update`; dar o quitar
  acceso `guardians:write` (owner, ADR 0012). Flag `module_families` (gateway), exige
  `module_students`.

## Ciclo por tarea

Rama desde `dev` → TDD (Codex sin cuota hasta 14-nov: Claude + agentes) → verificación →
revisión (tech-lead; + security cuando toca authz) → QA visible → PR verde → squash a
`dev` → `git-prune-merged --apply`. PR por tarea sin preguntar (modo de trabajo).

## Tareas

| # | Tarea | Rol | Estado |
|---|---|---|---|
| T1 | Contrato (`students.proto`), migración `students/0023` (families, family_id, descartes, RLS + aislamiento), dominio + repo + casos de uso: crear, renombrar, agregar/quitar hijos, disolver, listar, **sugerencias** y descartes | backend-sr, db-sr | ⏳ |
| T2 | Acceso por familia: `GrantFamilyAccess` / `RevokeFamilyAccess`, sincronía al entrar un hijo, tope de 2 también en `GrantContactAccess`/`LinkGuardian` | backend-sr, security | ⏳ |
| T3 | Gateway: rutas `/api/v1/families/*` detrás de `module_families`, invitación de un adulto a la familia (iam una vez + students), `familyId` en el JSON del alumno y filtro | backend-sr, security | ⏳ |
| T4 | Web: pestaña Familias (sugerencias, lista, detalle con hijos y hasta 2 adultos), columna + filtro en el directorio, tarjeta en la ficha | frontend-sr | ⏳ |
| T5 | Avisos: destinatario "Familias" + un solo push por papá por aviso (comms) | backend-sr, frontend-sr | ⏳ |
| T6 | QA visible completo, compuertas, FLAGS/MODULES/docs, flag en dev | qa-sr, tech-lead, security, doc-writer | ⏳ |
