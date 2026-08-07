-- students (Phase 9b) — guardian ↔ student links.
--
-- A guardian is an iam user (role "guardian"), not a row of this service: the
-- link stores guardian_user_id as a plain uuid validated by RPC at the use-case
-- layer — deliberately NOT a foreign key (iam owns users). Only the same-service
-- reference (student_id) is a real FK, matching students/0002.
--
-- The relation is many-to-many: a guardian may have several students (siblings)
-- and a student may have several guardians (mother, father, …). UNIQUE keeps the
-- pair idempotent per tenant.
--
-- RLS is tenant-scoped (same posture as students/0001 and 0002, ADR 0001):
-- policy filters on app.current_tenant, ENABLE + FORCE so it applies even to the
-- table owner. The application role is NOT the owner and NOT BYPASSRLS.

BEGIN;

CREATE TABLE guardian_students (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid NOT NULL,
    -- cross-service ref to iam (NO FK — validated by RPC, not the DB):
    guardian_user_id uuid NOT NULL,   -- iam.users (the guardian)
    student_id       uuid NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    relationship     text NOT NULL DEFAULT 'guardian'
        CHECK (relationship IN ('mother', 'father', 'guardian', 'other')),
    created_at       timestamptz NOT NULL DEFAULT now(),
    -- one link per guardian/student pair (domain invariant).
    UNIQUE (tenant_id, guardian_user_id, student_id)
);

CREATE INDEX idx_guardian_students_student ON guardian_students (tenant_id, student_id);
CREATE INDEX idx_guardian_students_guardian ON guardian_students (tenant_id, guardian_user_id);

-- Row Level Security (tenant isolation). Unset app.current_tenant ⇒ zero rows.
ALTER TABLE guardian_students ENABLE ROW LEVEL SECURITY;
ALTER TABLE guardian_students FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON guardian_students
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
