-- iam (Phase 7 support) — grant the finance/adeudos permissions to the owner role.
--
-- The finance routes are gated at the gateway by finance:read (concept catalog,
-- charge list, account statement, overdue report) and finance:write (concept
-- create/update, charge create/generate, discount, cancel, payment). Neither is
-- on the seeded owner role yet; this migration appends both idempotently so the
-- owner can drive the ledger. A re-run is a no-op once they are present.
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
   SET permissions = permissions || ARRAY['finance:read', 'finance:write']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND name = 'owner'
   AND NOT (permissions @> ARRAY['finance:read', 'finance:write']);

COMMIT;
