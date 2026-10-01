-- finance (T5) — el rango de fechas de un concepto: desde cuándo y hasta cuándo.
--
-- An inscription opens on a day and falls due on another. The deadline already
-- existed (enrollment_due_on, 0013); the opening day did not, so a charge was
-- shown to the family — and asked for — from the moment the run issued it.
--
-- 1. tuition_plans.enrollment_starts_on: the first day the inscription opens to
--    families. NULL keeps it open from issue, which is what every plan written
--    before today means. It needs a deadline and may not come after it — the
--    same rule as domain.ValidateChargeWindow, enforced here too so a writer
--    that skipped the domain cannot store a window nobody could mean.
--
-- 2. charges.starts_on: the opening day each charge was issued (or moved) with.
--    The family statement hides a charge until it arrives; the counter lists it
--    but does not ask for it. NULL is every charge issued before today.
--
-- Additive only: two nullable columns and their CHECKs. No row is rewritten.
-- Both tables already carry ENABLE + FORCE RLS and tenant_isolation (0001/0005).
BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE tuition_plans ADD COLUMN enrollment_starts_on date;

ALTER TABLE tuition_plans
    ADD CONSTRAINT tuition_plans_enrollment_window_ck
        CHECK (enrollment_starts_on IS NULL
               OR (enrollment_due_on IS NOT NULL AND enrollment_starts_on <= enrollment_due_on));

ALTER TABLE charges ADD COLUMN starts_on date;

ALTER TABLE charges
    ADD CONSTRAINT charges_window_ck
        CHECK (starts_on IS NULL OR starts_on <= due_date);

COMMIT;
