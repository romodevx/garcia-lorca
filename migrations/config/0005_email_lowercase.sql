-- config — teacher emails are stored trimmed and lowercase.
--
-- WHY. The login and the invitation look an address up by its normalized
-- spelling, but UNIQUE (teachers.tenant_id, email) is case-sensitive: "Ana@x.mx"
-- and "ana@x.mx" could both exist, and a row typed with capitals before the
-- application normalized it would never be found again.
--
-- WHAT IT DOES.
--   1. If any two rows of one tenant would COLLIDE once normalized, the
--      migration STOPS and changes nothing. It names the colliding row ids
--      (ids, not addresses: deploy logs must not carry personal data). Nothing
--      is deleted — which of the two is the real person is a decision for the
--      school, not for a migration. Fix the rows by hand and re-run.
--   2. Otherwise every row is rewritten to config_normalized_email(email) and the CHECK
--      teachers_email_normalized_ck forbids an unnormalized email from then on. With it,
--      the existing UNIQUE (tenant_id, email) is already case-insensitive.
--
-- Failing instead of skipping keeps the schema identical in every environment:
-- a CHECK that exists here and not there is a bug nobody sees until it bites.
--
-- RLS: teachers is FORCE ROW LEVEL SECURITY and the cloud migrator owns it
-- without BYPASSRLS, so SET LOCAL binds the one school this deployment serves
-- (same pattern as migrations/iam/0017). The CHECK is validated over the whole
-- table, independently of RLS.
--
-- lock_timeout: ADD CONSTRAINT takes ACCESS EXCLUSIVE on teachers while the
-- previous revision is still serving; waiting behind a long reader would queue
-- every request behind the migration. Fail fast and re-run instead.

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';
SET LOCAL lock_timeout = '5s';

-- config_normalized_email(email) is the one spelling an address is stored under: surrounding
-- whitespace removed and lowercase. It mirrors Go's strings.ToLower(
-- strings.TrimSpace(x)): TrimSpace strips every Unicode White_Space rune, and
-- btrim() strips only U+0020, so a tab, CR/LF or no-break space would survive a
-- btrim and pass the CHECK while Go never finds the row again. The class is
-- spelled out rune by rune (Go's unicode.IsSpace) rather than \s, which under
-- ICU also matches U+001C–U+001F — runes Go does NOT trim. The ICU collation
-- pins lower() to Unicode case mapping: under a C ctype the database's own
-- lower() folds ASCII only, and "JOSÉ" would become "josÉ". (ICU applies the
-- Greek final-sigma rule and Go does not; no address this school stores ends
-- a word in a capital sigma.)
CREATE FUNCTION config_normalized_email(email text) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    RETURN lower(regexp_replace(email COLLATE "und-x-icu", '^[ \t\n\v\f\r\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+|[ \t\n\v\f\r\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+$', '', 'g'));

DO $$
DECLARE
    clashes text;
BEGIN
    SELECT string_agg(id::text, ', ' ORDER BY normalized, id)
      INTO clashes
      FROM (SELECT id, config_normalized_email(email) AS normalized,
                   count(*) OVER (PARTITION BY tenant_id, config_normalized_email(email)) AS spellings
              FROM teachers) t
     WHERE spellings > 1;
    IF clashes IS NOT NULL THEN
        RAISE EXCEPTION 'config: teachers emails collide once lowercased; nothing was changed. Resolve these rows by hand and re-run: %', clashes
            USING ERRCODE = 'unique_violation';
    END IF;
END
$$;

UPDATE teachers
   SET email = config_normalized_email(email)
 WHERE email <> config_normalized_email(email);

ALTER TABLE teachers ADD CONSTRAINT teachers_email_normalized_ck CHECK (email = config_normalized_email(email));

COMMIT;
