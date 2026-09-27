-- config — each level's numeric grading scale (T6, calificaciones mensuales).
--
--   grading_enabled  false ⇒ the level takes no numeric capture (early years;
--                    qualitative assessment comes later).
--   grade_min/max    the inclusive range a mark may take.
--   grade_decimals   how many decimal places a mark may carry (0..2).
--   grade_passing    the lowest passing mark.
--
-- Defaults are the school's rule: 5 to 10, one decimal, 6 to pass. The columns
-- are numeric(5,2), not (4,2): the CHECK allows a 0–100 scale and numeric(4,2)
-- tops out at 99.99.
--
-- New levels get their default from the domain (DefaultGradingScale); this
-- migration applies the SAME rule to the levels that already exist: numeric
-- capture off when the trimmed, lowercased, accent-free name is exactly
-- preescolar, kinder or maternal. Same folding as 0008 (ICU lower + translate).
-- Additive only: no row is removed.
--
-- RLS: levels is FORCE ROW LEVEL SECURITY and the cloud migrator owns it without
-- BYPASSRLS, so SET LOCAL binds the one school this deployment serves (same
-- pattern as 0008).

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';
SET LOCAL lock_timeout = '5s';

ALTER TABLE levels
    ADD COLUMN grading_enabled boolean      NOT NULL DEFAULT true,
    ADD COLUMN grade_min       numeric(5,2) NOT NULL DEFAULT 5,
    ADD COLUMN grade_max       numeric(5,2) NOT NULL DEFAULT 10,
    ADD COLUMN grade_decimals  smallint     NOT NULL DEFAULT 1,
    ADD COLUMN grade_passing   numeric(5,2) NOT NULL DEFAULT 6,
    ADD CONSTRAINT levels_grade_decimals_range CHECK (grade_decimals BETWEEN 0 AND 2),
    ADD CONSTRAINT levels_grade_scale_order
        CHECK (grade_min >= 0 AND grade_min < grade_passing AND grade_passing <= grade_max AND grade_max <= 100);

UPDATE levels
   SET grading_enabled = false
 WHERE translate(lower(btrim(name) COLLATE "und-x-icu"), 'áéíóúü', 'aeiouu')
       IN ('preescolar', 'kinder', 'maternal');

COMMIT;
