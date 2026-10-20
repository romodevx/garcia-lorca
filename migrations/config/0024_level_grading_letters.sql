-- config — a level may grade with letters instead of numbers (owner, 20-oct).
--
--   grading_letters  true ⇒ the level's teachers capture A (Excelente), B (Bien),
--                    C (Suficiente) or D (Requiere apoyo) per subject and period.
--                    Never together with grading_enabled (numbers).
--
-- Preescolar grades with letters; maternal takes no grades. This applies the
-- domain's rule (letterLevelNames) to the levels that already exist, with the
-- same folding as 0009. Additive only.
--
-- RLS: levels is FORCE ROW LEVEL SECURITY; SET LOCAL binds the one school this
-- deployment serves (same pattern as 0009).

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';
SET LOCAL lock_timeout = '5s';

ALTER TABLE levels
    ADD COLUMN grading_letters boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT levels_grading_numbers_or_letters CHECK (NOT (grading_enabled AND grading_letters));

UPDATE levels
   SET grading_letters = true
 WHERE NOT grading_enabled
   AND translate(lower(btrim(name) COLLATE "und-x-icu"), 'áéíóúü', 'aeiouu') IN ('preescolar', 'kinder');

COMMIT;
