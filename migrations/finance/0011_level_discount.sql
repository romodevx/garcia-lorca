-- finance (M16) — el descuento por nivel.
--
-- The school gives maternal a discount. Any level may have one: a fixed amount
-- off each month, set once on the tuition table.
--
-- Once on the table and not per pupil, because that is the difference between a
-- rule and four hundred chances to mistype it. A discount that belongs to
-- somebody in particular — a scholarship, a courtesy, an arrangement — is
-- already expressible on the charge itself, with its own reason.
--
-- The CHECK keeps a discount from exceeding the month it comes off: a charge
-- that owes negative money would pay for itself and then some.
BEGIN;

ALTER TABLE tuition_plans ADD COLUMN monthly_discount_cents bigint NOT NULL DEFAULT 0
    CHECK (monthly_discount_cents >= 0);

ALTER TABLE tuition_plans ADD CONSTRAINT tuition_plans_discount_fits_ck
    CHECK (monthly_discount_cents <= monthly_cents);

COMMIT;
