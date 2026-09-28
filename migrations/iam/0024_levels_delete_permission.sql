-- iam — levels:delete, the permission to delete an education level (ADR 0021).
--
-- Deleting a level erases its whole academic history (grades, attendance,
-- enrollments, report cards) school-wide, behind a 7-day trash, and asks for
-- the director's password each time. It is the owner's decision alone:
-- levels:delete is in domain.ownerOnlyPermissions, so the admin surface refuses
-- to grant it to any other role.
--
-- Granted by the owner's fixed id (domain.OwnerRoleID), like 0021–0023, so a
-- renamed owner role still gets it. Idempotent. RLS: roles is FORCE ROW LEVEL
-- SECURITY, so SET LOCAL binds the tenant.

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

UPDATE roles
   SET permissions = permissions || ARRAY['levels:delete']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND id = '00000000-0000-0000-0000-0000000000a1'
   AND NOT ('levels:delete' = ANY (permissions));

COMMIT;
