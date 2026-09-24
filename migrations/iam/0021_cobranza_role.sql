-- iam — the Cobranza role (task 9, ADR 0015).
--
-- The office's cashier: finds a pupil, reads what the family owes and what is
-- due, what is late and the day's cut, and takes the money. Nothing that
-- prices (tuition tables, concepts), bills (runs), discounts, cancels a charge,
-- voids a deposit or reports income — that is finance:read/write, which stays
-- the owner's.
--
--   * collections:read — the counter's reads (gateway: finance:read OR it).
--   * payments:write   — taking money: payments and deposits (finance:write OR it).
--
-- The owner holds both, so it can delegate them (the caller-subset guard: an
-- admin grants only what it holds). Neither is owner-only: reading every
-- family's balance is exactly this role's job.
--
-- The role gets a fixed id, like guardian (0010). Idempotent. RLS: roles is
-- FORCE ROW LEVEL SECURITY, so SET LOCAL binds the tenant.

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

UPDATE roles
   SET permissions = permissions || ARRAY['collections:read']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND name = 'owner'
   AND NOT ('collections:read' = ANY (permissions));

UPDATE roles
   SET permissions = permissions || ARRAY['payments:write']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND name = 'owner'
   AND NOT ('payments:write' = ANY (permissions));

INSERT INTO roles (id, tenant_id, name, permissions)
VALUES (
    '00000000-0000-0000-0000-0000000000a3',
    '00000000-0000-0000-0000-000000000001',
    'cobranza',
    ARRAY['students:read', 'students:list', 'collections:read', 'payments:write']
)
ON CONFLICT (tenant_id, name) DO NOTHING;

COMMIT;
