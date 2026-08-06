-- iam (Phase 6 support) — grant the user/role admin permissions to the owner role.
--
-- The Phase 6 admin surface is gated at the gateway by users:read / users:write
-- (user CRUD + status + role assignment) and roles:read / roles:write (role
-- CRUD + permission editing). None are on the seeded owner role yet; this
-- migration appends all four idempotently so the owner can manage users and
-- roles. A re-run is a no-op once they are present.
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
   SET permissions = permissions || ARRAY['users:read', 'users:write', 'roles:read', 'roles:write']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND name = 'owner'
   AND NOT (permissions @> ARRAY['users:read', 'users:write', 'roles:read', 'roles:write']);

COMMIT;
