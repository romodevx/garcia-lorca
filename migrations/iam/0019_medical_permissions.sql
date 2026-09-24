-- iam — medical:read and medical:write for the owner (task 5).
--
-- A pupil's medical record (students/0009) is served under its own
-- permissions, not students:read: a teacher who reads the class list is not
-- thereby entitled to its allergies. Only the owner holds them: both are in
-- iam's owner-only set, so the admin surface refuses to grant them to any
-- other role (every pupil's health data, with no per-pupil scoping).
--
-- Idempotent. RLS: roles is FORCE ROW LEVEL SECURITY, so SET LOCAL binds the
-- tenant (same pattern as 0017).

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

UPDATE roles
   SET permissions = permissions || ARRAY['medical:read']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND name = 'owner'
   AND NOT ('medical:read' = ANY (permissions));

UPDATE roles
   SET permissions = permissions || ARRAY['medical:write']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND name = 'owner'
   AND NOT ('medical:write' = ANY (permissions));

COMMIT;
