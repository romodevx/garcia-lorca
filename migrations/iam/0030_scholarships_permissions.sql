-- iam — scholarships:read and scholarships:write, the permissions of the Becas
-- module (T18, ADR 0022).
--
-- scholarships:read  — the scholarships (kind, value, what they apply to), the
--                      list of what they can apply to, and who holds each.
-- scholarships:write — create and edit scholarships, give them to pupils and
--                      take them away. A scholarship changes what pending
--                      charges cost; collecting them is still Registrar pago
--                      (finance:write).
--
-- Plan §1: the owner holds both by default and the director may assign them
-- to other roles — so they are NOT in domain.ownerOnlyPermissions, and the
-- admin surface offers them. Granted by the owner's fixed id
-- (domain.OwnerRoleID), like 0021–0029, so a renamed owner role still gets
-- them. Idempotent. RLS: roles is FORCE ROW LEVEL SECURITY, so SET LOCAL binds
-- the tenant.

BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

UPDATE roles
   SET permissions = permissions || ARRAY['scholarships:read']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND id = '00000000-0000-0000-0000-0000000000a1'
   AND NOT ('scholarships:read' = ANY (permissions));

UPDATE roles
   SET permissions = permissions || ARRAY['scholarships:write']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND id = '00000000-0000-0000-0000-0000000000a1'
   AND NOT ('scholarships:write' = ANY (permissions));

COMMIT;
