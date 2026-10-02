-- iam — activities:read and activities:write, the permissions of the
-- Actividades module (T17).
--
-- activities:read  — the activities (schedule, price options, periodicity,
--                    dates, late fee) and who is enrolled in each.
-- activities:write — create and edit activities, enrol and unenrol pupils and
--                    change a pupil's price option. Enrolling issues the
--                    periods already billable as ordinary charges; collecting
--                    them is still Registrar pago (finance:write).
--
-- Plan §1: the owner holds both by default and the director may assign them
-- to other roles — so they are NOT in domain.ownerOnlyPermissions, and the
-- admin surface offers them. Granted by the owner's fixed id
-- (domain.OwnerRoleID), like 0021–0028, so a renamed owner role still gets
-- them. Idempotent. RLS: roles is FORCE ROW LEVEL SECURITY, so SET LOCAL binds
-- the tenant.

BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

UPDATE roles
   SET permissions = permissions || ARRAY['activities:read']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND id = '00000000-0000-0000-0000-0000000000a1'
   AND NOT ('activities:read' = ANY (permissions));

UPDATE roles
   SET permissions = permissions || ARRAY['activities:write']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND id = '00000000-0000-0000-0000-0000000000a1'
   AND NOT ('activities:write' = ANY (permissions));

COMMIT;
