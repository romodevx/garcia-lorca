-- uniforms — the counter sells to a pupil with no cycle yet (task 10).
--
-- A pupil registered but not yet placed in a group has no cycle, and the office
-- sells them uniforms all the same (the owner's decision). finance charges such
-- a sale to the pupil with no cycle (finance/0017), so the order may name the
-- pupil and no cycle — at the counter only. A family or staff order for a child
-- still belongs to a school year, and a sale to nobody still claims none.

BEGIN;

ALTER TABLE uniform_orders DROP CONSTRAINT uniform_orders_student_cycle;

ALTER TABLE uniform_orders ADD CONSTRAINT uniform_orders_student_cycle CHECK (
    (student_id IS NULL AND cycle_id IS NULL)
    OR (student_id IS NOT NULL AND cycle_id IS NOT NULL)
    OR (student_id IS NOT NULL AND cycle_id IS NULL AND source = 'counter')
);

COMMIT;
