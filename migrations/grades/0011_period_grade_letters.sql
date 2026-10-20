-- grades — a period grade may be a letter (preescolar, owner 20-oct).
--
--   period_grades.letter   A (Excelente), B (Bien), C (Suficiente) or
--                          D (Requiere apoyo); a cell holds a number OR a
--                          letter, exactly one. value becomes nullable for it;
--                          its range and whole-number CHECKs pass on NULL.
--   report_cards.grading_letters
--                          the card was issued for a level that grades with
--                          letters: its lines' period_values hold letters and
--                          it has no averages.
--
-- grading_letters is NULLABLE on purpose: the level trash (0007) restores rows
-- with jsonb_populate_record, where a key missing from an older archive comes
-- back NULL. NULL reads as "numbers", which is what such a card was.
--
-- RLS: both tables are ENABLE + FORCE with the tenant policy since 0003/0008;
-- new columns fall under it, and the table-level grants cover them.

BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE period_grades
    ALTER COLUMN value DROP NOT NULL,
    ADD COLUMN letter text CHECK (letter IN ('A', 'B', 'C', 'D')),
    ADD CONSTRAINT period_grades_value_or_letter CHECK ((value IS NULL) <> (letter IS NULL));

ALTER TABLE report_cards
    ADD COLUMN grading_letters boolean;

COMMIT;
