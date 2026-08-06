-- config (Phase 3) teachers + teaching_assignments: the academic knot binding
-- teacher ↔ group ↔ subject ↔ cycle. Extends the 0001 catalog in the same DB.
--
-- teachers are catalog PERSON records, NOT login accounts: user_id is a nullable
-- reserved slot for a future IAM link (roles/users phase) and is unused now.
--
-- teaching_assignments is the knot. Its cycle_id is DERIVED from the group at
-- create time (never taken from the client) so it can't drift. FKs are real
-- (all rows live in one config DB). cycle_id → cycles ON DELETE CASCADE (removing
-- a draft cycle removes its assignments); group/subject/teacher → ON DELETE
-- RESTRICT (can't drop something an assignment still points at). The UNIQUE
-- (tenant_id, cycle_id, group_id, subject_id) backstops "one teacher per
-- (group, subject) in a cycle".
--
-- RLS is tenant-scoped, identical shape to 0001: ENABLE + FORCE + a
-- tenant_isolation policy on app.current_tenant. Unset app.current_tenant ⇒
-- zero rows. gen_random_uuid() is in core since PG13.

BEGIN;

-- --- catalog: teachers ----------------------------------------------------
-- A teacher as a catalog person record (NOT a login account). user_id is a
-- reserved, currently-unused nullable link to a future IAM user.
CREATE TABLE teachers (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL,
    first_name text NOT NULL,
    last_name  text NOT NULL,
    email      text NOT NULL,
    user_id    uuid,                    -- reserved for a future IAM link; unused now
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    -- teacher email is unique per tenant (catalog invariant).
    UNIQUE (tenant_id, email)
);

-- --- the knot: teaching_assignments ---------------------------------------
-- Binds a teacher to a (group, subject) within a cycle. cycle_id is DERIVED
-- from the group at create time by the use case, never supplied by the client.
CREATE TABLE teaching_assignments (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL,
    cycle_id   uuid NOT NULL REFERENCES cycles (id) ON DELETE CASCADE,
    group_id   uuid NOT NULL REFERENCES groups (id) ON DELETE RESTRICT,
    subject_id uuid NOT NULL REFERENCES subjects (id) ON DELETE RESTRICT,
    teacher_id uuid NOT NULL REFERENCES teachers (id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    -- one teacher per (group, subject) in a cycle.
    UNIQUE (tenant_id, cycle_id, group_id, subject_id)
);

-- --- indexes (every FK + tenant_id + common access paths) -----------------
CREATE INDEX idx_teachers_tenant ON teachers (tenant_id);
CREATE INDEX idx_teaching_assignments_tenant ON teaching_assignments (tenant_id);
CREATE INDEX idx_teaching_assignments_cycle ON teaching_assignments (cycle_id);
CREATE INDEX idx_teaching_assignments_group ON teaching_assignments (group_id);
CREATE INDEX idx_teaching_assignments_subject ON teaching_assignments (subject_id);
CREATE INDEX idx_teaching_assignments_teacher ON teaching_assignments (teacher_id);
-- "assignments of a teacher" and "assignments in a cycle" access patterns.
CREATE INDEX idx_teaching_assignments_tenant_teacher ON teaching_assignments (tenant_id, teacher_id);
CREATE INDEX idx_teaching_assignments_tenant_cycle ON teaching_assignments (tenant_id, cycle_id);

-- --- Row Level Security (tenant isolation) --------------------------------
-- Identical shape to 0001: USING/WITH CHECK on app.current_tenant, bound per
-- transaction by the config postgres adapter via pkg/authctx.

ALTER TABLE teachers ENABLE ROW LEVEL SECURITY;
ALTER TABLE teachers FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON teachers
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE teaching_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE teaching_assignments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON teaching_assignments
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
