-- Owner/director bootstrap. Seeds the single owner USER for the default tenant,
-- hashing the temporary password from the psql :owner_temp_password variable via
-- pgcrypto (bcrypt). The runner sources that variable from $OWNER_TEMP_PASSWORD
-- (never committed). Idempotent. In shared envs the password comes from the
-- secret manager, not a local .env.
BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

INSERT INTO users (id, tenant_id, email, display_name, password_hash, status, must_change_password)
VALUES (
    '00000000-0000-0000-0000-0000000000d1',
    '00000000-0000-0000-0000-000000000001',
    'director@garcialorca.edu.mx',
    'Director',
    crypt(:'owner_temp_password', gen_salt('bf')),
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
