-- iam — grades:close, the permission to close a trimester (T7).
--
-- Closing a period locks every group's grades for that trimester school-wide
-- and is what makes the boleta final. It is the owner's decision alone:
-- grades:close is in domain.ownerOnlyPermissions, so the admin surface refuses
-- to grant it to any other role. grades:write (capturing) stays delegable.
--
-- Granted by the owner's fixed id (domain.OwnerRoleID), like 0021/0022, so a
-- renamed owner role still gets it. Idempotent. RLS: roles is FORCE ROW LEVEL
-- SECURITY, so SET LOCAL binds the tenant.

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

UPDATE roles
   SET permissions = permissions || ARRAY['grades:close']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND id = '00000000-0000-0000-0000-0000000000a1'
   AND NOT ('grades:close' = ANY (permissions));

COMMIT;
