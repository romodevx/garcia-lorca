-- iam (teacher-scoped attendance support) — grant the attendance_own permissions
-- to the owner role.
--
-- attendance_own:read / attendance_own:write gate the self-scoped
-- /api/v1/teaching/attendance/* surface a maestro uses to read the roster and
-- capture attendance for the groups·subjects they teach (ownership scoped by the
-- principal→teacher link, not by these permissions). Unlike attendance:read /
-- attendance:write (which are owner-only and gate the admin-wide pass list), these
-- are GRANTABLE (deliberately NOT owner-only), but the owner must HOLD them to
-- delegate them to a Maestro role — the admin subset-guard lets an admin grant
-- only permissions it already holds. This migration appends both idempotently so
-- the owner can mint teacher roles. A re-run is a no-op once they are present.
--
-- Plain SQL only (no psql variables): this file must apply BOTH via the psql
-- runner AND via the Go integration test, which execs the SQL through GORM.
--
-- RLS: the roles table is FORCE ROW LEVEL SECURITY, so the tenant_isolation
-- policy applies even to the table owner. SET LOCAL binds app.current_tenant so
-- the UPDATE's USING/WITH CHECK is satisfied under a non-superuser owner (e.g.
-- Cloud SQL, which grants no BYPASSRLS). Harmless where the migrator is a
-- superuser (superusers ignore RLS regardless).

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

UPDATE roles
   SET permissions = permissions || ARRAY['attendance_own:read', 'attendance_own:write']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND name = 'owner'
   AND NOT (permissions @> ARRAY['attendance_own:read', 'attendance_own:write']);

COMMIT;
