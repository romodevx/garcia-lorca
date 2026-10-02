-- config — evaluation periods per grade, with their own dates (T14, ADR 0029).
--
-- WHY. Grades stop being captured month by month (T15): they are captured per
-- evaluation period (a trimester, a bimester). Until now a period was a TERM,
-- one per (cycle, level), mapped to months (0010). The school wants each grade
-- of a level able to keep its own calendar, with real start and end dates.
--
-- WHAT.
--   evaluation_periods (tenant, cycle, grade, ordinal, name, starts_on, ends_on)
--     * both dates inclusive, starts_on <= ends_on;
--     * UNIQUE (tenant, cycle, grade, ordinal);
--     * EXCLUDE USING gist: two periods of one grade in one cycle never share a
--       day, so not even two concurrent writers can make them overlap;
--     * term_id: the term a migrated row came from (provenance, NULL for rows
--       the school enters). Grades still close periods and issue report cards
--       per term; T15 maps a period to its term by (cycle, level, ordinal).
--   Neither constraint is DEFERRABLE: the level trash restores rows with
--   ON CONFLICT DO NOTHING, which refuses deferrable arbiters. To rewrite a
--   grade's whole set (shifting every period a week passes through overlapping
--   states) the repository deletes the grade's rows and inserts the new set
--   with the SAME ids, in one transaction.
--   That a period lies inside its cycle is checked by the use case.
--
-- DATA (additive; terms and term_months are kept untouched). For every term,
-- every grade of the term's level gets the same period: from the first day of
-- the term's first month to the last day of its last month, clamped to the
-- cycle; a term without months uses its own dates, clamped. Terms go in
-- ordinal order and one that would overlap a period already derived for that
-- grade is skipped with a NOTICE (months like {Sep, Nov} + {Oct} interleave):
-- the school enters it by hand. A level frozen by a running deletion (0012)
-- is skipped: its structure is on its way to the trash and the freeze trigger
-- would refuse the insert. Nothing stops the migration — this is a
-- catalog of dates, editable on the screen, not money or grades.
--
-- btree_gist (standard contrib: available on Cloud SQL, Neon and the
-- postgres:16 image) lets a GiST index compare uuids with =. CREATE EXTENSION
-- needs cloudsqlsuperuser / neon_superuser, which the migrator role has.
--
-- The level-deletion freeze (0012) also covers this table, and the trash
-- archives/restores it with the level (adapter).
--
-- RLS: ENABLE + FORCE + tenant_isolation, like every config table. The cloud
-- migrator owns the tables without BYPASSRLS, so SET LOCAL binds the one school
-- this deployment serves (same pattern as 0005/0014).

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';
SET LOCAL lock_timeout = '5s';

CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE evaluation_periods (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL,
    cycle_id       uuid NOT NULL REFERENCES cycles (id) ON DELETE CASCADE,
    grade_level_id uuid NOT NULL REFERENCES grade_levels (id) ON DELETE RESTRICT,
    ordinal        integer NOT NULL,
    name           text NOT NULL,
    starts_on      date NOT NULL,
    ends_on        date NOT NULL,
    term_id        uuid NULL REFERENCES terms (id) ON DELETE SET NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT evaluation_periods_ordinal_ck CHECK (ordinal BETWEEN 1 AND 12),
    CONSTRAINT evaluation_periods_name_ck CHECK (char_length(btrim(name)) BETWEEN 1 AND 60),
    CONSTRAINT evaluation_periods_dates_ck CHECK (starts_on <= ends_on),
    CONSTRAINT evaluation_periods_ordinal_key
        UNIQUE (tenant_id, cycle_id, grade_level_id, ordinal),
    CONSTRAINT evaluation_periods_no_overlap
        EXCLUDE USING gist (
            tenant_id WITH =,
            cycle_id WITH =,
            grade_level_id WITH =,
            daterange(starts_on, ends_on, '[]') WITH &&
        )
);

-- The FK RESTRICT check on grade_levels and the level trash read by grade;
-- ON DELETE SET NULL reads by term.
CREATE INDEX idx_evaluation_periods_grade ON evaluation_periods (grade_level_id);
CREATE INDEX idx_evaluation_periods_term ON evaluation_periods (term_id) WHERE term_id IS NOT NULL;

ALTER TABLE evaluation_periods ENABLE ROW LEVEL SECURITY;
ALTER TABLE evaluation_periods FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON evaluation_periods
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

-- The freeze of 0012, now also over evaluation_periods (resolved by grade).
CREATE OR REPLACE FUNCTION config_refuse_deleting_level() RETURNS trigger
    LANGUAGE plpgsql AS
$$
DECLARE
    lvl uuid;
BEGIN
    CASE TG_TABLE_NAME
        WHEN 'grade_levels', 'terms', 'term_months' THEN
            lvl := NEW.level_id;
        WHEN 'subjects', 'groups', 'evaluation_periods' THEN
            SELECT level_id INTO lvl FROM grade_levels WHERE id = NEW.grade_level_id;
        WHEN 'teaching_assignments' THEN
            SELECT gl.level_id INTO lvl FROM groups g JOIN grade_levels gl ON gl.id = g.grade_level_id
             WHERE g.id = NEW.group_id;
        WHEN 'school_events' THEN
            lvl := NEW.level_id;
            IF lvl IS NULL AND NEW.group_id IS NOT NULL THEN
                SELECT gl.level_id INTO lvl FROM groups g JOIN grade_levels gl ON gl.id = g.grade_level_id
                 WHERE g.id = NEW.group_id;
            END IF;
        ELSE
            RETURN NEW;
    END CASE;
    IF lvl IS NOT NULL AND config_level_is_deleting(lvl) THEN
        RAISE EXCEPTION 'LEVEL_BEING_DELETED' USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END
$$;

CREATE TRIGGER evaluation_periods_level_not_deleting BEFORE INSERT OR UPDATE ON evaluation_periods
    FOR EACH ROW EXECUTE FUNCTION config_refuse_deleting_level();

-- Data: one period per (term, grade of its level), in ordinal order, skipping
-- what would overlap. A loop, not one INSERT … SELECT: whether a row fits
-- depends on the rows inserted before it.
DO $$
DECLARE
    candidate record;
BEGIN
    FOR candidate IN
        SELECT t.id AS term_id, t.tenant_id, t.cycle_id, g.id AS grade_level_id, t.ordinal,
               COALESCE(NULLIF(left(btrim(t.name), 60), ''), 'Periodo ' || t.ordinal) AS name,
               GREATEST(COALESCE(m.first_month, t.starts_on), c.starts_on) AS starts_on,
               LEAST(COALESCE((m.last_month + interval '1 month' - interval '1 day')::date, t.ends_on), c.ends_on) AS ends_on
          FROM terms t
          JOIN cycles c ON c.id = t.cycle_id
          JOIN levels l ON l.id = t.level_id AND l.status <> 'deleting'
          JOIN grade_levels g ON g.level_id = t.level_id AND g.tenant_id = t.tenant_id
          LEFT JOIN (SELECT term_id, min(month) AS first_month, max(month) AS last_month
                       FROM term_months GROUP BY term_id) m ON m.term_id = t.id
         WHERE t.ordinal BETWEEN 1 AND 12
         ORDER BY t.tenant_id, t.cycle_id, g.id, t.ordinal
    LOOP
        IF candidate.starts_on > candidate.ends_on THEN
            RAISE NOTICE 'config 0016: term % has no days inside its cycle; grade % gets no period for it',
                candidate.term_id, candidate.grade_level_id;
        ELSIF EXISTS (
            SELECT 1 FROM evaluation_periods p
             WHERE p.tenant_id = candidate.tenant_id
               AND p.cycle_id = candidate.cycle_id
               AND p.grade_level_id = candidate.grade_level_id
               AND daterange(p.starts_on, p.ends_on, '[]') && daterange(candidate.starts_on, candidate.ends_on, '[]')
        ) THEN
            RAISE NOTICE 'config 0016: term % overlaps an earlier period of grade %; enter it by hand',
                candidate.term_id, candidate.grade_level_id;
        ELSE
            INSERT INTO evaluation_periods (tenant_id, cycle_id, grade_level_id, ordinal, name, starts_on, ends_on, term_id)
            VALUES (candidate.tenant_id, candidate.cycle_id, candidate.grade_level_id, candidate.ordinal,
                    candidate.name, candidate.starts_on, candidate.ends_on, candidate.term_id);
        END IF;
    END LOOP;
END
$$;

COMMIT;
