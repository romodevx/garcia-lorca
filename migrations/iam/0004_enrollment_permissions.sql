-- iam (Phase 4 support) — grant the enrollment permission to the owner role.
--
-- The enrollment routes are gated at the gateway by students:read (list/get)
-- and students:enroll (enroll/transfer/withdraw). students:read is already on
-- the seeded owner role (migrations/iam/0002); this migration appends
-- students:enroll idempotently so the owner can drive enrollment. A re-run is a
-- no-op once it is present.
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
   SET permissions = permissions || ARRAY['students:enroll']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND name = 'owner'
   AND NOT (permissions @> ARRAY['students:enroll']);

COMMIT;
