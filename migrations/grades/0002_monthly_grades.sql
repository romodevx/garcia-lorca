-- grades — monthly grades (T6, calificaciones mensuales).
--
-- The school captures one grade per student, subject and calendar MONTH. The
-- term-based grade_entries table (0001) stays as it is: untouched, no longer
-- written, and no data is transformed.
--
-- month is the first day of the month (CHECK). value is bounded per level by the
-- use case (config's grading scale); the table only guarantees a sane 0–100
-- hundredths value. captured_by is the user who last wrote the cell (a director
-- may correct a teacher's grade). References are cross-service UUIDs validated
-- by RPC — deliberately NO foreign keys (same posture as 0001).
--
-- RLS is tenant-scoped exactly like grade_entries: ENABLE + FORCE, policy on
-- app.current_tenant; the application role is neither owner nor BYPASSRLS.

BEGIN;

CREATE TABLE monthly_grades (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL,
    enrollment_id uuid NOT NULL,   -- students.enrollments (RPC-validated)
    subject_id    uuid NOT NULL,   -- config.subjects      (DERIVED from the assignment)
    teacher_id    uuid NOT NULL,   -- config.teachers      (DERIVED from the assignment)
    month         date NOT NULL CHECK (extract(day FROM month) = 1),
    value         numeric(5,2) NOT NULL CHECK (value >= 0 AND value <= 100),
    captured_by   uuid NOT NULL,   -- iam user who last wrote the cell
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    -- One grade per student·subject·month (the upsert key).
    UNIQUE (tenant_id, enrollment_id, subject_id, month)
);

-- The capture sheet reads one subject·month for a group's enrollments; the
-- family view reads every grade of one enrollment (the UNIQUE index's prefix
-- already serves it, but a narrow index keeps that read cheap and explicit).
CREATE INDEX idx_monthly_grades_subject_month ON monthly_grades (tenant_id, subject_id, month);
CREATE INDEX idx_monthly_grades_enrollment ON monthly_grades (tenant_id, enrollment_id);

-- Row Level Security (tenant isolation). Unset app.current_tenant ⇒ zero rows.
ALTER TABLE monthly_grades ENABLE ROW LEVEL SECURITY;
ALTER TABLE monthly_grades FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON monthly_grades
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
