# PLAN — Avisos de importación y borradores sin CURP

- **Origen:** validación de `importaciones/` contra la base local (21-oct-2026). 567 de 570 filas entraron bien, pero el importador **descarta o cambia datos en silencio**: solo avisa en la pantalla de importación, el aviso no se guarda y el valor original del Excel se pierde.
- **Estado:** BORRADOR — pendiente de aprobación del dueño.
- **Ya resuelto aparte:** siglo de la fecha de nacimiento (PR #171) y Go 1.26.10 (PR #172).

## Decisiones del dueño (21-oct-2026)

| # | Decisión |
|---|---|
| D1 | Ningún dato se pierde ni cambia en silencio. Lo que el sistema no puede resolver queda como **aviso en el alumno**: ícono ⚠ en la lista y alerta en el campo de su ficha. |
| D2 | En la **vista previa de la importación**, antes de «Importar», cada fila con aviso se puede **corregir ahí mismo**, **omitir**, o (si no trae CURP) **importar como borrador**. *(Corregir cualquier aviso, no solo la CURP: recomendación aceptada por omisión; el dueño puede limitarlo a CURP.)* |
| D3 | **Borrador · falta CURP**: sale en la lista con marca y filtro propios; no se puede inscribir, cambiar de grupo, cobrar, ligar a una familia ni invitar a la app. Al capturar una CURP válida y no repetida deja de ser borrador. |
| D4 | «Está bien así» (cerrar un aviso sin cambiar el dato) lo usa quien tenga `students:update`. *(Recomendación por omisión; el dueño puede restringirlo al director.)* |
| D5 | Las filas «Baja» se siguen omitiendo, como hoy. |

## Diseño

### Datos (migración `students/0022`)
- `student_import_issues` — `tenant_id`, `student_id`, `field` (birth_date · phone_home · phone_mobile · email · postal_code · state · bank_reference · surnames · first_name · address · curp), `code` (los códigos que ya existen: `birth_date_from_curp`, `phone_invalid`…), `original_value` (lo que decía el Excel), `stored_value`, `import_batch_id`, `created_at`, `resolved_at`, `resolved_by`, `resolution` (`corrected` · `accepted`). **ENABLE + FORCE RLS** por `app.current_tenant`; pruebas de aislamiento A/B.
- Borrador: `admission_status` suma `draft`; `curp` pasa a NULL **solo** con `CHECK (curp IS NOT NULL OR admission_status = 'draft')`. El `UNIQUE (tenant_id, curp)` no cambia (los NULL no chocan). El grupo destino del borrador se guarda en su aviso `curp` para ofrecer «Inscribir en 2° A» al completarlo.

### Contrato primero (proto + `docs/api/openapi.yaml`)
- Commit de importación acepta `rows: [{row, action: import|omit|draft, overrides: {curp?, birthDate?, phones?, email?, postalCode?, bankReference?, …}}]`. El gateway **vuelve a leer el archivo** (como hoy), aplica las correcciones y **las valida en el servidor**; el `planHash` sigue cubriendo archivo + niveles + cupos.
- `Student` suma `openIssues` (conteo) y `draft`.
- `GET /api/v1/students/{id}/import-issues` y `POST /api/v1/students/{id}/import-issues/{issueId}/accept`.
- Guardar la ficha cierra como `corrected` los avisos de los campos que cambiaron.

### Bloqueos del borrador (servidor, no solo la pantalla)
Inscribir (`enrollment.go:96`), cambiar de grupo (`:252`), ligar familia (`guardian.go:76`), invitar a la app (iam `admin.go:184`). Finanzas no lo ve: sin inscripción no hay `StudentEnrolled`.

### Pantallas
- **Vista previa** (`ImportTable`): filas con aviso muestran el motivo y tres acciones (Corregir · Omitir · Borrador si falta CURP). Corregir abre la fila en edición en el lugar (≤4 campos, regla de formularios); se valida al salir del campo.
- **Lista de alumnos**: ⚠ con número de avisos; filtros «Con avisos N» y «Borradores N»; el borrador lleva etiqueta gris «Borrador · falta CURP».
- **Ficha** (`StudentEditScreen`): variante **aviso** de `Field` (ámbar, no bloquea, no marca `aria-invalid`): «En el Excel decía X; se guardó Y». Botón «Está bien así».
- Flag nuevo **`module_import_review`** (OFF por omisión; registrado en FLAGS.md y su migración). Sin el flag todo se comporta como hoy.

## Entrega (un PR por paso, CI + GitGuardian verdes → squash a dev)

| PR | Contenido | Tests primero |
|---|---|---|
| 1 | Contrato + migración 0022 + students: avisos guardados, borrador, bloqueos, aceptar/corregir | dominio (borrador, transiciones), integración Postgres + RLS A/B, contrato |
| 2 | Gateway: correcciones por fila en el commit, valores originales, borradores, rutas nuevas tras flag; iam bloquea invitación a borrador | handler (flag ON/OFF, validación de correcciones), legacyimport |
| 3 | Web: vista previa editable, ⚠ y filtros en la lista, alertas en la ficha | Vitest/RTL por componente, flag ON/OFF |
| 4 | QA visible: vaciar la base LOCAL, reimportar los 5 Excel desde el navegador corrigiendo en la vista previa, recorrer avisos y borradores, revalidar con el script campo por campo | — |

Compuertas tech-lead + seguridad antes de encender el flag en dev.

**Estimación:** ~2 días de trabajo (PR 1 y 2 el grueso).
