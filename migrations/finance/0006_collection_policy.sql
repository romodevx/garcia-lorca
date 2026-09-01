-- finance (M16) — the school's collection rules: a daily surcharge and a fixed
-- early-payment discount.
--
-- NEW COLUMNS, never a reinterpretation of late_fee_cents.
--
-- That column is documented as "the fixed surcharge added once per completed
-- overdue month" and the demo seeds it at 15000 ($150/month). Reusing it for a
-- daily rate would silently turn $150 a month into $150 a DAY — about $4,500 a
-- month, thirty times the intended charge, across the whole roll. A column whose
-- meaning changes under existing data is how a migration becomes a refund
-- programme.
--
-- late_fee_mode makes "existing plans keep their old rule" enforceable in the
-- data rather than a convention someone has to remember.
BEGIN;

ALTER TABLE tuition_plans
    -- The school's actual policy: $10 per day past the due date.
    ADD COLUMN late_fee_daily_cents bigint NOT NULL DEFAULT 0
        CHECK (late_fee_daily_cents >= 0),

    -- 'monthly' is the old rule (late_fee_cents, once per completed overdue
    -- month). 'daily' is late_fee_daily_cents per day. Existing rows default to
    -- monthly so nothing already billed changes meaning.
    ADD COLUMN late_fee_mode text NOT NULL DEFAULT 'monthly'
        CHECK (late_fee_mode IN ('monthly', 'daily')),

    -- Whether weekends and holidays are billable days. This school said yes;
    -- another may not, and the difference is real money, so it is data.
    ADD COLUMN surcharge_counts_all_days boolean NOT NULL DEFAULT true,

    -- A FIXED amount forgiven per month settled inside the window, not a
    -- percentage: "el pronto pago es una cantidad fija".
    ADD COLUMN early_payment_cents bigint NOT NULL DEFAULT 0
        CHECK (early_payment_cents >= 0),

    -- The window's width in WORKING days, counted from the start of the month
    -- being paid. Working days so a window ending on a Sunday does not cost a
    -- family the discount.
    ADD COLUMN early_payment_days int NOT NULL DEFAULT 0
        CHECK (early_payment_days >= 0 AND early_payment_days <= 31);

-- A daily plan with no rate charges nothing, which is almost certainly a
-- half-finished setup rather than a school that wants free credit. Caught here
-- instead of discovered when a parent is not billed.
ALTER TABLE tuition_plans
    ADD CONSTRAINT tuition_plans_daily_rate_ck
    CHECK (late_fee_mode <> 'daily' OR late_fee_daily_cents > 0);

COMMIT;
