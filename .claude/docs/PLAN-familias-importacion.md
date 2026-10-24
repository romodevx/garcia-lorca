# PLAN — Familias desde la importación (oct 2026)

- **Origen:** pedido del dueño, 2026-10-23: "desde la importación, que el sistema vaya
  armando las familias".
- **Decisión del dueño (2026-10-23): "Armarlas solas".** Al confirmar una importación, las
  familias muy probables se crean sin preguntar, y el resumen las muestra con **Deshacer**.
  Las dudosas quedan como sugerencias en la pestaña Familias. Esta decisión cambia la D2 de
  `PLAN-familias.md` solo para la importación; la pestaña sigue pidiendo confirmación.
- **ADR:** enmienda a `adr/0041-families.md` (sección "Familias desde la importación").

## Lo que encontramos en el código

1. **El Excel no trae a los papás.** Por alumno trae nombre, correo, teléfono de casa y
   celular (`services/gateway/internal/legacyimport/parse.go`). La importación guarda esos
   datos **en la ficha del alumno** (`students.email`, `students.phones`), no como
   contactos (`guardian_students`).
2. **Las sugerencias solo miran `guardian_students`.** A un alumno importado solo lo emparejan
   los apellidos, que es confianza media. Sin el cambio de abajo, la importación no armaría
   ninguna familia sola.
3. **Hay un Excel por nivel.** Un hermano en Primaria y otro en Secundaria vienen en archivos
   distintos, y las sugerencias excluyen a quien ya tiene familia. Si la primera importación
   arma GARCÍA TORRES, al importar el siguiente nivel su hermano tiene que **sumarse a esa
   familia**, no quedarse fuera.
4. Importar ya es solo del owner (`students:import`), y el owner tiene `guardians:write`. No
   hay un permiso nuevo.

## Reglas

- **Muy probable (se arma sola)** = mismos dos apellidos (`SurnameKey`) **y** el mismo correo
  o teléfono, comparando la ficha del alumno y sus contactos. Se unen solo los pares que
  cumplen las **dos** condiciones, así un primo que comparte apellidos pero no teléfono no
  entra. Siguen aplicando los topes de contacto: un valor que comparten más de 6 alumnos (el
  teléfono del colegio, por ejemplo) no une a nadie. Una familia tiene de 2 a 20 alumnos.
  Union-find, O((n+c)·α(n)).
- Solo cuentan los grupos con **al menos un alumno de este lote** (`import_batch_id`).
- El grupo **no tiene familia** → se crea la familia, salvo que alguno ya tenga cuenta de la
  app (H1: armarla le daría acceso a otro) o que el grupo esté descartado. En esos casos
  queda como sugerencia.
- El grupo toca **exactamente 1 familia, y esa familia tiene 0 cuentas** → los demás se suman
  a ella.
- Toca 2 familias o más, o una familia con cuentas → no se hace nada; la oficina decide.
- **Deshacer** quita a los alumnos que la importación puso en cada familia, y si una familia
  queda vacía, se disuelve (un alumno que la oficina agregó se queda en ella). Se registra en
  `family_import_links` (RLS). Deshacer dos veces da el mismo resultado. Si la oficina ya
  movió a un alumno, no se le toca. Una familia que **ya tiene acceso a la app** (alguna
  cuenta en cualquiera de sus alumnos) no se toca: sacar alumnos les dejaría ese acceso. La
  oficina quita el acceso primero y vuelve a deshacer; el resumen dice cuántas quedaron así.
- Las sugerencias de la pestaña Familias también leen la ficha del alumno (correo y
  teléfonos), así que lo que queda por revisar sale con la confianza correcta.
- Si armar las familias falla, la importación **no falla**: el resumen dice "No pudimos armar
  las familias" y todo queda en sugerencias.
- Solo con `module_families` encendido. Con el flag apagado, la importación funciona igual
  que hoy.

## Tareas

| # | Tarea | Rol | Estado |
|---|---|---|---|
| I1 | students: contactos de la ficha en las sugerencias; dominio `PlanImportFamilies`; migración `0024_family_import_links` (RLS + aislamiento); RPCs `BuildFamiliesFromImport` / `UndoFamiliesFromImport` | backend-sr, db-sr | ⏳ |
| I2 | gateway: el commit de la importación arma las familias (si el flag está encendido, best-effort); campo `families` en la respuesta; `POST /api/v1/imports/students/{batchId}/families/undo`; OpenAPI | backend-sr, security | ⏳ |
| I3 | web: bloque Familias en el resumen de la importación ("Armamos N familias" + Deshacer, "M por revisar →") | frontend-sr | ⏳ |
| I4 | QA visible (importar 2 Excel de niveles distintos con hermanos), compuertas, ADR, docs | qa-sr, tech-lead, security | ⏳ |

Ciclo por tarea igual que en `PLAN-familias.md`: rama → TDD → revisión → PR → squash a `dev`.
