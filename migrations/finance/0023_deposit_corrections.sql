-- finance (T9) — a registered payment can be corrected, and the original is kept.
--
-- THE PROBLEM. A cashier keys $500 when the family handed over $1,000, or the
-- wrong day, or cash for what was a transfer. Voiding and re-keying loses the
-- link between the two receipts and records nobody's reason for the change.
--
-- THE SHAPE. A correction is ONE transaction in finance:
--
--   * the original deposit is SUPERSEDED — superseded_at / superseded_by set,
--     its allocations released exactly as a void releases them. The row, its
--     folio and its tenders stay: a school that deletes money rows cannot
--     explain its own ledger. A superseded deposit counts for nothing.
--   * a NEW deposit carries the corrected amount, date and tenders, with its
--     own folio and corrects_deposit_id naming the original. It is split by
--     the same rule as any deposit, on the corrected date, so surcharges and
--     the early-payment reward are worked out again by the same arithmetic.
--   * deposit_corrections records it: before and after (JSON), who corrected
--     it (user id + name as it read then), when, and the mandatory reason.
--
-- superseded_by and corrects_deposit_id say the same link from both ends; the
-- unique index on corrects_deposit_id makes "corrected at most once" a fact of
-- the schema (a correction of a correction corrects the NEW deposit).
--
-- Schema only, additive: every existing deposit reads as never corrected.
-- Reversible: DROP TABLE deposit_corrections and the three columns.
BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE deposits
    ADD COLUMN superseded_at       timestamptz NULL,
    ADD COLUMN superseded_by       uuid NULL REFERENCES deposits (id) ON DELETE RESTRICT,
    ADD COLUMN corrects_deposit_id uuid NULL REFERENCES deposits (id) ON DELETE RESTRICT;

ALTER TABLE deposits ADD CONSTRAINT deposits_superseded_pair_ck
    CHECK ((superseded_at IS NULL) = (superseded_by IS NULL));
-- A deposit is either cancelled or replaced by its correction, never both.
ALTER TABLE deposits ADD CONSTRAINT deposits_void_or_superseded_ck
    CHECK (voided_at IS NULL OR superseded_at IS NULL);

CREATE UNIQUE INDEX uq_deposits_corrects ON deposits (tenant_id, corrects_deposit_id)
    WHERE corrects_deposit_id IS NOT NULL;

CREATE TABLE deposit_corrections (
    id                   uuid PRIMARY KEY,
    tenant_id            uuid NOT NULL,
    -- The pupil whose ledger it is: the history is read per pupil.
    student_id           uuid NOT NULL,
    original_deposit_id  uuid NOT NULL REFERENCES deposits (id) ON DELETE RESTRICT,
    corrected_deposit_id uuid NOT NULL REFERENCES deposits (id) ON DELETE RESTRICT,
    reason               text NOT NULL CHECK (btrim(reason) <> ''),
    -- {amountCents, paidAt, folio, tenders[], lines[]} before and after.
    before               jsonb NOT NULL,
    after                jsonb NOT NULL,
    corrected_by         uuid NULL,
    corrected_by_name    text NOT NULL DEFAULT '',
    corrected_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT deposit_corrections_original_unique UNIQUE (original_deposit_id),
    CONSTRAINT deposit_corrections_corrected_unique UNIQUE (corrected_deposit_id)
);

CREATE INDEX idx_deposit_corrections_student ON deposit_corrections (tenant_id, student_id, corrected_at);

ALTER TABLE deposit_corrections ENABLE ROW LEVEL SECURITY;
ALTER TABLE deposit_corrections FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON deposit_corrections
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
