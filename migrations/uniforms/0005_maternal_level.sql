-- uniforms (M16) — admit the Maternal level, matching students/0004.
--
-- Two CHECKs here: the catalogue item's optional level, and the level stamped on
-- an order line. Both listed the four SEP levels, so a maternal pupil could not
-- be sold a uniform even once students accepted them.
--
-- Additive: existing rows all satisfy the wider constraint.
BEGIN;

ALTER TABLE uniform_items DROP CONSTRAINT IF EXISTS uniform_items_level_check;
ALTER TABLE uniform_items ADD CONSTRAINT uniform_items_level_check
    CHECK (level IN ('maternal', 'kinder', 'primaria', 'secundaria', 'preparatoria'));

ALTER TABLE uniform_orders DROP CONSTRAINT IF EXISTS uniform_orders_student_level_check;
ALTER TABLE uniform_orders ADD CONSTRAINT uniform_orders_student_level_check
    CHECK (student_level IN ('maternal', 'kinder', 'primaria', 'secundaria', 'preparatoria'));

COMMIT;
