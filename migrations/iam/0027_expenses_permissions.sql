-- iam — expenses:read and expenses:write, the permissions of the Egresos
-- module (T10).
--
-- expenses:read  — the expense list, its history and receipts, the period's
--                  income/expenses/balance and the reports (aggregates only:
--                  no family is named in them).
-- expenses:write — record, edit and cancel expenses, manage the categories,
--                  attach receipts, close the day's cash cut.
-- Reopening a closed cut stays the director's (finance:write).
--
-- Owner answer 7: the owner holds both by default and the director may assign
-- them to other roles — so they are NOT in domain.ownerOnlyPermissions, and the
-- admin surface offers them. Granted by the owner's fixed id
-- (domain.OwnerRoleID), like 0021–0026, so a renamed owner role still gets
-- them. Idempotent. RLS: roles is FORCE ROW LEVEL SECURITY, so SET LOCAL binds
-- the tenant.

BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

UPDATE roles
   SET permissions = permissions || ARRAY['expenses:read']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND id = '00000000-0000-0000-0000-0000000000a1'
   AND NOT ('expenses:read' = ANY (permissions));

UPDATE roles
   SET permissions = permissions || ARRAY['expenses:write']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND id = '00000000-0000-0000-0000-0000000000a1'
   AND NOT ('expenses:write' = ANY (permissions));

COMMIT;
