-- iam (Phase 9b support) — guardian management permissions + the guardian role.
--
-- Two distinct surfaces, deliberately kept apart:
--
--   1. guardians:read / guardians:write — the ADMIN surface. These gate the
--      admin-side guardian-link routes (listing a student's guardians, linking
--      and unlinking a guardian to a student). They are appended to the seeded
--      owner role, which administers the tenant.
--
--   2. the `guardian` role carrying exactly family:read — the FAMILY app's
--      self-scoped read. A guardian signs in to the mobile family app and reads
--      only the students they are linked to.
--
-- SCOPING NOTE: family:read is a self-scoped permission, NOT a tenant-wide read.
-- The guardian role deliberately gets NONE of the owner-only tenant-wide reads
-- (finance:read, attendance:read, comms:read, grades:read) — those would let a
-- guardian read any student's data. Per-object scoping for the family path lives
-- in the gateway's /api/v1/family/* endpoints, which resolve the caller's linked
-- students and constrain every downstream read to that set. Granting tenant-wide
-- reads and filtering later is exactly the mistake this split avoids.
--
-- Both changes are idempotent: a re-run appends no duplicate permissions and
-- inserts no duplicate role.
--
-- Plain SQL only (no psql variables): this file must apply BOTH via the psql
-- runner AND via the Go integration test, which execs the SQL through GORM.
--
-- RLS: the roles table is FORCE ROW LEVEL SECURITY, so the tenant_isolation
-- policy applies even to the table owner. SET LOCAL binds app.current_tenant so
-- the UPDATE's USING and the INSERT's WITH CHECK are satisfied under a
-- non-superuser owner (e.g. Cloud SQL, which grants no BYPASSRLS). Harmless
-- where the migrator is a superuser (superusers ignore RLS regardless).

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

-- --- admin surface: owner may manage guardian links -----------------------

UPDATE roles
   SET permissions = permissions || ARRAY['guardians:read', 'guardians:write']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND name = 'owner'
   AND NOT (permissions @> ARRAY['guardians:read', 'guardians:write']);

-- --- family surface: the guardian role (default tenant) -------------------
-- Fixed UUID (mirrors the owner role seed in 0002) so re-running is a no-op and
-- the role can be referenced by id when assigning it to guardian users.

INSERT INTO roles (id, tenant_id, name, permissions)
VALUES (
    '00000000-0000-0000-0000-0000000000a2',
    '00000000-0000-0000-0000-000000000001',
    'guardian',
    -- family:read is the ONLY permission. See the SCOPING NOTE above.
    ARRAY['family:read']
)
ON CONFLICT (tenant_id, name) DO NOTHING;

COMMIT;
