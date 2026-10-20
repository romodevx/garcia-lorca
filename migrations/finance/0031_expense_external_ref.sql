-- C2-015: an expense another module records on its own behalf (a uniforms
-- refund) carries that module's reference, unique per school: the same
-- reference twice is the same expense, so a double click or a lost response
-- cannot pay a family back twice. NULL for every expense typed by a person.
BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE expenses ADD COLUMN external_ref text NULL
    CHECK (external_ref IS NULL OR (btrim(external_ref) <> '' AND char_length(external_ref) <= 120));

CREATE UNIQUE INDEX uq_expenses_external_ref ON expenses (tenant_id, external_ref) WHERE external_ref IS NOT NULL;

COMMIT;
