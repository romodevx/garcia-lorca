-- students (M03) initial schema.
--
-- RLS is tenant-scoped (same posture as iam, ADR 0001): policy filters on
-- app.current_tenant, ENABLE + FORCE so it applies even to the table owner. The
-- application role is NOT the owner and NOT BYPASSRLS.
--
-- Run by a privileged migration role. gen_random_uuid() is in core since PG13.

BEGIN;

CREATE TABLE students (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid NOT NULL,
    curp             text NOT NULL,
    first_name       text NOT NULL,
    last_name        text NOT NULL,
    level            text NOT NULL CHECK (level IN ('kinder', 'primaria', 'secundaria', 'preparatoria')),
    grade_group      text NOT NULL DEFAULT '',
    guardian_name    text NOT NULL DEFAULT '',
    admission_status text NOT NULL DEFAULT 'pending'
        CHECK (admission_status IN ('pending', 'accepted', 'enrolled', 'rejected', 'withdrawn')),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    -- CURP is unique per tenant (domain invariant).
    UNIQUE (tenant_id, curp)
);

CREATE INDEX idx_students_tenant ON students (tenant_id);
CREATE INDEX idx_students_status ON students (tenant_id, admission_status);
CREATE INDEX idx_students_level ON students (tenant_id, level);

-- Row Level Security (tenant isolation). Unset app.current_tenant ⇒ zero rows.
ALTER TABLE students ENABLE ROW LEVEL SECURITY;
ALTER TABLE students FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON students
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
