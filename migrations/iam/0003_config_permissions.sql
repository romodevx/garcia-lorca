-- iam (M04 support) — grant the academic-catalog permissions to the owner role.
--
-- The config module (academic catalog) is gated at the gateway by config:read
-- (list) and config:write (create). The seeded owner role (migrations/iam/0002)
-- must carry both so the owner can drive the catalog. This migration appends
-- them idempotently: a re-run is a no-op once they are present.
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
   SET permissions = permissions || ARRAY['config:read', 'config:write']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND name = 'owner'
   AND NOT (permissions @> ARRAY['config:read', 'config:write']);

COMMIT;
