-- iam — el dueño sostiene family:read, para poder delegarlo.
--
-- POR QUÉ. El guard anti-escalada del borde (gateway/internal/handler/admin.go)
-- solo deja asignar roles cuyos permisos el llamador YA tiene. El rol `guardian`
-- lleva exactamente `family:read` (migración 0010) y nadie lo tenía, así que
-- nadie podía asignar el rol: `POST /api/v1/users` y `/users/invite` respondían
-- 403 a cualquiera, dueño incluido. Ninguna familia podía recibir la app sin que
-- alguien escribiera SQL a mano.
--
-- POR QUÉ ES SEGURO. Para el dueño, `family:read` es estrictamente un
-- subconjunto de lo que ya lee: profile←students:read, account←finance:read,
-- grades←grades:read, attendance←attendance:read, notifications←comms:read. No
-- gana ninguna capacidad; gana la condición que el guard le pide para delegar lo
-- que ya puede hacer. Llamar /api/v1/family/* con su sesión le devuelve una
-- lista vacía, porque esas rutas resuelven los hijos desde los vínculos del
-- principal y el dueño no tiene vínculos.
--
-- POR QUÉ NO SE TOCÓ EL GUARD. Eximir `family:read` en general abriría una
-- escalada real: un administrador delegado con `users:write` Y `guardians:write`
-- invitaría una cuenta de tutor a un correo suyo, se vincularía cualquier alumno
-- y le leería calificaciones y finanzas. El permiso NO se agrega a `admin` ni a
-- ningún otro rol, justamente para que ese camino siga cerrado. Ver ADR 0012.
--
-- Idempotente: una segunda corrida no agrega el permiso dos veces.
--
-- SQL plano (sin variables de psql): este archivo se aplica tanto por el runner
-- de psql como por el test de integración en Go, que lo ejecuta vía GORM.
--
-- RLS: roles es FORCE ROW LEVEL SECURITY, así que la policy aplica incluso al
-- dueño de la tabla. SET LOCAL enlaza app.current_tenant para que el USING del
-- UPDATE se satisfaga bajo un owner sin BYPASSRLS (p. ej. Cloud SQL).

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

UPDATE roles
   SET permissions = permissions || ARRAY['family:read']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND name = 'owner'
   AND NOT (permissions @> ARRAY['family:read']);

COMMIT;
