-- finance (M16) — débito o crédito.
--
-- The school asked to record which kind of card a payment came in on. It is not
-- a new payment method: the money still arrives by card, and the ledger, the
-- cash-up and the export all treat it as one. What changes is that the office
-- can tell a debit slip from a credit one when reconciling with the terminal,
-- which is the report they actually build at the end of the month.
--
-- Empty is the ordinary state: cash and transfers have no card type, and so do
-- the card payments taken before today. The CHECK keeps the two consistent —
-- a card type on a cash payment is a contradiction somebody would eventually
-- have to explain.
BEGIN;

ALTER TABLE deposits ADD COLUMN card_type text NOT NULL DEFAULT ''
    CHECK (card_type IN ('', 'debit', 'credit'));

ALTER TABLE deposits ADD CONSTRAINT deposits_card_type_needs_a_card_ck
    CHECK (card_type = '' OR method = 'card');

COMMIT;
