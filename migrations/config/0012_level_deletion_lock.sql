-- config — the level deletion's lock, its single-use token and its one clock
-- (ADR 0021, review fixes).
--
-- levels.status: 'active' | 'deleting'. BeginLevelDeletion (the saga's FIRST
-- step) sets 'deleting'; while it holds, every write that resolves the level is
-- refused with LEVEL_BEING_DELETED — here by trigger (config's own structure),
-- in grades/attendance/students by their level lookup. Compensation or a
-- restore puts it back to 'active'.
--
-- level_deletions grows the saga's lifecycle:
--   deleting          the saga is running (at most ONE per level: unique index)
--   deleted           every step archived — in the trash
--   failed_partial    it failed and could not be fully undone — in the trash;
--                     pending_steps names the services whose archives may still
--                     hold rows, skipped what the compensation could not put back
--   restored_partial  restored, but some rows did not fit — still in the trash
--   restored          fully restored
--   cancelled         it failed and was fully undone: nothing was deleted
--   purged            its archives are gone
-- token_hash is the SHA-256 of the confirm token: a token opens one saga only.
-- restorable_until is written from the database's now() at BeginLevelDeletion
-- and is the only clock the trash reads (restore window, purge sweep).
-- purged_at marks a batch whose archives the sweep already dropped.
--
-- Additive only; RLS already ENABLE + FORCE on both tables.

BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE levels
    ADD COLUMN status text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'deleting'));

ALTER TABLE level_deletions DROP CONSTRAINT level_deletions_status_check;
ALTER TABLE level_deletions
    ADD CONSTRAINT level_deletions_status_check CHECK (status IN (
        'deleting', 'deleted', 'failed_partial', 'restored_partial',
        'restored', 'cancelled', 'purged'));
ALTER TABLE level_deletions
    ADD COLUMN token_hash    text CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    ADD COLUMN pending_steps text[] NOT NULL DEFAULT '{}',
    ADD COLUMN skipped       jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN purged_at     timestamptz;

-- The lock: one saga per level at a time.
CREATE UNIQUE INDEX level_deletions_one_running
    ON level_deletions (tenant_id, level_id) WHERE status = 'deleting';
-- The single-use token.
CREATE UNIQUE INDEX level_deletions_token_once
    ON level_deletions (tenant_id, token_hash) WHERE token_hash IS NOT NULL;
-- The sweep reads what is past its window and not purged yet.
CREATE INDEX idx_level_deletions_sweep
    ON level_deletions (tenant_id, restorable_until) WHERE purged_at IS NULL;

-- The freeze. A row of the level's structure cannot be written while the level
-- is being deleted: the saga archives config LAST, so a write in between would
-- otherwise land beside rows other services already moved to the trash.
-- SQLSTATE 55000 (object_not_in_prerequisite_state) + the code as the message;
-- the repository maps it to LEVEL_BEING_DELETED. Runs as the invoking role, so
-- RLS scopes every lookup to the session's school.
CREATE FUNCTION config_level_is_deleting(p_level uuid) RETURNS boolean
    LANGUAGE sql STABLE AS
$$ SELECT EXISTS (SELECT 1 FROM levels WHERE id = p_level AND status = 'deleting') $$;

CREATE FUNCTION config_refuse_deleting_level() RETURNS trigger
    LANGUAGE plpgsql AS
$$
DECLARE
    lvl uuid;
BEGIN
    CASE TG_TABLE_NAME
        WHEN 'grade_levels', 'terms', 'term_months' THEN
            lvl := NEW.level_id;
        WHEN 'subjects', 'groups' THEN
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

CREATE TRIGGER grade_levels_level_not_deleting BEFORE INSERT OR UPDATE ON grade_levels
    FOR EACH ROW EXECUTE FUNCTION config_refuse_deleting_level();
CREATE TRIGGER subjects_level_not_deleting BEFORE INSERT OR UPDATE ON subjects
    FOR EACH ROW EXECUTE FUNCTION config_refuse_deleting_level();
CREATE TRIGGER groups_level_not_deleting BEFORE INSERT OR UPDATE ON groups
    FOR EACH ROW EXECUTE FUNCTION config_refuse_deleting_level();
CREATE TRIGGER terms_level_not_deleting BEFORE INSERT OR UPDATE ON terms
    FOR EACH ROW EXECUTE FUNCTION config_refuse_deleting_level();
CREATE TRIGGER term_months_level_not_deleting BEFORE INSERT OR UPDATE ON term_months
    FOR EACH ROW EXECUTE FUNCTION config_refuse_deleting_level();
CREATE TRIGGER teaching_assignments_level_not_deleting BEFORE INSERT OR UPDATE ON teaching_assignments
    FOR EACH ROW EXECUTE FUNCTION config_refuse_deleting_level();
CREATE TRIGGER school_events_level_not_deleting BEFORE INSERT OR UPDATE ON school_events
    FOR EACH ROW EXECUTE FUNCTION config_refuse_deleting_level();

-- The level itself: while deleting, only the status may change (compensation
-- or restore puts it back to active).
CREATE FUNCTION config_refuse_deleting_level_update() RETURNS trigger
    LANGUAGE plpgsql AS
$$
BEGIN
    IF OLD.status = 'deleting' AND NEW.status = 'deleting' THEN
        RAISE EXCEPTION 'LEVEL_BEING_DELETED' USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END
$$;

CREATE TRIGGER levels_not_deleting BEFORE UPDATE ON levels
    FOR EACH ROW EXECUTE FUNCTION config_refuse_deleting_level_update();

COMMIT;
