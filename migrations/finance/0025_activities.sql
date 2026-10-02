-- finance (T17) — activities (actividades extracurriculares) and their charge.
--
-- THE PROBLEM. The school runs swimming, music, robotics… each with its own
-- weekly schedule and its own price, charged per period on top of the
-- tuition. Nothing in the ledger knows about them, so the office bills them by
-- hand and the parents' app never shows them.
--
-- THE SHAPE (ADR 0031).
--
--   * activities: name (one per school, whatever its case), the weekly
--     schedule as JSONB [{weekday 1–7, start "HH:MM", end "HH:MM"}] (it is
--     descriptive — what the family is told — and never read by SQL), the
--     periodicity (monthly | weekly | per_cycle), the first billing date and
--     the first due date (later periods repeat both), an optional fixed late
--     fee (0 = none), active.
--   * activity_price_options: the charging schemes — a price per period for
--     attending N days a week, or (days_per_week NULL) a flat fee. Retired,
--     never deleted, once an enrolment has pointed at one: the history keeps
--     its option.
--   * activity_enrollments: a pupil in an activity with the option chosen for
--     them, from starts_on until ends_on (the first day no longer enrolled;
--     NULL = still enrolled). At most one open enrolment per pupil and
--     activity. The option belongs to the SAME activity (composite FK).
--
-- Charges are ordinary charges under the new system concept kind 'activity',
-- ref "activity:<activity>:<student>:<period>" (unique per tenant, ADR 0008),
-- so the run is idempotent and nothing here duplicates the ledger.
--
-- Money is integer cents. Additive; reversible by dropping the three tables
-- and restoring the kind CHECK of 0013. RLS: ENABLE + FORCE + tenant_isolation
-- on every table.
BEGIN;

SET LOCAL lock_timeout = '5s';

-- The concept every activity charge hangs from (found by KIND, like 0004).
ALTER TABLE charge_concepts DROP CONSTRAINT IF EXISTS charge_concepts_kind_check;
ALTER TABLE charge_concepts
    ADD CONSTRAINT charge_concepts_kind_check
        CHECK (kind IN ('custom', 'tuition', 'enrollment', 'late_fee', 'fee', 'activity'));

CREATE TABLE activities (
    id                uuid PRIMARY KEY,
    tenant_id         uuid NOT NULL,
    name              text NOT NULL CHECK (btrim(name) <> '' AND char_length(name) <= 80),
    schedule          jsonb NOT NULL CHECK (jsonb_typeof(schedule) = 'array' AND jsonb_array_length(schedule) > 0),
    periodicity       text NOT NULL DEFAULT 'monthly' CHECK (periodicity IN ('monthly', 'weekly', 'per_cycle')),
    billing_starts_on date NOT NULL,
    first_due_on      date NOT NULL,
    late_fee_cents    bigint NOT NULL DEFAULT 0 CHECK (late_fee_cents >= 0 AND late_fee_cents <= 1000000000000000),
    active            boolean NOT NULL DEFAULT true,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT activities_due_after_start_ck CHECK (first_due_on >= billing_starts_on)
);

-- One name per school, whatever its case. (Accents are compared by the use
-- case, which knows the school's LineKey.)
CREATE UNIQUE INDEX uq_activities_name ON activities (tenant_id, lower(btrim(name)));

CREATE TABLE activity_price_options (
    id            uuid PRIMARY KEY,
    tenant_id     uuid NOT NULL,
    activity_id   uuid NOT NULL REFERENCES activities (id) ON DELETE RESTRICT,
    days_per_week smallint NULL CHECK (days_per_week BETWEEN 1 AND 7),
    amount_cents  bigint NOT NULL CHECK (amount_cents > 0 AND amount_cents <= 1000000000000000),
    retired       boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),

    -- The target of the enrolments' composite FK: an option of THIS activity.
    CONSTRAINT uq_activity_price_options_activity_id UNIQUE (activity_id, id)
);

-- One live option per number of days, and one live flat fee (NULL → 0).
CREATE UNIQUE INDEX uq_activity_price_options_days
    ON activity_price_options (activity_id, COALESCE(days_per_week, 0))
    WHERE NOT retired;

CREATE TABLE activity_enrollments (
    id          uuid PRIMARY KEY,
    tenant_id   uuid NOT NULL,
    activity_id uuid NOT NULL REFERENCES activities (id) ON DELETE RESTRICT,
    student_id  uuid NOT NULL,
    option_id   uuid NOT NULL,
    starts_on   date NOT NULL,
    ends_on     date NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT activity_enrollments_option_fk FOREIGN KEY (activity_id, option_id)
        REFERENCES activity_price_options (activity_id, id) ON DELETE RESTRICT
);

-- At most one open enrolment per pupil and activity.
CREATE UNIQUE INDEX uq_activity_enrollments_open
    ON activity_enrollments (tenant_id, activity_id, student_id)
    WHERE ends_on IS NULL;
-- The run walks an activity's enrolments; the student's view reads by pupil.
CREATE INDEX idx_activity_enrollments_activity ON activity_enrollments (tenant_id, activity_id);
CREATE INDEX idx_activity_enrollments_student ON activity_enrollments (tenant_id, student_id);
CREATE INDEX idx_activity_enrollments_option ON activity_enrollments (activity_id, option_id);

ALTER TABLE activities ENABLE ROW LEVEL SECURITY;
ALTER TABLE activities FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON activities
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE activity_price_options ENABLE ROW LEVEL SECURITY;
ALTER TABLE activity_price_options FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON activity_price_options
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE activity_enrollments ENABLE ROW LEVEL SECURITY;
ALTER TABLE activity_enrollments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON activity_enrollments
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
