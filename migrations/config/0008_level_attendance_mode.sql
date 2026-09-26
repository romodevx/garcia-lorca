-- config — how each level takes attendance (T3, asistencia por lista).
--
-- attendance_mode:
--   daily      one pass list per group per day; any teacher of the group may take
--              or correct it (Preescolar, Primaria: a homeroom teacher).
--   per_class  one pass list per group, subject and day; only that subject's
--              teacher takes it (Secundaria, Preparatoria: a teacher per subject).
--
-- New levels get their default from the domain (DefaultAttendanceMode); this
-- migration applies the SAME rule to the levels that already exist: per_class
-- when the trimmed, lowercased, accent-free name is exactly secundaria,
-- preparatoria or bachillerato; daily otherwise. The ICU collation pins lower()
-- to Unicode case mapping (under a C ctype lower() folds ASCII only, so
-- "SECUNDÁRIA" would keep its capital Á), and translate() strips the accents
-- without needing the unaccent extension. Additive only: no row is removed.
--
-- RLS: levels is FORCE ROW LEVEL SECURITY and the cloud migrator owns it without
-- BYPASSRLS, so SET LOCAL binds the one school this deployment serves (same
-- pattern as 0005 and migrations/iam/0017).

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';
SET LOCAL lock_timeout = '5s';

ALTER TABLE levels
    ADD COLUMN attendance_mode text NOT NULL DEFAULT 'daily'
        CHECK (attendance_mode IN ('daily', 'per_class'));

UPDATE levels
   SET attendance_mode = 'per_class'
 WHERE translate(lower(btrim(name) COLLATE "und-x-icu"), 'áéíóúü', 'aeiouu')
       IN ('secundaria', 'preparatoria', 'bachillerato');

COMMIT;
