-- config — what makes a subject the same subject, and when it may go (T13).
--
-- 1. One name per grade whatever the capitalization. The web captures catalog
--    names in uppercase and older rows (the demo seed) are mixed case, so
--    "Matemáticas" and "MATEMÁTICAS" could sit side by side in one grade: two
--    rows that look like one, split in two in the teacher's load. Same rule as
--    grade names (0006), but folded with the ICU collation: lower() under the
--    database's own ctype leaves Á and Ñ alone on a C-locale cluster, and
--    "ESPAÑOL" would slip past "Español". If a grade already repeats a name in another case the
--    migration STOPS, names the rows by id and changes nothing: which one holds
--    grades is the school's call.
--
-- 2. subjects.first_assigned_at: when the subject first entered a teacher's
--    load. Grades, monthly grades, report cards and per-class attendance (other
--    services) keep the subject id after the slot is freed, so a subject that
--    was EVER assigned cannot be deleted — only renamed. A trigger stamps it on
--    every teaching_assignments insert / subject change; every subject that
--    exists before this migration is backfilled as used (its history is not
--    visible from config).
--
-- RLS: subjects and teaching_assignments are FORCE ROW LEVEL SECURITY; the
-- trigger runs as the invoker under the caller's tenant. SET LOCAL binds the one
-- school this deployment serves for the backfill (same pattern as 0006).
-- Additive only.

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';
SET LOCAL lock_timeout = '5s';

DO $$
DECLARE
    clashes text;
BEGIN
    SELECT string_agg(id::text, ', ' ORDER BY grade_level_id, lower(name COLLATE "und-x-icu"), id)
      INTO clashes
      FROM (SELECT id, grade_level_id, name,
                   count(*) OVER (PARTITION BY tenant_id, grade_level_id, lower(name COLLATE "und-x-icu")) AS spellings
              FROM subjects) s
     WHERE spellings > 1;
    IF clashes IS NOT NULL THEN
        RAISE EXCEPTION 'config: subject names repeat within a grade; nothing was changed. Resolve these rows by hand and re-run: %', clashes
            USING ERRCODE = 'unique_violation';
    END IF;
END
$$;

CREATE UNIQUE INDEX subjects_name_per_grade_key
    ON subjects (tenant_id, grade_level_id, lower(name COLLATE "und-x-icu"));

ALTER TABLE subjects ADD COLUMN first_assigned_at timestamptz;

-- Backfill: EVERY subject that exists today counts as used. A slot freed before
-- this migration leaves no assignment behind, but its grades and attendance
-- (other services) still carry the subject id, and config cannot see them. So
-- the past is assumed used: an old subject can be renamed, not deleted. The
-- date is the first assignment when there is one, else the subject's creation.
UPDATE subjects s
   SET first_assigned_at = COALESCE(
       (SELECT min(ta.created_at) FROM teaching_assignments ta
         WHERE ta.subject_id = s.id AND ta.tenant_id = s.tenant_id),
       s.created_at);

CREATE FUNCTION config_mark_subject_assigned() RETURNS trigger
    LANGUAGE plpgsql AS
$$
BEGIN
    UPDATE subjects
       SET first_assigned_at = now()
     WHERE id = NEW.subject_id AND tenant_id = NEW.tenant_id AND first_assigned_at IS NULL;
    RETURN NEW;
END
$$;

CREATE TRIGGER teaching_assignments_mark_subject_assigned
    AFTER INSERT OR UPDATE OF subject_id ON teaching_assignments
    FOR EACH ROW EXECUTE FUNCTION config_mark_subject_assigned();

COMMIT;
