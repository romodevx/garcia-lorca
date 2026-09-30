-- iam — deleting a user (T2): a soft delete, and the permission to do it.
--
-- WHY SOFT. Payments a user registered, grades they captured and the audit
-- trail reference the account by id plus a name snapshot, with no foreign keys
-- across services. A hard delete would leave those ids pointing at nothing, so
-- the row stays and gains a fourth status: `deleted` cannot sign in and leaves
-- every listing. Inviting the same address again brings THIS row back as
-- `invited` (owner answer 3), history attached.
--
-- The stamp records when and by whom. `deleted_by` has no foreign key on
-- purpose: users are never hard-deleted, and a stamp must not be able to block
-- anything. The consistency CHECK keeps status and stamp moving together, so a
-- half-deleted row cannot exist.
--
-- users:delete is owner-only (domain.ownerOnlyPermissions, owner answer 4) and
-- granted by the owner's fixed id, like 0021–0024, so a renamed owner role
-- still gets it. Idempotent throughout. RLS: roles is FORCE ROW LEVEL SECURITY,
-- so SET LOCAL binds the tenant for the grant.

BEGIN;

SET LOCAL lock_timeout = '5s';

-- --- users.status gains `deleted` ----------------------------------------
-- A strict superset of 0016's CHECK: no existing row can violate it.

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_status_check;
ALTER TABLE users ADD CONSTRAINT users_status_check
    CHECK (status IN ('active', 'suspended', 'invited', 'deleted'));

ALTER TABLE users ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE users ADD COLUMN IF NOT EXISTS deleted_by uuid;

-- No row is deleted before this migration, so every existing row satisfies it.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_deleted_stamp_check;
ALTER TABLE users ADD CONSTRAINT users_deleted_stamp_check
    CHECK ((status = 'deleted') = (deleted_at IS NOT NULL)
       AND (deleted_by IS NULL OR deleted_at IS NOT NULL));

-- --- the permission ------------------------------------------------------

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

UPDATE roles
   SET permissions = permissions || ARRAY['users:delete']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND id = '00000000-0000-0000-0000-0000000000a1'
   AND NOT ('users:delete' = ANY (permissions));

COMMIT;
