-- finance (T8, review) — a payment row remembers debit or credit.
--
-- A combined payment against one charge (Registrar pago, a uniforms sale) is
-- one payment row per tender and has no deposit to carry the card type, and a
-- deposit paid by two cards splits into payments whose card is per tender. The
-- office reconciles every card payment against the terminal at month end, so
-- the row keeps it, under the same rule as deposits.card_type (0009).
--
-- Schema only, additive: existing rows read '' (not recorded), which is what
-- every card payment taken before this already meant.
BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE payments ADD COLUMN card_type text NOT NULL DEFAULT ''
    CHECK (card_type IN ('', 'debit', 'credit'));
ALTER TABLE payments ADD CONSTRAINT payments_card_type_needs_a_card_ck
    CHECK (card_type = '' OR method = 'card');

COMMIT;
