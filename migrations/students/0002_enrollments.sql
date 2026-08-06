-- students (Phase 4) — enrollments + transfer history.
--
-- Places a student into a group of a cycle (enrollments) and records dated,
-- reasoned moves between groups (enrollment_transfers).
--
-- RLS is tenant-scoped (same posture as students/0001 and iam, ADR 0001):
-- policy filters on app.current_tenant, ENABLE + FORCE so it applies even to the
-- table owner. The application role is NOT the owner and NOT BYPASSRLS.
--
-- Bounded-context discipline: cycle_id / group_id / term_id belong to the config
-- service. They are plain uuid columns validated by RPC at the use-case layer —
-- deliberately NOT foreign keys (config owns those tables). Only same-service
-- references (student_id, enrollment_id) are real FKs.

BEGIN;

CREATE TABLE enrollments (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL,
    student_id  uuid NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    -- cross-service refs to config (NO FK — validated by RPC, not the DB):
    cycle_id    uuid NOT NULL,   -- config.cycles  (derived from the group)
    group_id    uuid NOT NULL,   -- config.groups  (RPC-validated)
    term_id     uuid NULL,       -- config.terms   (RPC-validated when present)
    status      text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'transferred', 'withdrawn')),
    enrolled_at timestamptz NOT NULL DEFAULT now(),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    -- one enrollment per student per cycle (domain invariant).
    UNIQUE (tenant_id, student_id, cycle_id)
);

CREATE INDEX idx_enrollments_student ON enrollments (tenant_id, student_id);
CREATE INDEX idx_enrollments_cycle ON enrollments (tenant_id, cycle_id);
CREATE INDEX idx_enrollments_group ON enrollments (tenant_id, group_id);

-- Row Level Security (tenant isolation). Unset app.current_tenant ⇒ zero rows.
ALTER TABLE enrollments ENABLE ROW LEVEL SECURITY;
ALTER TABLE enrollments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON enrollments
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

CREATE TABLE enrollment_transfers (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL,
    enrollment_id uuid NOT NULL REFERENCES enrollments(id) ON DELETE CASCADE,
    -- cross-service refs to config (NO FK — see note above):
    from_group_id uuid NOT NULL,   -- config.groups
    to_group_id   uuid NOT NULL,   -- config.groups
    reason        text NOT NULL,
    moved_at      timestamptz NOT NULL DEFAULT now(),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_enrollment_transfers_enrollment ON enrollment_transfers (tenant_id, enrollment_id);

-- Row Level Security (tenant isolation). Unset app.current_tenant ⇒ zero rows.
ALTER TABLE enrollment_transfers ENABLE ROW LEVEL SECURITY;
ALTER TABLE enrollment_transfers FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON enrollment_transfers
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
