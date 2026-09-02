-- finance (M17) — las cuotas extra y la fecha límite de la inscripción.
--
-- Two things the school asked for, in one migration because they arrive in the
-- same drawer and the same write.
--
-- 1. FEES. A school's year and its calendar do not agree: "la cuota de julio"
--    is charged for a month the year does not bill, so it is collected inside
--    months that it does. Saying that used to mean hand-writing a schedule of
--    month:amount pairs, which folded the fee into the tuition price and left
--    nobody able to explain either number afterwards.
--
--    JSONB and not a child table, for the same reasons as `schedule` (0010):
--    the list is small, owned entirely by its plan, always read whole and always
--    written whole. Nobody will ask "which levels collect a fee in March".
--
-- 2. enrollment_due_on. The inscription is its own charge and the school sets
--    its own deadline for it — collected in June for a year that opens in
--    August, which deriving the date from the cycle's first month made
--    impossible to say. NULL keeps the old rule, so every plan written before
--    today goes on meaning what it meant.
BEGIN;

ALTER TABLE tuition_plans ADD COLUMN fees jsonb NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE tuition_plans ADD CONSTRAINT tuition_plans_fees_is_a_list_ck
    CHECK (jsonb_typeof(fees) = 'array');

ALTER TABLE tuition_plans ADD COLUMN enrollment_due_on date;

-- The billing run needs a concept to hang fee charges on, and it finds its
-- concepts by kind (0004).
ALTER TABLE charge_concepts DROP CONSTRAINT IF EXISTS charge_concepts_kind_check;

ALTER TABLE charge_concepts
    ADD CONSTRAINT charge_concepts_kind_check
        CHECK (kind IN ('custom', 'tuition', 'enrollment', 'late_fee', 'fee'));

COMMIT;
