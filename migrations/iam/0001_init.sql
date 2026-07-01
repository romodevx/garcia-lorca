-- iam (M01) initial schema: users, roles, user_roles.
--
-- RLS is tenant-scoped (see ADR 0001): policies filter on app.current_tenant,
-- ENABLE + FORCE so the policy applies even to the table owner. The application
-- role is NOT the owner and is NOT BYPASSRLS (deploy/terraform/db/setup_app_role.sql).
--
-- Run by a privileged migration role. gen_random_uuid() is in core since PG13.

BEGIN;

-- --- tables ---------------------------------------------------------------

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL,
    email         text NOT NULL,
    display_name  text NOT NULL DEFAULT '',
    password_hash text NOT NULL,
    status        text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    -- email is unique per tenant (domain invariant).
    UNIQUE (tenant_id, email)
);

CREATE TABLE roles (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL,
    name        text NOT NULL,
    -- "resource:action" permission strings.
    permissions text[] NOT NULL DEFAULT '{}',
    UNIQUE (tenant_id, name)
);

CREATE TABLE user_roles (
    user_id   uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- ON DELETE RESTRICT: a role cannot be deleted while assigned (domain invariant).
    role_id   uuid NOT NULL REFERENCES roles (id) ON DELETE RESTRICT,
    tenant_id uuid NOT NULL,
    PRIMARY KEY (user_id, role_id)
);

CREATE INDEX idx_users_tenant ON users (tenant_id);
CREATE INDEX idx_roles_tenant ON roles (tenant_id);
CREATE INDEX idx_user_roles_user ON user_roles (user_id);

-- --- Row Level Security (tenant isolation) --------------------------------
-- current_setting('app.current_tenant', true) is NULL when unset, so an
-- unscoped connection sees zero rows (fail-closed). Bound per transaction by the
-- iam postgres adapter via SET LOCAL / set_config.

ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON users
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE roles ENABLE ROW LEVEL SECURITY;
ALTER TABLE roles FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON roles
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE user_roles ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_roles FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON user_roles
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
