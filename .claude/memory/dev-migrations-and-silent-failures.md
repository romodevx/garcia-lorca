---
name: dev-migrations-and-silent-failures
description: El deploy aplica migraciones desde el 2026-09-07, y por qué los fallos de dev se veían como errores de datos del usuario.
metadata:
  type: project
---

**El deploy de dev aplica las migraciones** (paso "Apply the migrations", entre
construir y desplegar). Si fallan, no despliega: la demo se queda en la versión
anterior, que funciona.

Antes no las aplicaba nadie. En local es imposible que derive —compose obliga a
cada servicio a esperar al migrador— así que el hueco existía SÓLO en el entorno
que mira el colegio. Resultado: 11 migraciones de retraso durante nueve días, y
la forma de enterarse fue que no se pudiera guardar una colegiatura.

**Cómo se diagnostica una deriva de esquema** (los síntomas parecen de dominio,
no de base):
1. `gcloud logging read` filtrando `httpRequest.status >= 400` y jsonPayload con
   "error" — ahí sale el `SQLSTATE`.
2. Diferencia `schema_migrations` contra los archivos del repo. **La columna se
   llama `filename`, no `version`.**
3. Toda consulta a la base de dev necesita `SET app.current_tenant` primero (RLS),
   y `psql -c` con varias sentencias corre TODO en una transacción: un fallo al
   final revierte lo de antes.

**Dos permisos nuevos para la cuenta del build, acotados a propósito:**
`cloudsql.client` (sólo abrir la conexión) y lectura de UN secreto, el del
migrador. `secretAccessor` a nivel de proyecto le habría dado la llave de firma
de IAM y las contraseñas de todo.

## Los fallos no se cuentan como errores del usuario

Dos veces el mismo día una falla nuestra salió como una afirmación sobre lo que
el usuario hizo:

- Sesión caducada → *"No se pudo crear el grado. Revisa que no exista ya con ese
  orden"*. `authFetch` devolvía el 401 en silencio y cada panel lo traducía a un
  choque de datos. Ahora un refresh rechazado se anuncia y la app dice "Tu sesión
  terminó".
- `/me` fallando → el dashboard mostraba el **esqueleto de carga para siempre**,
  que se lee como "no hay módulos".

Regla para este código: un fallo nunca se reporta como algo que hizo la persona,
ni como un estado de carga. Ver [[identity-platform-cutover-2026-09]].
