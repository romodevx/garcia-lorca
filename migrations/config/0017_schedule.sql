-- config — the school timetable (T16, ADR 0023).
--
-- WHY. The school wants its weekly timetable in the system — Monday to Friday,
-- per group and per teacher — and a way to change one day without touching
-- the base ("the music teacher is late; another free teacher takes that block
-- today"). Classes are built ONLY from the cycle's teaching assignments (T13):
-- a class is a (teacher, group, subject) the school already assigned, placed on
-- a weekday and a time.
--
-- WHAT.
--   schedule_settings (tenant, level, grid_start, grid_end)
--     the hours the calendar shows per level; no row = 07:00–15:00.
--   schedule_blocks (tenant, cycle, assignment, group, subject, teacher,
--                    weekday 1–5, starts_at, ends_at)
--     times are minutes since midnight in 5-minute steps; a class is the
--     half-open range [starts_at, ends_at), so back-to-back classes do not
--     collide. group/subject/teacher are the ASSIGNMENT's, tied to it by a
--     composite FK (ON UPDATE CASCADE: reassigning the assignment's teacher
--     moves the class to the new teacher — and the EXCLUDE below refuses it
--     if that teacher is already busy then; ON DELETE CASCADE: freeing the
--     slot removes its classes).
--     Two EXCLUDE constraints (btree_gist, enabled by 0016):
--       * a teacher is never in two groups at the same time;
--       * a group never has two classes at the same time.
--     Two editors racing cannot double-book: the database refuses the second.
--   schedule_overrides (tenant, cycle, block, on_date, kind, teacher,
--                       swap_block, reason, created_by…)
--     ONE class changed on ONE date: substitute (teacher), swap (trades its
--     time with swap_block of the same group that day) or cancel. The base is
--     never touched. One change per class and date. The conflict rules on the
--     EFFECTIVE timetable of a date (base + its changes) cannot be an EXCLUDE
--     (they are computed), so config evaluates them under a per-cycle advisory
--     lock that serialises every timetable write.
--   schedule_events — the history: who, when, what, why (every change, and
--     every base edit made once the cycle has started). No FK to blocks or
--     overrides: the history outlives what it describes.
--
-- Neither EXCLUDE is DEFERRABLE: the level trash restores rows with
-- ON CONFLICT DO NOTHING, which refuses deferrable arbiters (as 0016).
-- The level-deletion freeze (0012) covers the new tables, and the trash
-- archives/restores them with the level (adapter).
--
-- RLS: ENABLE + FORCE + tenant_isolation on all four, like every config table.
-- Additive only: no data is created or transformed.

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';
SET LOCAL lock_timeout = '5s';

-- The composite FK target: an assignment as a whole (id is the PK, so this is
-- trivially unique; Postgres needs the constraint to reference the columns).
ALTER TABLE teaching_assignments
    ADD CONSTRAINT teaching_assignments_schedule_key
        UNIQUE (id, tenant_id, cycle_id, group_id, subject_id, teacher_id);

CREATE TABLE schedule_settings (
    tenant_id  uuid NOT NULL,
    level_id   uuid NOT NULL REFERENCES levels (id) ON DELETE CASCADE,
    grid_start integer NOT NULL,
    grid_end   integer NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, level_id),
    CONSTRAINT schedule_settings_grid_ck CHECK (
        grid_start >= 0 AND grid_end <= 1440 AND grid_start % 5 = 0 AND grid_end % 5 = 0
        AND grid_end - grid_start >= 60)
);
CREATE INDEX idx_schedule_settings_level ON schedule_settings (level_id);

CREATE TABLE schedule_blocks (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL,
    cycle_id      uuid NOT NULL REFERENCES cycles (id) ON DELETE CASCADE,
    assignment_id uuid NOT NULL,
    group_id      uuid NOT NULL,
    subject_id    uuid NOT NULL,
    teacher_id    uuid NOT NULL,
    weekday       integer NOT NULL,
    starts_at     integer NOT NULL,
    ends_at       integer NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT schedule_blocks_assignment_fk
        FOREIGN KEY (assignment_id, tenant_id, cycle_id, group_id, subject_id, teacher_id)
        REFERENCES teaching_assignments (id, tenant_id, cycle_id, group_id, subject_id, teacher_id)
        ON UPDATE CASCADE ON DELETE CASCADE,
    CONSTRAINT schedule_blocks_weekday_ck CHECK (weekday BETWEEN 1 AND 5),
    CONSTRAINT schedule_blocks_time_ck CHECK (
        starts_at >= 0 AND ends_at <= 1440 AND starts_at < ends_at
        AND starts_at % 5 = 0 AND ends_at % 5 = 0),
    CONSTRAINT schedule_blocks_teacher_free
        EXCLUDE USING gist (
            tenant_id WITH =,
            cycle_id WITH =,
            teacher_id WITH =,
            weekday WITH =,
            int4range(starts_at, ends_at) WITH &&
        ),
    CONSTRAINT schedule_blocks_group_free
        EXCLUDE USING gist (
            tenant_id WITH =,
            cycle_id WITH =,
            group_id WITH =,
            weekday WITH =,
            int4range(starts_at, ends_at) WITH &&
        )
);
-- The FK checks and the level trash read by assignment and by group.
CREATE INDEX idx_schedule_blocks_assignment ON schedule_blocks (assignment_id);
CREATE INDEX idx_schedule_blocks_group ON schedule_blocks (group_id);
CREATE INDEX idx_schedule_blocks_cycle ON schedule_blocks (tenant_id, cycle_id, weekday);

CREATE TABLE schedule_overrides (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL,
    cycle_id        uuid NOT NULL REFERENCES cycles (id) ON DELETE CASCADE,
    block_id        uuid NOT NULL REFERENCES schedule_blocks (id) ON DELETE CASCADE,
    on_date         date NOT NULL,
    kind            text NOT NULL,
    teacher_id      uuid NULL REFERENCES teachers (id) ON DELETE RESTRICT,
    swap_block_id   uuid NULL REFERENCES schedule_blocks (id) ON DELETE CASCADE,
    reason          text NOT NULL,
    created_by      text NOT NULL,
    created_by_name text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT schedule_overrides_kind_ck CHECK (
        (kind = 'substitute' AND teacher_id IS NOT NULL AND swap_block_id IS NULL)
        OR (kind = 'swap' AND teacher_id IS NULL AND swap_block_id IS NOT NULL AND swap_block_id <> block_id)
        OR (kind = 'cancel' AND teacher_id IS NULL AND swap_block_id IS NULL)),
    CONSTRAINT schedule_overrides_reason_ck CHECK (char_length(btrim(reason)) BETWEEN 1 AND 200),
    -- One change per class and date (a swap's other class is checked by
    -- config under the cycle lock, and by the unique index below).
    CONSTRAINT schedule_overrides_block_date_key UNIQUE (tenant_id, block_id, on_date)
);
CREATE UNIQUE INDEX schedule_overrides_swap_date_key
    ON schedule_overrides (tenant_id, swap_block_id, on_date) WHERE swap_block_id IS NOT NULL;
CREATE INDEX idx_schedule_overrides_cycle_date ON schedule_overrides (tenant_id, cycle_id, on_date);
CREATE INDEX idx_schedule_overrides_block ON schedule_overrides (block_id);
CREATE INDEX idx_schedule_overrides_teacher ON schedule_overrides (teacher_id) WHERE teacher_id IS NOT NULL;

CREATE TABLE schedule_events (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL,
    cycle_id    uuid NOT NULL REFERENCES cycles (id) ON DELETE CASCADE,
    at          timestamptz NOT NULL DEFAULT now(),
    actor_id    text NOT NULL,
    actor_name  text NOT NULL,
    action      text NOT NULL CHECK (action IN (
        'block_created', 'block_updated', 'block_deleted', 'override_created', 'override_deleted')),
    block_id    uuid NULL,
    override_id uuid NULL,
    on_date     date NULL,
    reason      text NOT NULL DEFAULT '',
    summary     text NOT NULL
);
CREATE INDEX idx_schedule_events_cycle_at ON schedule_events (tenant_id, cycle_id, at DESC);

ALTER TABLE schedule_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE schedule_settings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON schedule_settings
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE schedule_blocks ENABLE ROW LEVEL SECURITY;
ALTER TABLE schedule_blocks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON schedule_blocks
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE schedule_overrides ENABLE ROW LEVEL SECURITY;
ALTER TABLE schedule_overrides FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON schedule_overrides
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE schedule_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE schedule_events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON schedule_events
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

-- The freeze of 0012, now also over the timetable (resolved by group, by the
-- class, or by the level).
CREATE OR REPLACE FUNCTION config_refuse_deleting_level() RETURNS trigger
    LANGUAGE plpgsql AS
$$
DECLARE
    lvl uuid;
BEGIN
    CASE TG_TABLE_NAME
        WHEN 'grade_levels', 'terms', 'term_months', 'schedule_settings' THEN
            lvl := NEW.level_id;
        WHEN 'subjects', 'groups', 'evaluation_periods' THEN
            SELECT level_id INTO lvl FROM grade_levels WHERE id = NEW.grade_level_id;
        WHEN 'teaching_assignments', 'schedule_blocks' THEN
            SELECT gl.level_id INTO lvl FROM groups g JOIN grade_levels gl ON gl.id = g.grade_level_id
             WHERE g.id = NEW.group_id;
        WHEN 'schedule_overrides' THEN
            SELECT gl.level_id INTO lvl FROM schedule_blocks b
              JOIN groups g ON g.id = b.group_id
              JOIN grade_levels gl ON gl.id = g.grade_level_id
             WHERE b.id = NEW.block_id;
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

CREATE TRIGGER schedule_settings_level_not_deleting BEFORE INSERT OR UPDATE ON schedule_settings
    FOR EACH ROW EXECUTE FUNCTION config_refuse_deleting_level();
CREATE TRIGGER schedule_blocks_level_not_deleting BEFORE INSERT OR UPDATE ON schedule_blocks
    FOR EACH ROW EXECUTE FUNCTION config_refuse_deleting_level();
CREATE TRIGGER schedule_overrides_level_not_deleting BEFORE INSERT OR UPDATE ON schedule_overrides
    FOR EACH ROW EXECUTE FUNCTION config_refuse_deleting_level();

COMMIT;
