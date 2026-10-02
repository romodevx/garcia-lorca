-- comms — the school's birthday greeting (T11).
--
-- Every morning comms greets, in the family app, the pupils whose birthday it
-- is. The text is the school's: one row per tenant, `{nombre}` replaced by the
-- pupil's first name when it is sent. While no row exists comms sends its
-- default text, so a school that never opens the setting still greets.
--
-- Who sent which greeting is NOT here: each greeting is a notification, claimed
-- in processed_events as 'birthday:<student>:<year>' (0001) in the same
-- transaction, which is what stops a re-run from greeting a family twice.
--
-- updated_by is an iam user id (RPC-validated, NOT a FK), as everywhere in comms.
-- Bounds mirror the service (80 / 500 characters); the database repeats them so
-- a raw client cannot store what the screen would never show.
--
-- RLS: ENABLE + FORCE from this first migration, tenant_isolation on the
-- tenant's own row. Additive only: nothing existing changes.

BEGIN;

SET LOCAL lock_timeout = '5s';

CREATE TABLE IF NOT EXISTS birthday_templates (
    tenant_id   uuid PRIMARY KEY,
    title       text NOT NULL CHECK (char_length(btrim(title)) BETWEEN 1 AND 80),
    body        text NOT NULL CHECK (char_length(btrim(body)) BETWEEN 1 AND 500),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    updated_by  uuid NOT NULL
);

ALTER TABLE birthday_templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE birthday_templates FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON birthday_templates;
CREATE POLICY tenant_isolation ON birthday_templates
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
