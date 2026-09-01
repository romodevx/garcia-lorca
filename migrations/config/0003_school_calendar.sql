-- config (M16) — the school calendar: the days the school does not operate.
--
-- WHY IT LIVES HERE. The school asked for one calendar serving both its
-- collection rules and its own activities. Activities are not a money concern,
-- so putting this in finance would force every other module to ask the billing
-- service what day it is. uniforms already assumed this location — its
-- 0003_settings.sql says the school calendar "deliberately lives in the config
-- service" — and then found nothing there. This fills the seam that migration
-- described.
--
-- STORED AS RANGES, not one row per day. "Vacaciones de invierno" is one
-- decision a person makes once; sixteen rows would be sixteen chances for half
-- of them to be edited. The domain expands a range into days for the arithmetic.
BEGIN;

CREATE TABLE non_working_periods (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL,
    cycle_id   uuid NOT NULL REFERENCES cycles (id) ON DELETE CASCADE,
    label      text NOT NULL,
    kind       text NOT NULL CHECK (kind IN ('holiday', 'vacation')),
    -- Both ends inclusive. A single holiday has starts_on = ends_on.
    starts_on  date NOT NULL,
    ends_on    date NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT non_working_periods_range_ck CHECK (ends_on >= starts_on),
    -- The same label twice in one cycle is a duplicate entry, not two closures.
    -- Scoped to the cycle so "Semana Santa" can exist once per school year.
    UNIQUE (tenant_id, cycle_id, label)
);

-- The read is always "every closed day of this cycle", so that is the index.
CREATE INDEX idx_non_working_periods_cycle
    ON non_working_periods (tenant_id, cycle_id, starts_on);

-- RLS, same posture as every other tenant-scoped table here: ENABLE + FORCE, so
-- the policy applies even to the table owner, and the app role can never read
-- another school's calendar. app.current_tenant is bound per transaction by the
-- config postgres adapter via pkg/authctx.
ALTER TABLE non_working_periods ENABLE ROW LEVEL SECURITY;
ALTER TABLE non_working_periods FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON non_working_periods
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
