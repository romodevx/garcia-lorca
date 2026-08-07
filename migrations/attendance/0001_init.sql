-- attendance (M06) initial schema — daily attendance marks.
--
-- RLS is tenant-scoped (same posture as iam/students/grades, ADR 0001): policy
-- filters on app.current_tenant, ENABLE + FORCE so it applies even to the table
-- owner. The application role is NOT the owner and NOT BYPASSRLS.
--
-- Default-present model: only EXCEPTIONS are stored. A student with no row for a
-- given date is implicitly present, so a full-roster day costs zero writes and
-- the table grows with absences, not with enrollments × school days.
--
-- enrollment_id is a cross-service UUID (students.enrollments) validated by RPC
-- at the use-case layer — deliberately NO foreign key (it lives in the students
-- service's database logically).
--
-- Run by a privileged migration role. gen_random_uuid() is in core since PG13.

BEGIN;

CREATE TABLE attendance (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL,
    enrollment_id uuid NOT NULL,   -- students.enrollments (RPC-validated, NOT a FK)
    date          date NOT NULL,
    -- 'present' is deliberately allowed although the default-present model above
    -- never stores it: it is reserved for the Phase 8b QR check-in, which records
    -- an explicit present. Until then the service drops present marks.
    status        text NOT NULL CHECK (status IN ('present', 'absent', 'late', 'excused')),
    -- Free text from a teacher's browser: bounded here as well as in the service.
    note          text NOT NULL DEFAULT '' CHECK (length(note) <= 500),   -- optional reason for late/excused
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    -- One mark per student per day (the reconcile/upsert key).
    UNIQUE (tenant_id, enrollment_id, date)
);

CREATE INDEX idx_attendance_enrollment ON attendance (tenant_id, enrollment_id);
CREATE INDEX idx_attendance_date ON attendance (tenant_id, date);

-- Row Level Security (tenant isolation). Unset app.current_tenant ⇒ zero rows.
ALTER TABLE attendance ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON attendance
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
