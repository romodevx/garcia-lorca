-- iam — the Cobranza role (task 9, ADR 0015).
--
-- The office's cashier: finds a pupil, reads what the family owes and what is
-- due, what is late and the day's cut, and takes the money. Nothing that
-- prices (tuition tables, concepts), bills (runs), discounts, cancels a charge,
-- voids a deposit or reports income — that is finance:read/write, which stays
-- the owner's.
--
--   * collections:read — the counter's reads (gateway: finance:read OR it).
--   * payments:write   — taking money through deposits, priced (finance:write OR it).
--     Not POST /finance/payments: one charge at its stored amount, no late fee.
--
-- The owner holds both, so it can delegate them (the caller-subset guard: an
-- admin grants only what it holds). Neither is owner-only: reading every
-- family's balance is exactly this role's job.
--
-- The role gets a fixed id, like guardian (0010), when it is new. Idempotent. RLS: roles is
-- FORCE ROW LEVEL SECURITY, so SET LOCAL binds the tenant.

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

-- The owner by its fixed id (domain.OwnerRoleID), as the owner guard finds it:
-- a renamed owner role must still receive what it has to delegate.
UPDATE roles
   SET permissions = permissions || ARRAY['collections:read']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND id = '00000000-0000-0000-0000-0000000000a1'
   AND NOT ('collections:read' = ANY (permissions));

UPDATE roles
   SET permissions = permissions || ARRAY['payments:write']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND id = '00000000-0000-0000-0000-0000000000a1'
   AND NOT ('payments:write' = ANY (permissions));

INSERT INTO roles (id, tenant_id, name, permissions)
VALUES (
    '00000000-0000-0000-0000-0000000000a3',
    '00000000-0000-0000-0000-000000000001',
    'cobranza',
    ARRAY['students:read', 'students:list', 'collections:read', 'payments:write']
)
-- A role the school already named "cobranza" is brought to exactly these four
-- permissions rather than skipped: left as it was, it could keep broader access
-- (finance:write) or none, and nothing would say so. It keeps its own id.
ON CONFLICT (tenant_id, name) DO UPDATE SET permissions = EXCLUDED.permissions;

COMMIT;
