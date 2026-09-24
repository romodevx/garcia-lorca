-- config — a grade's name is unique within its level.
--
-- WHY. Removing the SEP rules (task 2, ADR 0014) keeps the basic integrity
-- checks, and one was missing: grade_levels was unique by (level, ordinal) but
-- not by name, so "1° Preescolar" could be created twice at orders 1 and 2 and
-- a pupil placed in either. The name is compared case-insensitively — the web
-- captures it in uppercase, an API client might not.
--
-- If existing rows already repeat a name within a level, the migration STOPS
-- and changes nothing, naming the rows by id: which one is the real grade is
-- the school's call, and a grade may already hold pupils. Nothing is deleted.
--
-- RLS: grade_levels is FORCE ROW LEVEL SECURITY and the cloud migrator owns it
-- without BYPASSRLS, so SET LOCAL binds the one school this deployment serves
-- (same pattern as migrations/iam/0017). The unique index covers every row.

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';
SET LOCAL lock_timeout = '5s';

DO $$
DECLARE
    clashes text;
BEGIN
    SELECT string_agg(id::text, ', ' ORDER BY level_id, lower(name), id)
      INTO clashes
      FROM (SELECT id, level_id, name,
                   count(*) OVER (PARTITION BY tenant_id, level_id, lower(name)) AS spellings
              FROM grade_levels) g
     WHERE spellings > 1;
    IF clashes IS NOT NULL THEN
        RAISE EXCEPTION 'config: grade names repeat within a level; nothing was changed. Resolve these rows by hand and re-run: %', clashes
            USING ERRCODE = 'unique_violation';
    END IF;
END
$$;

CREATE UNIQUE INDEX grade_levels_name_per_level_key
    ON grade_levels (tenant_id, level_id, lower(name));

COMMIT;
