-- iam — notes on a pupil (task 6): students:notes_write and students:notes_void
-- for the owner.
--
-- students:notes_write adds a note; the owner may delegate it (a teacher who
-- writes notes needs it). students:notes_void voids ANY note at any time — the
-- author can void their own for 15 minutes without it — and is in iam's
-- owner-only set: after the window, only the owner takes a note back.
--
-- Idempotent. RLS: roles is FORCE ROW LEVEL SECURITY, so SET LOCAL binds the
-- tenant (same pattern as 0019).

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

UPDATE roles
   SET permissions = permissions || ARRAY['students:notes_write']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND name = 'owner'
   AND NOT ('students:notes_write' = ANY (permissions));

UPDATE roles
   SET permissions = permissions || ARRAY['students:notes_void']
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND name = 'owner'
   AND NOT ('students:notes_void' = ANY (permissions));

COMMIT;
