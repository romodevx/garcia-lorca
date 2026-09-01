-- students (M16) — a pupil has more than one contact, and most of them have no
-- account.
--
-- WHAT WAS WRONG. guardian_students required an iam user: to record that a
-- grandmother may collect the child, or that the father's phone is the one that
-- answers, somebody first had to create a login for them. So the school did not
-- record it. The office kept the second number on paper, and the system showed
-- one contact per pupil as though that were the truth.
--
-- WHAT CHANGES. The user link becomes optional and the row learns to hold a
-- person: a name, a phone, an email. A contact WITH a user id is a family that
-- can open the app; a contact WITHOUT one is a phone number the office can
-- call. Both are contacts, and there is still exactly one place that lists them.
--
-- The CHECK is what keeps the row meaningful: a contact with neither an account
-- nor a name is nobody.
--
-- Additive and backward compatible on purpose. Every existing row keeps its user
-- id and gains empty strings; the family-auth read filters on guardian_user_id
-- and never sees the account-less rows. The UNIQUE stays as it was — Postgres
-- does not collide NULLs, which is exactly what lets one pupil have several
-- contacts without accounts.
BEGIN;

ALTER TABLE guardian_students ALTER COLUMN guardian_user_id DROP NOT NULL;

ALTER TABLE guardian_students ADD COLUMN full_name text NOT NULL DEFAULT '';
ALTER TABLE guardian_students ADD COLUMN phone     text NOT NULL DEFAULT '';
ALTER TABLE guardian_students ADD COLUMN email     text NOT NULL DEFAULT '';

ALTER TABLE guardian_students ADD CONSTRAINT guardian_students_is_somebody_ck
    CHECK (guardian_user_id IS NOT NULL OR full_name <> '');

COMMIT;
