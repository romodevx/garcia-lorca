-- config — a teaching assignment's subject belongs to its group's grade (T13).
--
-- WHY. Subjects are per grade (0001: subjects.grade_level_id NOT NULL) and a
-- teacher is assigned to a (group, subject). The use case already refuses a
-- subject of another grade (CreateTeachingAssignment, AssignTeachingLoad), but
-- only there: a script, a restore or a future code path could write
-- "Matemáticas de 2°" into a 1° group. T16 builds the timetable on these rows,
-- so the database now keeps the rule too.
--
-- HOW. A trigger, not a composite FK: a composite FK would need a copy of the
-- grade on every assignment, and the level-deletion trash (0011) archives and
-- restores assignments as JSONB rows — a new column there is a restore format
-- change. The trigger checks the same thing with no new column:
--   * teaching_assignments INSERT / UPDATE OF group_id, subject_id: the group and
--     the subject exist in the assignment's tenant and share grade_level_id;
--   * subjects / groups UPDATE OF grade_level_id: a row cannot change grade
--     while an assignment pairs it with the other side's old grade.
-- Both raise check_violation with CONSTRAINT teaching_assignments_subject_of_group_grade
-- so the adapter maps it to KindInvalid.
--
-- If an existing assignment already breaks the rule the migration STOPS, names
-- the rows by id and changes nothing (the count on the local DB at T13 was 0).
--
-- RLS: every table involved is FORCE ROW LEVEL SECURITY. The functions run as
-- the invoker, under the caller's tenant, and also compare tenant_id explicitly.
-- The cloud migrator owns the tables without BYPASSRLS, so SET LOCAL binds the
-- one school this deployment serves (same pattern as 0006). Additive only.

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';
SET LOCAL lock_timeout = '5s';

DO $$
DECLARE
    broken text;
BEGIN
    SELECT string_agg(ta.id::text, ', ' ORDER BY ta.id)
      INTO broken
      FROM teaching_assignments ta
      JOIN groups g   ON g.id = ta.group_id
      JOIN subjects s ON s.id = ta.subject_id
     WHERE g.grade_level_id <> s.grade_level_id
        OR g.tenant_id <> ta.tenant_id
        OR s.tenant_id <> ta.tenant_id;
    IF broken IS NOT NULL THEN
        RAISE EXCEPTION 'config: teaching assignments pair a group with a subject of another grade; nothing was changed. Resolve these rows by hand and re-run: %', broken
            USING ERRCODE = 'check_violation';
    END IF;
END
$$;

CREATE FUNCTION config_assignment_subject_of_group_grade() RETURNS trigger
    LANGUAGE plpgsql AS
$$
BEGIN
    IF NOT EXISTS (
        SELECT 1
          FROM groups g
          JOIN subjects s ON s.grade_level_id = g.grade_level_id
         WHERE g.id = NEW.group_id AND g.tenant_id = NEW.tenant_id
           AND s.id = NEW.subject_id AND s.tenant_id = NEW.tenant_id
    ) THEN
        RAISE EXCEPTION 'subject does not belong to the group''s grade level'
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'teaching_assignments_subject_of_group_grade';
    END IF;
    RETURN NEW;
END
$$;

CREATE TRIGGER teaching_assignments_subject_of_group_grade
    BEFORE INSERT OR UPDATE OF group_id, subject_id, tenant_id ON teaching_assignments
    FOR EACH ROW EXECUTE FUNCTION config_assignment_subject_of_group_grade();

-- A subject or group keeps its grade while an assignment pairs it. One function
-- per table, so each lookup is a plain equality the index on subject_id /
-- group_id serves.
CREATE FUNCTION config_keep_assigned_subject_grade() RETURNS trigger
    LANGUAGE plpgsql AS
$$
BEGIN
    IF NEW.grade_level_id IS DISTINCT FROM OLD.grade_level_id AND EXISTS (
        SELECT 1 FROM teaching_assignments ta
         WHERE ta.subject_id = OLD.id AND ta.tenant_id = OLD.tenant_id
    ) THEN
        RAISE EXCEPTION 'the subject is in a teaching assignment; its grade level cannot change'
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'teaching_assignments_subject_of_group_grade';
    END IF;
    RETURN NEW;
END
$$;

CREATE FUNCTION config_keep_assigned_group_grade() RETURNS trigger
    LANGUAGE plpgsql AS
$$
BEGIN
    IF NEW.grade_level_id IS DISTINCT FROM OLD.grade_level_id AND EXISTS (
        SELECT 1 FROM teaching_assignments ta
         WHERE ta.group_id = OLD.id AND ta.tenant_id = OLD.tenant_id
    ) THEN
        RAISE EXCEPTION 'the group is in a teaching assignment; its grade level cannot change'
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'teaching_assignments_subject_of_group_grade';
    END IF;
    RETURN NEW;
END
$$;

CREATE TRIGGER subjects_keep_assigned_grade
    BEFORE UPDATE OF grade_level_id ON subjects
    FOR EACH ROW EXECUTE FUNCTION config_keep_assigned_subject_grade();

CREATE TRIGGER groups_keep_assigned_grade
    BEFORE UPDATE OF grade_level_id ON groups
    FOR EACH ROW EXECUTE FUNCTION config_keep_assigned_group_grade();

COMMIT;
