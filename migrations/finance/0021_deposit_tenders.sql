-- finance (T8) — a payment can combine methods, and one of them is a credit note.
--
-- THE PROBLEM. Some families hold a credit note the school issued by hand. The
-- school does not automate credit balances: the cashier picks "Nota de crédito",
-- types its amount, and completes the rest in cash, by card or by transfer. A
-- deposit had room for exactly one method.
--
-- THE SHAPE. deposit_tenders holds the methods a deposit came in by: one row per
-- method, in the order the cashier listed them, adding up to the deposit's
-- amount (enforced by the domain inside the deposit's transaction — a CHECK
-- cannot sum across rows). It is the source of truth for "how was this paid".
--
--   * deposits.method stays, now a DERIVED summary written with the deposit:
--     the one tender's method, or 'mixed' when there are several. Kept so every
--     reader and report that already reads it keeps working; new readers use
--     the tenders.
--   * deposits.card_type likewise: the one card tender's type, '' when mixed.
--     The card type lives on each card tender.
--   * payments.method gains credit_note. A deposit's allocations are cut where
--     one tender ends and the next begins, so every payment row carries exactly
--     one method and "how much of this concept was paid with a credit note" is
--     a sum, not an apportionment.
--
-- A credit note is not money: cash-ups and income reports keep it on its own
-- line, outside the takings.
--
-- BACKFILL, additive and non-destructive: every existing deposit gets ONE tender
-- with its own method, amount, reference and card type. No existing row changes.
-- Reversible: DROP TABLE deposit_tenders and restore the two method CHECKs (no
-- deposit is 'mixed' or 'credit_note' until T8 code writes one).
--
-- Following finance/0018: the tables are FORCE RLS and the migrator has no
-- BYPASSRLS, so SET LOCAL binds the one school this deployment serves for the
-- backfill.
BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

ALTER TABLE deposits DROP CONSTRAINT deposits_method_check;
ALTER TABLE deposits ADD CONSTRAINT deposits_method_check
    CHECK (method IN ('cash', 'card', 'transfer', 'credit_note', 'mixed'));

ALTER TABLE payments DROP CONSTRAINT payments_method_check;
ALTER TABLE payments ADD CONSTRAINT payments_method_check
    CHECK (method IN ('cash', 'card', 'transfer', 'credit_note'));

CREATE TABLE deposit_tenders (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL,
    -- RESTRICT, like the deposit's payments: a deposit is voided, never deleted.
    deposit_id   uuid NOT NULL REFERENCES deposits (id) ON DELETE RESTRICT,
    -- The order the cashier listed them: allocations are cut in this order.
    position     smallint NOT NULL CHECK (position BETWEEN 0 AND 3),
    method       text NOT NULL CHECK (method IN ('cash', 'card', 'transfer', 'credit_note')),
    amount_cents bigint NOT NULL CHECK (amount_cents > 0),
    -- Free text: a credit note's folio, a voucher, a transfer id.
    reference    text NOT NULL DEFAULT '',
    card_type    text NOT NULL DEFAULT '' CHECK (card_type IN ('', 'debit', 'credit')),
    created_at   timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT deposit_tenders_card_type_needs_a_card_ck
        CHECK (card_type = '' OR method = 'card'),
    CONSTRAINT deposit_tenders_position_unique UNIQUE (deposit_id, position)
);

-- The one read: a page of deposits' tenders, by deposit.
CREATE INDEX idx_deposit_tenders_deposit ON deposit_tenders (tenant_id, deposit_id);

ALTER TABLE deposit_tenders ENABLE ROW LEVEL SECURITY;
ALTER TABLE deposit_tenders FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON deposit_tenders
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

INSERT INTO deposit_tenders (tenant_id, deposit_id, position, method, amount_cents, reference, card_type, created_at)
SELECT tenant_id, id, 0, method, amount_cents, reference, card_type, created_at
  FROM deposits;

COMMIT;
