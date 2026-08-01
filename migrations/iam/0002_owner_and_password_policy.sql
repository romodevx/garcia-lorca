-- iam (M01) — password policy + owner role (schema + role only).
--
-- Adds must_change_password (forces a rotation at first login) and seeds the
-- owner ROLE for the default tenant. It contains NO credentials: the owner USER
-- is seeded separately from the environment (docker/seed-owner.sql, driven by
-- OWNER_TEMP_PASSWORD) so no working password ever lands in git. Idempotent.
--
-- Plain SQL only (no psql variables): this file must apply BOTH via the psql
-- runner AND via the Go integration test, which execs the SQL through GORM.
--
-- RLS: the iam tables are FORCE ROW LEVEL SECURITY, so policies apply even to
-- the table owner. The SET LOCAL below binds app.current_tenant so the role
-- INSERT satisfies the tenant_isolation WITH CHECK under a non-superuser owner
-- (e.g. Cloud SQL, which grants no BYPASSRLS). Harmless where the migrator is a
-- superuser (superusers ignore RLS regardless).

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

-- --- password policy ------------------------------------------------------

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS must_change_password boolean NOT NULL DEFAULT false;

-- --- owner role (default tenant, single-tenant M01) -----------------------
-- Fixed UUID so re-running is a no-op and the env-driven owner user seed
-- (docker/seed-owner.sql) can reference it.

INSERT INTO roles (id, tenant_id, name, permissions)
VALUES (
    '00000000-0000-0000-0000-0000000000a1',
    '00000000-0000-0000-0000-000000000001',
    'owner',
    -- Spine permissions the owner needs today; later phases append their own.
    ARRAY['students:create', 'students:read', 'students:list', 'students:update']
)
ON CONFLICT (tenant_id, name) DO NOTHING;

COMMIT;
