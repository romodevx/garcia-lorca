-- students — a contact may be marked as whom to call first (task 5).
--
-- The office keeps several contacts per pupil; the emergency mark says which
-- ones to try first when something happens. Additive: existing contacts are
-- not marked. RLS is the table's own (0003).

BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE guardian_students ADD COLUMN is_emergency boolean NOT NULL DEFAULT false;

COMMIT;
