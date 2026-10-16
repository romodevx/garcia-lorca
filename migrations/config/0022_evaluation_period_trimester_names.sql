-- config — evaluation periods take the trimester name (T10, QA ISSUE-020).
--
-- WHY. One thing had two names: the month calendar says "1.er trimestre" while
-- the evaluation period (and so the report card and its notification, which
-- copy the period's name) said "PERIODO 1". The screen's prefill now proposes
-- the trimester name; this renames the rows that still carry a SYSTEM name.
--
-- RULE (a row is renamed only when all hold):
--   * its name is exactly 'PERIODO <ordinal>' (the screen's old prefill) or
--     'Periodo <ordinal>' (0016's fallback): text the school never typed;
--   * the number is its own ordinal — a row renumbered by date since then
--     ("PERIODO 4" at ordinal 3) is ambiguous and is left alone;
--   * its ordinal is 1..3 and its grade has no period beyond the 3rd in that
--     cycle: four or more periods are not trimesters;
--   * its level is not being deleted (the 0012/0016 freeze would refuse it).
-- New names: 1 → '1.er trimestre', 2 → '2.º trimestre', 3 → '3.er trimestre'
-- (the same text as the month calendar's terms).
--
-- Report cards already issued are snapshots and keep the name they were
-- issued with; the next close copies the new one.
--
-- RLS: evaluation_periods is FORCE ROW LEVEL SECURITY and the cloud migrator
-- owns it without BYPASSRLS, so SET LOCAL binds the one school this
-- deployment serves (same pattern as 0016). Policies are unchanged.

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';
SET LOCAL lock_timeout = '5s';

UPDATE evaluation_periods p
   SET name = CASE p.ordinal
                  WHEN 1 THEN '1.er trimestre'
                  WHEN 2 THEN '2.º trimestre'
                  WHEN 3 THEN '3.er trimestre'
              END,
       updated_at = now()
  FROM grade_levels g
  JOIN levels l ON l.id = g.level_id
 WHERE g.id = p.grade_level_id
   AND l.status <> 'deleting'
   AND p.ordinal BETWEEN 1 AND 3
   AND p.name IN ('PERIODO ' || p.ordinal, 'Periodo ' || p.ordinal)
   AND NOT EXISTS (
       SELECT 1 FROM evaluation_periods o
        WHERE o.tenant_id = p.tenant_id
          AND o.cycle_id = p.cycle_id
          AND o.grade_level_id = p.grade_level_id
          AND o.ordinal > 3
   );

COMMIT;
