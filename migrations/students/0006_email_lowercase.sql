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
-- RLS: guardian_students is FORCE ROW LEVEL SECURITY and the cloud migrator
-- owns it without BYPASSRLS, so SET LOCAL binds the one school this deployment
-- serves (same pattern as migrations/iam/0017).
--
-- lock_timeout: ADD CONSTRAINT takes ACCESS EXCLUSIVE while the previous
-- revision is still serving; fail fast rather than queue every request.

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';
SET LOCAL lock_timeout = '5s';

-- students_normalized_email(email) is the one spelling an address is stored under: surrounding
-- whitespace removed and lowercase. It mirrors Go's strings.ToLower(
-- strings.TrimSpace(x)): TrimSpace strips every Unicode White_Space rune, and
-- btrim() strips only U+0020, so a tab, CR/LF or no-break space would survive a
-- btrim and pass the CHECK while Go never finds the row again. The ICU
-- collation pins lower() to full Unicode case folding: under a C ctype the
-- database's own lower() folds ASCII only, and "JOSÉ" would become "josÉ".
CREATE FUNCTION students_normalized_email(email text) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    RETURN lower(regexp_replace(email COLLATE "und-x-icu", '^[\s\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+|[\s\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+$', '', 'g'));

UPDATE guardian_students
   SET email = students_normalized_email(email)
 WHERE email <> students_normalized_email(email);

ALTER TABLE guardian_students ADD CONSTRAINT guardian_students_email_normalized_ck
    CHECK (email = students_normalized_email(email));

COMMIT;
