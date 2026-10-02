-- grades — grades by evaluation period (T15, ADR 0030).
--
-- WHY. The school stops capturing grades month by month: a grade is captured
-- once per student, subject and EVALUATION PERIOD of the pupil's grade (the
-- periods config holds per cycle and grade since T14, ADR 0029). The owner
-- decided grades are WHOLE NUMBERS; only the averages the system computes
-- carry decimals (exact, truncated to 2).
--
-- WHAT.
--   period_grades      one row per (enrollment, subject, period). value is an
--                      exact numeric holding a whole number (CHECK value =
--                      trunc(value)); never a float. The level's scale bounds
--                      it in the use case; the table guarantees 0–100.
--                      period_id is config's evaluation period — deliberately
--                      NO foreign key (another service; config rewrites a
--                      grade's period rows keeping their ids, ADR 0029).
--                      captured_by / captured_by_name: who last wrote it.
--   report_cards.periods          the columns of a card issued since T15:
--                      [{"id","ordinal","name","startsOn"}] — the pupil's
--                      grade's periods up to the one closed. '[]' on cards
--                      issued before (their columns stay in `months`).
--   report_card_lines.period_values  {"<period id>": "9"} — the line's grade
--                      per period column. '{}' on cards issued before.
--
-- DATA. Purely additive: monthly_grades and its rows are KEPT untouched (no
-- conversion; nothing reads or writes them any more). Report cards already
-- issued keep every column they had and read back exactly as issued. Safe on
-- an empty or a populated database: no backfill, no rewrite, defaults only.
--
-- RLS: ENABLE + FORCE + tenant_isolation on the new table, like every grades
-- table; the application role is neither owner nor BYPASSRLS. The level
-- trash (0007) archives period_grades with its level (adapter).

BEGIN;

SET LOCAL lock_timeout = '5s';

CREATE TABLE period_grades (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL,
    enrollment_id     uuid NOT NULL,   -- students.enrollments (RPC-validated)
    subject_id        uuid NOT NULL,   -- config.subjects (DERIVED from the assignment)
    period_id         uuid NOT NULL,   -- config.evaluation_periods (RPC-validated)
    value             numeric(5,2) NOT NULL,
    captured_by       uuid NOT NULL,   -- iam user who last wrote the cell
    captured_by_name  text NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT period_grades_value_range_ck CHECK (value >= 0 AND value <= 100),
    CONSTRAINT period_grades_value_whole_ck CHECK (value = trunc(value)),
    -- One grade per student·subject·period (the upsert key).
    CONSTRAINT period_grades_key UNIQUE (tenant_id, enrollment_id, subject_id, period_id)
);

-- The capture sheet reads one subject·period for a group's enrollments; a
-- close and "does this period hold grades?" (config's guard) read by period.
CREATE INDEX idx_period_grades_period_subject ON period_grades (tenant_id, period_id, subject_id);
-- The family view reads every grade of one enrollment (the UNIQUE index's
-- prefix serves it too; a narrow index keeps it explicit and cheap).
CREATE INDEX idx_period_grades_enrollment ON period_grades (tenant_id, enrollment_id);

ALTER TABLE period_grades ENABLE ROW LEVEL SECURITY;
ALTER TABLE period_grades FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON period_grades
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE report_cards
    ADD COLUMN periods jsonb NOT NULL DEFAULT '[]'::jsonb
        CONSTRAINT report_cards_periods_ck CHECK (jsonb_typeof(periods) = 'array');

ALTER TABLE report_card_lines
    ADD COLUMN period_values jsonb NOT NULL DEFAULT '{}'::jsonb
        CONSTRAINT report_card_lines_period_values_ck CHECK (jsonb_typeof(period_values) = 'object');

COMMIT;
