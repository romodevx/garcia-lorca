-- iam (M01) — password policy + owner/director seed.
--
-- Adds must_change_password (forces a rotation at first login) and seeds the
-- single owner account for the default tenant. Idempotent: safe to re-run.
--
-- RLS: the iam tables are FORCE ROW LEVEL SECURITY, so policies apply even to
-- the table owner. The SET LOCAL below binds app.current_tenant so the seed
-- INSERTs satisfy the tenant_isolation WITH CHECK under a non-superuser owner
-- (e.g. Cloud SQL, which grants no BYPASSRLS). Harmless where the migrator is a
-- superuser (superusers ignore RLS regardless).

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

-- --- password policy ------------------------------------------------------

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS must_change_password boolean NOT NULL DEFAULT false;

-- --- owner seed (default tenant, single-tenant M01) -----------------------
-- Fixed UUIDs so re-running is a no-op and later migrations can reference them.

INSERT INTO roles (id, tenant_id, name, permissions)
VALUES (
    '00000000-0000-0000-0000-0000000000a1',
    '00000000-0000-0000-0000-000000000001',
    'owner',
    -- Spine permissions the owner needs today; later phases append their own.
    ARRAY['students:create', 'students:read', 'students:list', 'students:update']
)
ON CONFLICT (tenant_id, name) DO NOTHING;

INSERT INTO users (id, tenant_id, email, display_name, password_hash, status, must_change_password)
VALUES (
    '00000000-0000-0000-0000-0000000000d1',
    '00000000-0000-0000-0000-000000000001',
    'director@garcialorca.edu.mx',
    'Director',
    '$2a$10$4DzqOAZ/lTjnbD76l0c/TePHj0o4C87GJLInXxxpUfiHdllRIAsk6',   -- bcrypt of GarciaLorca2026! (from Step 1)
    'active',
    true
)
ON CONFLICT (tenant_id, email) DO NOTHING;

INSERT INTO user_roles (user_id, role_id, tenant_id)
VALUES (
    '00000000-0000-0000-0000-0000000000d1',
    '00000000-0000-0000-0000-0000000000a1',
    '00000000-0000-0000-0000-000000000001'
)
ON CONFLICT (user_id, role_id) DO NOTHING;

COMMIT;
