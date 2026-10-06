-- iam — students:import, the permission to migrate students from the legacy
-- system's Excel export (PLAN-student-import item 3, ADR 0033).
--
-- One import creates the missing catalog (levels, grades, groups) and creates
-- and enrolls every student of the file: it composes config:write +
-- students:create + students:enroll. It is the owner's decision alone:
-- students:import is in domain.ownerOnlyPermissions, so the admin surface
-- refuses to grant it to any other role.
--
-- Granted by the owner's fixed id (domain.OwnerRoleID), like 0021–0030, so a
-- renamed owner role still gets it. Idempotent. RLS: roles is FORCE ROW LEVEL
-- SECURITY, so SET LOCAL binds the tenant.

BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

UPDATE roles
   SET permissions = permissions || ARRAY['students:import']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND id = '00000000-0000-0000-0000-0000000000a1'
   AND NOT ('students:import' = ANY (permissions));

COMMIT;
