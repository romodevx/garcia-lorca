-- iam — payments:edit, the permission to correct a registered payment (T9).
--
-- A correction replaces a receipt: the original is kept, superseded, and a new
-- one carries the corrected amount, date, methods and concepts, with the
-- reason and who did it on record (finance/0023). It moves money between the
-- family's months, so it is its own permission, not payments:write (taking
-- money at the listed price) nor finance:write.
--
-- Owner answer 6: the owner holds it by default and the director may assign
-- it to other roles — so it is NOT in domain.ownerOnlyPermissions, and the
-- admin surface offers it. Granted by the owner's fixed id
-- (domain.OwnerRoleID), like 0021–0025, so a renamed owner role still gets it.
-- Idempotent. RLS: roles is FORCE ROW LEVEL SECURITY, so SET LOCAL binds the
-- tenant.

BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

UPDATE roles
   SET permissions = permissions || ARRAY['payments:edit']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND id = '00000000-0000-0000-0000-0000000000a1'
   AND NOT ('payments:edit' = ANY (permissions));

COMMIT;
