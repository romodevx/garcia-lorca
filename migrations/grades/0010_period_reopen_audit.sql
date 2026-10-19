-- grades — who reopened a period, and why (CC-308).
--
-- A reopen unlocks a closed term so its grades can change after the report
-- cards went out. period_closures kept only WHEN (reopened_at); who did it and
-- why lived in a log line. The reason is now required at the edge and stored
-- here with the actor, beside closed_by / closed_by_name, for the latest
-- reopen of the term.
--
-- Nullable: a term never reopened (or reopened before this migration) has
-- none. The level trash (0007) restores rows from their JSONB archive, where a
-- missing key comes back NULL, so an older archive restores unchanged.
--
-- RLS: period_closures is ENABLE + FORCE with the tenant policy since 0003; a
-- new column falls under it, and the table-level grants cover it.

BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE period_closures
    ADD COLUMN IF NOT EXISTS reopened_by      uuid NULL,  -- iam user who last reopened it
    ADD COLUMN IF NOT EXISTS reopened_by_name text NULL
        CHECK (reopened_by_name IS NULL OR length(reopened_by_name) <= 200),
    ADD COLUMN IF NOT EXISTS reopen_reason    text NULL
        CHECK (reopen_reason IS NULL OR length(btrim(reopen_reason)) BETWEEN 1 AND 500);

COMMIT;
