-- iam — login emails are stored trimmed and lowercase.
--
-- WHY. The login and the invitation look an address up by its normalized
-- spelling, but UNIQUE (users.tenant_id, email) is case-sensitive: "Ana@x.mx"
-- and "ana@x.mx" could both exist, and a row typed with capitals before the
-- application normalized it would never be found again.
--
-- WHAT IT DOES.
--   1. Rows that would COLLIDE once lowercased are recorded in user_email_conflicts
--      and left exactly as they are. Nothing is deleted: which of the two is
--      the real person is a decision for the school, not for a migration.
--   2. Every other row is rewritten as lower(btrim(email)).
--   3. The CHECK users_email_normalized_ck forbids an unnormalized email from then on — but only
--      when there is nothing left to resolve. While collisions remain it is
--      NOT added (a NOT VALID check would still reject updating the colliding
--      rows); a follow-up migration adds it once they are resolved. The
--      migration raises a WARNING naming the count so the run log shows it.
--
-- With the CHECK in place the existing UNIQUE (tenant_id, email) is already
-- case-insensitive: every stored spelling is the lowercase one.
--
-- RLS: users is FORCE ROW LEVEL SECURITY, so SET LOCAL binds the one school
-- this deployment serves (same pattern as migrations/iam/0017). The CHECK
-- itself is validated over the whole table, independently of RLS.

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

CREATE TABLE user_email_conflicts (
    user_id         uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    tenant_id        uuid NOT NULL,
    original_email   text NOT NULL,
    normalized_email text NOT NULL,
    detected_at      timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE user_email_conflicts ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_email_conflicts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON user_email_conflicts
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

INSERT INTO user_email_conflicts (user_id, tenant_id, original_email, normalized_email)
SELECT t.id, t.tenant_id, t.email, lower(btrim(t.email))
  FROM users t
 WHERE EXISTS (
        SELECT 1
          FROM users other
         WHERE other.tenant_id = t.tenant_id
           AND other.id <> t.id
           AND lower(btrim(other.email)) = lower(btrim(t.email)));

UPDATE users
   SET email = lower(btrim(email))
 WHERE email <> lower(btrim(email))
   AND id NOT IN (SELECT user_id FROM user_email_conflicts);

DO $$
DECLARE
    pending bigint;
BEGIN
    SELECT count(*) INTO pending FROM user_email_conflicts;
    IF pending > 0 THEN
        RAISE WARNING 'iam: % users email(s) collide once lowercased — see user_email_conflicts; users_email_normalized_ck not added', pending;
    ELSE
        ALTER TABLE users ADD CONSTRAINT users_email_normalized_ck CHECK (email = lower(btrim(email)));
    END IF;
END
$$;

COMMIT;
