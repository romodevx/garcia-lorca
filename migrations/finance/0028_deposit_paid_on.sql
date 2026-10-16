-- finance (T5b, ADR 0035) — the day a payment is judged on, stored apart.
--
-- THE PROBLEM. paid_at held two different things. A date picked at the
-- counter arrives as midnight UTC of that day; a live payment stores the real
-- instant. The school is in Morelia (UTC-6), so a live payment at 19:00 on the
-- 15th is 01:00 UTC on the 16th, and every reader that took the UTC date of
-- paid_at judged it the NEXT day: paid_by_due (whether a scholarship is kept
-- for paying on time), the cash cut and income windows, the statement line,
-- the receipt's "Fecha de pago", and a correction that kept the date.
--
-- THE SHAPE. paid_on is the school-local civil date: the day picked at the
-- counter, or the school's today (clock.Today()) for a live payment. paid_at
-- stays as it is, the audit instant. Both deposits and payments get it,
-- because a payment straight against a charge has the same two meanings and
-- paid_by_due sums payments. A deposit's allocations carry its day.
--
-- THE BACKFILL reads paid_at by its shape:
--   * exactly midnight UTC ⇒ a day picked at the counter: its UTC date;
--   * anything else        ⇒ a live instant: its date in America/Mexico_City.
-- A live payment taken at exactly 00:00:00.000000 UTC (18:00 in Morelia) would
-- read as the next day; the microsecond clock makes that a non-case.
--
-- RLS is untouched: the tables keep their tenant_isolation policies.
-- Reversible: DROP the two indexes and the two columns.
BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE deposits ADD COLUMN paid_on date;
ALTER TABLE payments ADD COLUMN paid_on date;

UPDATE deposits SET paid_on = CASE
    WHEN (paid_at AT TIME ZONE 'UTC')::time = time '00:00'
        THEN (paid_at AT TIME ZONE 'UTC')::date
    ELSE (paid_at AT TIME ZONE 'America/Mexico_City')::date
END;

UPDATE payments SET paid_on = CASE
    WHEN (paid_at AT TIME ZONE 'UTC')::time = time '00:00'
        THEN (paid_at AT TIME ZONE 'UTC')::date
    ELSE (paid_at AT TIME ZONE 'America/Mexico_City')::date
END;

-- No DEFAULT: a row nobody dated is a bug to refuse, not a day to guess.
ALTER TABLE deposits ALTER COLUMN paid_on SET NOT NULL;
ALTER TABLE payments ALTER COLUMN paid_on SET NOT NULL;

-- The day's reads: the cash cut, the income windows, "did this day change".
CREATE INDEX idx_deposits_paid_on ON deposits (tenant_id, paid_on);
CREATE INDEX idx_payments_paid_on ON payments (tenant_id, paid_on);

COMMIT;
