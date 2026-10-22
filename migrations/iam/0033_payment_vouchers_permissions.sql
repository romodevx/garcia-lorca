-- iam — vouchers_own:write, a guardian sending a payment voucher (comprobante
-- de pago) for their OWN child from the family app (ADR 0040).
--
-- It is the SECOND write permission ever granted to a guardian (the first is
-- uniforms_own:write, ADR 0010), and it carries ADR 0010's four conditions:
--   1. The permission does not say whose: ownership is the guardian->student
--      link, checked at the gateway on every call; a child that is not theirs
--      answers 404, never 403.
--   2. The scope is enforced in the service too: finance refuses a voucher
--      read that names no pupils unless it is the school's queue.
--   3. The family's figures are never money: a voucher only prefills what the
--      office confirms and registers through the existing deposit path.
--   4. There are ceilings: 5 MB per file, three pending vouchers per pupil,
--      the same file deduplicated, a rate limit per principal at the edge.
--
-- Reading their vouchers needs nothing new: family:read already scopes every
-- /api/v1/family/* read to the guardian's children. The staff side reuses
-- collections:read / payments:write (Cobranza, iam 0021) and finance:*.
--
-- Granted to the seeded guardian role by its fixed id (domain.GuardianRoleID)
-- and to the owner by its fixed id (domain.OwnerRoleID) so it can delegate it,
-- as 0014 did for uniforms_own:*. Grantable, not owner-only: it resolves no
-- pupil but the caller's own. Idempotent. RLS: roles is FORCE ROW LEVEL
-- SECURITY, so SET LOCAL binds the tenant.

BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

UPDATE roles
   SET permissions = permissions || ARRAY['vouchers_own:write']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND id IN ('00000000-0000-0000-0000-0000000000a2', '00000000-0000-0000-0000-0000000000a1')
   AND NOT ('vouchers_own:write' = ANY (permissions));

COMMIT;
