-- finance (M16) — el abono: money arrives before it is decided what it pays.
--
-- THE PROBLEM. Until now a payment named exactly one charge, because that is how
-- the screen worked: pick a debt, pay it. The school does not work that way. A
-- parent hands over $6,700 and says nothing else; the office is expected to know
-- that it covers September, the surcharge September earned, and October. Asking
-- the cashier to split that by hand is how a family ends up with October paid
-- and September still accruing $10 a day.
--
-- THE SHAPE. A deposit is the money as it arrived: one amount, one date, one
-- method, one receipt. Its allocations are payments — the same `payments` table,
-- now carrying deposit_id — so there is exactly ONE place that answers "what has
-- been collected against this charge". A second table of allocations would be a
-- second answer to that question, and the two would drift the first time one
-- write failed.
--
-- A payment with deposit_id NULL is a payment made the old way, against one
-- named charge. Nothing about it changes.
--
-- IDEMPOTENCY MOVES UP. The key now belongs to the deposit: the retry a lost
-- response provokes is "I handed over $6,700 again", not "I paid $300 against
-- charge X again". The old partial index on payments stays for the direct path.
--
-- VOIDING IS AN ENTRY, NEVER A DELETE. A cashier who keys the wrong amount has
-- to be able to undo it, and a school that deletes money rows cannot explain its
-- own ledger. Voiding stamps voided_at with a reason and releases the
-- allocations; the deposit row stays, and so does what it was.
BEGIN;

CREATE TABLE deposits (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL,
    student_id   uuid NOT NULL,
    amount_cents bigint NOT NULL CHECK (amount_cents > 0),
    method       text NOT NULL CHECK (method IN ('cash', 'card', 'transfer')),
    reference    text NOT NULL DEFAULT '',
    paid_at      timestamptz NOT NULL DEFAULT now(),
    idempotency_key text NOT NULL DEFAULT '',
    -- Which school calendar the allocation was worked out against. A discount or
    -- a surcharge is a number someone has to defend months later; without this,
    -- a recomputation cannot tell whether it is looking at the same calendar the
    -- cashier saw. 'unavailable' means config could not be reached and the
    -- deadline was worked out without the school's holidays.
    calendar_version text NOT NULL DEFAULT '',
    voided_at    timestamptz NULL,
    void_reason  text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT deposits_void_has_reason_ck
        CHECK (voided_at IS NULL OR void_reason <> '')
);

-- The two reads: a family's deposits, newest first, and the day's takings.
CREATE INDEX idx_deposits_student ON deposits (tenant_id, student_id, paid_at DESC);
CREATE INDEX idx_deposits_paid_at ON deposits (tenant_id, paid_at DESC);

-- Partial, for the same reason as payments: the blank key is not a key, and a
-- plain UNIQUE would allow exactly one keyless deposit per school.
CREATE UNIQUE INDEX uq_deposits_idempotency ON deposits (tenant_id, idempotency_key)
    WHERE idempotency_key <> '';

ALTER TABLE deposits ENABLE ROW LEVEL SECURITY;
ALTER TABLE deposits FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON deposits
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

-- The link. NULL means a payment made the old way, against one named charge.
-- ON DELETE RESTRICT: a deposit with allocations cannot be deleted, only voided.
ALTER TABLE payments
    ADD COLUMN deposit_id uuid NULL REFERENCES deposits (id) ON DELETE RESTRICT;

CREATE INDEX idx_payments_deposit ON payments (tenant_id, deposit_id)
    WHERE deposit_id IS NOT NULL;

COMMIT;
