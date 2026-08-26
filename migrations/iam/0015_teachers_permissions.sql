-- iam (M14 support) — the staff roster gets permissions of its own.
--
-- Docentes used to be a panel inside Configuración, so reading it meant holding
-- config:read — the whole academic catalogue. Now that it is a module in its own
-- right it gates on teachers:read / teachers:write, which is the point: a school
-- can hand the secretaria the staff roster without handing her the cycles,
-- levels, grades and groups as well.
--
-- The owner holds both so it can delegate them. Neither is owner-only: a teacher
-- roster carries staff names and assignments, not student data, so it does not
-- meet the bar that put finance:read and attendance:read behind the owner guard.
--
-- Plain SQL only (no psql variables): this file must apply BOTH via the psql
-- runner AND via the Go integration test, which execs the SQL through GORM.
--
-- RLS: the roles table is FORCE ROW LEVEL SECURITY, so the tenant_isolation
-- policy applies even to the table owner. SET LOCAL binds app.current_tenant so
-- the UPDATE's USING/WITH CHECK is satisfied under a non-superuser owner (e.g.
-- Cloud SQL, which grants no BYPASSRLS).

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

UPDATE roles
   SET permissions = permissions || ARRAY['teachers:read', 'teachers:write']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND name = 'owner'
   AND NOT (permissions @> ARRAY['teachers:read', 'teachers:write']);

COMMIT;
