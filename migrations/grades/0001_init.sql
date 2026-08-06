-- grades (M07) initial schema — grade capture.
--
-- RLS is tenant-scoped (same posture as iam/students, ADR 0001): policy filters
-- on app.current_tenant, ENABLE + FORCE so it applies even to the table owner.
-- The application role is NOT the owner and NOT BYPASSRLS.
--
-- All references (enrollment_id, subject_id, term_id, teacher_id) are
-- cross-service UUIDs validated by RPC at the use-case layer — deliberately NO
-- foreign keys (they live in the students/config services' databases logically).
--
-- Run by a privileged migration role. gen_random_uuid() is in core since PG13.

BEGIN;

CREATE TABLE grade_entries (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL,
    enrollment_id uuid NOT NULL,   -- students.enrollments (RPC-validated)
    subject_id    uuid NOT NULL,   -- config.subjects      (DERIVED from the assignment)
    term_id       uuid NOT NULL,   -- config.terms         (RPC-validated, in the assignment's cycle)
    teacher_id    uuid NOT NULL,   -- config.teachers      (DERIVED from the assignment)
    value         numeric(4,2) NOT NULL CHECK (value >= 0 AND value <= 10),
    published_at  timestamptz NULL,   -- reserved for the future boleta/publish flow; unused now
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    -- One grade per student·subject·term (the upsert key).
    UNIQUE (tenant_id, enrollment_id, subject_id, term_id)
);

CREATE INDEX idx_grade_entries_enrollment ON grade_entries (tenant_id, enrollment_id);
CREATE INDEX idx_grade_entries_subject_term ON grade_entries (tenant_id, subject_id, term_id);
CREATE INDEX idx_grade_entries_teacher ON grade_entries (tenant_id, teacher_id);

-- Row Level Security (tenant isolation). Unset app.current_tenant ⇒ zero rows.
ALTER TABLE grade_entries ENABLE ROW LEVEL SECURITY;
ALTER TABLE grade_entries FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON grade_entries
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
