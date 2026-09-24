-- students — contact emails are stored trimmed and lowercase.
--
-- WHY. The invitation that turns a contact into a family account looks the
-- address up in iam by its normalized spelling. A contact typed with capitals
-- is the same person; storing one spelling keeps the lookup exact.
--
-- guardian_students has no uniqueness on email (a mother may be the contact of
-- two siblings), so normalizing cannot collide: every row is rewritten and the
-- CHECK is added at once. '' (no email) is already normalized.
--
-- RLS: guardian_students is FORCE ROW LEVEL SECURITY, so SET LOCAL binds the one
-- school this deployment serves (same pattern as migrations/iam/0017).

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

UPDATE guardian_students
   SET email = lower(btrim(email))
 WHERE email <> lower(btrim(email));

ALTER TABLE guardian_students ADD CONSTRAINT guardian_students_email_normalized_ck
    CHECK (email = lower(btrim(email)));

COMMIT;
