-- iam — schedule:read and schedule:write, the permissions of the Horarios
-- module (T16).
--
-- schedule:read  — the school timetable (week by group, grade, level or
--                  teacher), "¿Qué está pasando ahora?", the free teachers of
--                  a slot and the change history.
-- schedule:write — build the base timetable, change one date (substitute,
--                  swap, cancel), undo a change, set a level's time grid.
-- A teacher's own timetable is teaching:read (self-scoped), not these.
--
-- Plan §1: the owner holds both by default and the director may assign them
-- to other roles — so they are NOT in domain.ownerOnlyPermissions, and the
-- admin surface offers them. Granted by the owner's fixed id
-- (domain.OwnerRoleID), like 0021–0027, so a renamed owner role still gets
-- them. Idempotent. RLS: roles is FORCE ROW LEVEL SECURITY, so SET LOCAL binds
-- the tenant.

BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

UPDATE roles
   SET permissions = permissions || ARRAY['schedule:read']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND id = '00000000-0000-0000-0000-0000000000a1'
   AND NOT ('schedule:read' = ANY (permissions));

UPDATE roles
   SET permissions = permissions || ARRAY['schedule:write']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND id = '00000000-0000-0000-0000-0000000000a1'
   AND NOT ('schedule:write' = ANY (permissions));

COMMIT;
