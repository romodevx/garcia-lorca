-- grades — level-trash rows archived before 0008 restore again (T15 review).
--
-- WHY. 0008 added report_cards.periods and report_card_lines.period_values as
-- NOT NULL with defaults. The level trash (0007) restores a row by rebuilding
-- it from its archived JSONB, where a key the row did not have comes back
-- NULL — so a level trashed before 0008 could no longer be restored (NOT NULL
-- violation, the whole restore rolls back).
--
-- WHAT. Give every archived report card / line that lacks the new key the
-- column's default ('[]' / '{}'). Idempotent (only rows without the key),
-- touches nothing outside the archive, safe on an empty database.
--
-- RLS: the archive is FORCE-RLS; the cloud migrator owns it without
-- BYPASSRLS, so SET LOCAL binds the one school this deployment serves (same
-- pattern as config/0005, 0014, 0016).

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';
SET LOCAL lock_timeout = '5s';

UPDATE grades_deletion_archive
   SET "row" = "row" || '{"periods": []}'::jsonb
 WHERE table_name = 'report_cards' AND NOT ("row" ? 'periods');

UPDATE grades_deletion_archive
   SET "row" = "row" || '{"period_values": {}}'::jsonb
 WHERE table_name = 'report_card_lines' AND NOT ("row" ? 'period_values');

COMMIT;
