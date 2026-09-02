-- finance (M17) — la colegiatura dice el mes con letra.
--
-- "Colegiatura 2026-09" is a machine's idea of a month. The school reads these
-- rows out to parents on the phone and prints them on statements, and nobody on
-- the other end of that call is holding a calendar to decode 09.
--
-- The billing run now names new charges "Colegiatura septiembre 2026". Existing
-- ones would keep the old name forever: the run is idempotent by external_ref,
-- so it never touches a charge it already issued. Without this backfill the
-- school would read a mix of both for a year and the change would look like it
-- had not been made.
--
-- Safe to run because the description is DISPLAY. Identity is external_ref and
-- the due date, neither of which is touched here, so nothing is re-issued, no
-- payment moves, and no balance changes.
BEGIN;

UPDATE charges
SET description = 'Colegiatura '
    || (ARRAY['enero','febrero','marzo','abril','mayo','junio',
              'julio','agosto','septiembre','octubre','noviembre','diciembre'])
       [substring(description from 18 for 2)::int]
    || ' ' || substring(description from 13 for 4)
WHERE description ~ '^Colegiatura \d{4}-(0[1-9]|1[0-2])$';

COMMIT;
