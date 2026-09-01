-- config (M16) — la agenda de la escuela.
--
-- The review asks for one calendar serving two purposes: the days the school is
-- shut, and the things the school does. They look alike on a wall planner and
-- they are not the same fact.
--
-- WHY A SEPARATE TABLE. non_working_periods is read by finance to work out
-- deadlines: a day in it moves a due date and stops a surcharge. An activity
-- must never do that — a festival on the 10th does not mean nobody could pay on
-- the 10th. Filing both in one table with a `kind` column would put one CHECK
-- between the school's agenda and every family's due date, and the day somebody
-- adds "Festival de primavera" as a holiday, four hundred deadlines move and
-- nobody finds out until a parent argues about a surcharge.
--
-- So: events live here, closures live there, and the screen shows both.
--
-- AUDIENCE. An event may be for everybody, for one level, or for one group.
-- Both columns are plain uuids and nullable; the pair is validated by the use
-- case, which knows what a level and a group are. There is no FK to groups on
-- purpose: an event survives the group being renamed or re-formed next cycle.
BEGIN;

CREATE TABLE school_events (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL,
    cycle_id    uuid NOT NULL REFERENCES cycles (id) ON DELETE CASCADE,
    title       text NOT NULL CHECK (length(title) BETWEEN 1 AND 120),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 500),
    -- Both ends inclusive. A one-day event has starts_on = ends_on.
    starts_on   date NOT NULL,
    ends_on     date NOT NULL,
    -- NULL in both means the whole school. At most one may be set.
    level_id    uuid NULL,
    group_id    uuid NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT school_events_range_ck CHECK (ends_on >= starts_on),
    CONSTRAINT school_events_one_audience_ck
        CHECK (level_id IS NULL OR group_id IS NULL)
);

-- The read is always "what is happening in this cycle", in date order.
CREATE INDEX idx_school_events_cycle ON school_events (tenant_id, cycle_id, starts_on);

ALTER TABLE school_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE school_events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON school_events
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
