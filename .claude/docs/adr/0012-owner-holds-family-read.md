# ADR 0012 — El `owner` sostiene `family:read`, y el guard no se toca

- **Estado:** aceptado (2026-09-12)
- **Contexto del módulo:** iam · acceso de las familias a la app

## Contexto

Ninguna familia podía recibir la app. No por un error, sino por el encuentro de dos
decisiones que por separado son correctas.

La primera es el **guard anti-escalada del borde**
(`services/gateway/internal/handler/admin.go`): quien asigna un rol solo puede asignar
roles cuyos permisos **ya tiene**. Existe porque sin él un administrador delegado con
`users:write` podría crear (o invitar a) una cuenta con poderes de dueño, abrir la liga
él mismo y llegar como dueño. Está aplicado en `CreateUser`, `InviteUser` y
`ResendInvitation`, los tres caminos por los que se alcanza una cuenta.

La segunda es el **rol `guardian`**, creado en `migrations/iam/0010_guardians.sql` con
exactamente un permiso: `family:read`. Su comentario deja escrito por qué no lleva
ninguna de las lecturas del dueño — `finance:read`, `grades:read`, `attendance:read`,
`comms:read` son lecturas de todo el colegio, y dárselas a un tutor sería justo el error
que el camino auto-acotado de `/api/v1/family/*` (ADR 0004) evita.

Juntas producen un callejón: **nadie en el sistema tiene `family:read`**, así que nadie
puede asignar el rol `guardian`. `POST /api/v1/users` y `POST /api/v1/users/invite` con
ese rol responden **403 a cualquier llamador, incluido el dueño**. La única forma de
crear un tutor era escribir SQL contra la base.

## La opción que se descartó

Lo primero que uno piensa es eximir a `family:read` del guard: es un permiso
auto-acotado, no concede una lectura de todo el colegio, luego no puede escalar.

**Es falso, y el guard tiene razón.** `family:read` no escala *solo*. Escala acompañado:
un administrador delegado que tuviera `users:write` **y** `guardians:write` podría
invitar una cuenta de tutor a un correo suyo, vincularse cualquier alumno del colegio y
leerle calificaciones, asistencia y estado de cuenta por `/api/v1/family/*` — lecturas
que su propio rol no le da. Una exención general abre exactamente esa puerta, y la abre
en el lugar donde nadie volvería a mirar.

## Decisión

**Se agrega `family:read` al rol `owner`** (`migrations/iam/0017_owner_family_read.sql`).
El guard queda intacto.

Es seguro por una razón concreta y verificable, no por conveniencia: para el dueño,
`family:read` es **estrictamente un subconjunto de lo que ya lee**. Cada endpoint que ese
permiso abre tiene su equivalente de todo el colegio ya en manos del dueño:

| `/api/v1/family/…` | Lo que el dueño ya tiene |
|---|---|
| `students/{id}/profile` | `students:read` |
| `students/{id}/account` | `finance:read` |
| `students/{id}/grades` | `grades:read` |
| `students/{id}/attendance` | `attendance:read` |
| `notifications` | `comms:read` |

El dueño no gana **ninguna** capacidad. Gana la condición que el guard le pide para poder
delegar lo que ya puede hacer. Y como el permiso no se agrega a `admin` ni a ningún otro
rol, el administrador delegado del párrafo anterior sigue bloqueado: el guard sigue
haciendo su trabajo, contra el mismo atacante, por la misma razón.

Como efecto secundario el dueño puede llamar `/api/v1/family/*` con su propia sesión. No
ve nada: esas rutas resuelven los hijos desde los vínculos del principal, y el dueño no
tiene vínculos. Una lista vacía, que es la respuesta correcta.

## Consecuencias

- El dueño puede invitar tutores por el API. Es lo que habilita
  `POST /api/v1/students/{studentId}/guardians/{contactId}/invite` y su botón en el panel.
- **Quien quiera dar de alta tutores necesita ser dueño.** Delegar esa tarea a un rol
  menor no es cuestión de agregarle `guardians:write`: también necesitaría `family:read`,
  y dárselo reabre la escalada descrita arriba. Si algún día el colegio quiere una
  secretaria que invite familias sin ver las finanzas de todos, la respuesta **no** es
  aflojar el guard, sino que el vínculo tutor→alumno deje de ser suficiente para leer:
  otro ADR, otro modelo.
- La tabla de arriba es una invariante. Si alguna vez `/api/v1/family/*` sirve un dato
  que el dueño **no** pueda leer por su cuenta, esta decisión deja de ser segura y hay
  que revisitarla.

## Alternativas consideradas

- **Eximir `family:read` del guard.** Descartada arriba: abre la escalada
  `users:write` + `guardians:write`.
- **Un endpoint dedicado que esquive el guard.** Mueve el problema de lugar sin
  resolverlo, y deja un segundo camino hacia la creación de cuentas — precisamente lo
  que el guard existe para que no haya.
- **Sembrar las cuentas de tutor por migración.** No es una decisión de arquitectura, es
  renunciar a que el colegio opere su propio sistema.
