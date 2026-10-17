-- config — a new level grades in whole numbers (T10, QA ISSUE-011).
--
-- Owner decision: grades are WHOLE NUMBERS only ("sólo enteros"). The domain
-- (DefaultGradingScale) already gives a new level grade_decimals = 0; this
-- aligns the column default from 0009 (1) with it, so no path that inserts a
-- level without naming its decimals can start one at "1 decimal".
--
-- Existing levels are NOT touched: their decimals are the school's choice.
-- Catalog-only DDL: no row is read or written, so no tenant binding is needed
-- and RLS (ENABLE + FORCE on levels) is unchanged.

BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE levels ALTER COLUMN grade_decimals SET DEFAULT 0;

COMMIT;
