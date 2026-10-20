-- config — a level may grade with letters instead of numbers (owner, 20-oct).
--
--   grading_letters  true ⇒ the level's teachers capture A (Excelente), B (Bien),
--                    C (Suficiente) or D (Requiere apoyo) per subject and period.
--                    Never together with grading_enabled (numbers).
--
-- Preescolar grades with letters; maternal takes no grades. This applies the
-- domain's rule (letterLevelNames) to the levels that already exist, folded
-- with config_fold_name (0019). Additive only.
--
-- LEVEL TRASH. An archived level is rebuilt from its JSONB with
-- jsonb_populate_record (ADR 0021), where a missing key comes back NULL: a
-- level trashed before this migration would break the NOT NULL and roll its
-- restore back. Every archived level gets the key here, by the same rule
-- (same fix as 0020's sort_order).
--
-- RLS: levels is FORCE ROW LEVEL SECURITY; SET LOCAL binds the one school this
-- deployment serves (same pattern as 0009).

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';
SET LOCAL lock_timeout = '5s';

ALTER TABLE levels
    ADD COLUMN grading_letters boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT levels_grading_numbers_or_letters CHECK (NOT (grading_enabled AND grading_letters));

-- A level mid-deletion (status 'deleting', ADR 0021) is skipped: the freeze
-- trigger refuses any change to it, and its rows reach the archive, which the
-- next statement completes.
UPDATE levels
   SET grading_letters = true
 WHERE NOT grading_enabled
   AND status <> 'deleting'
   AND config_fold_name(name) IN ('preescolar', 'kinder');

UPDATE config_deletion_archive a
   SET "row" = a."row" || jsonb_build_object('grading_letters',
       COALESCE((a."row"->>'grading_enabled')::boolean, true) = false
       AND config_fold_name(COALESCE(a."row"->>'name', '')) IN ('preescolar', 'kinder'))
 WHERE a.table_name = 'levels' AND NOT (a."row" ? 'grading_letters');

COMMIT;
