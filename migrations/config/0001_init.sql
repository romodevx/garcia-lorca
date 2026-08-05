-- config (M04) initial schema: the academic catalog the whole spine hangs off.
-- Cross-cycle catalog: levels, grade_levels, subjects.
-- Per-cycle:          cycles, terms, groups.
--
-- RLS is tenant-scoped (same posture as iam/students, ADR 0001): every policy
-- filters on app.current_tenant, ENABLE + FORCE so it applies even to the table
-- owner. The application role is NOT the owner and is NOT BYPASSRLS
-- (deploy/terraform/db/setup_app_role.sql). Unset app.current_tenant ⇒ zero rows.
--
-- Intra-config references are real FKs (all six entities live in one DB). Catalog
-- FKs are ON DELETE RESTRICT (can't drop a level that still has grades/terms);
-- cycle FKs are ON DELETE CASCADE (removing a draft cycle removes its terms/groups).
-- FKs reference id only: ids are globally unique UUIDs, so they are tenant-safe.
--
-- Run by a privileged migration role. gen_random_uuid() is in core since PG13.

BEGIN;

-- --- catalog: levels ------------------------------------------------------
-- A SEP education level (e.g. primaria). enrollment_granularity drives the
-- hybrid cycle model: a level enrolls per CYCLE or per TERM.
CREATE TABLE levels (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id              uuid NOT NULL,
    name                   text NOT NULL,
    sep_code               text NOT NULL DEFAULT '',
    enrollment_granularity text NOT NULL
        CHECK (enrollment_granularity IN ('CYCLE', 'TERM')),
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    -- level name is unique per tenant (catalog invariant).
    UNIQUE (tenant_id, name)
);

-- --- per-cycle: cycles ----------------------------------------------------
-- A school-year umbrella (e.g. "2026-2027"). status is a lifecycle state.
CREATE TABLE cycles (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL,
    name       text NOT NULL,
    status     text NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'active', 'closed')),
    starts_on  date NOT NULL,
    ends_on    date NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    -- cycle name is unique per tenant (catalog invariant).
    UNIQUE (tenant_id, name)
);

-- --- catalog: grade_levels ------------------------------------------------
-- A grade within a level (e.g. "1° primaria"). ordinal orders grades in a level.
CREATE TABLE grade_levels (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL,
    level_id   uuid NOT NULL REFERENCES levels (id) ON DELETE RESTRICT,
    name       text NOT NULL,
    ordinal    integer NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    -- ordinal is unique within a level per tenant.
    UNIQUE (tenant_id, level_id, ordinal)
);

-- --- catalog: subjects ----------------------------------------------------
-- A subject taught at a grade (e.g. "Matemáticas").
CREATE TABLE subjects (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL,
    grade_level_id uuid NOT NULL REFERENCES grade_levels (id) ON DELETE RESTRICT,
    name           text NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    -- subject name is unique within a grade per tenant.
    UNIQUE (tenant_id, grade_level_id, name)
);

-- --- per-cycle: terms -----------------------------------------------------
-- A period within a cycle for a level (e.g. "T1"). Removed with its cycle.
CREATE TABLE terms (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL,
    cycle_id   uuid NOT NULL REFERENCES cycles (id) ON DELETE CASCADE,
    level_id   uuid NOT NULL REFERENCES levels (id) ON DELETE RESTRICT,
    name       text NOT NULL,
    ordinal    integer NOT NULL,
    starts_on  date NOT NULL,
    ends_on    date NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    -- ordinal is unique within a (cycle, level) per tenant.
    UNIQUE (tenant_id, cycle_id, level_id, ordinal)
);

-- --- per-cycle: groups ----------------------------------------------------
-- A class group within a cycle for a grade (e.g. "A"). Removed with its cycle.
CREATE TABLE groups (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL,
    cycle_id       uuid NOT NULL REFERENCES cycles (id) ON DELETE CASCADE,
    grade_level_id uuid NOT NULL REFERENCES grade_levels (id) ON DELETE RESTRICT,
    name           text NOT NULL,
    capacity       integer NOT NULL DEFAULT 0,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    -- group name is unique within a (cycle, grade) per tenant.
    UNIQUE (tenant_id, cycle_id, grade_level_id, name)
);

-- --- indexes (every FK + tenant_id) ---------------------------------------
CREATE INDEX idx_levels_tenant ON levels (tenant_id);
CREATE INDEX idx_cycles_tenant ON cycles (tenant_id);
CREATE INDEX idx_grade_levels_tenant ON grade_levels (tenant_id);
CREATE INDEX idx_grade_levels_level ON grade_levels (level_id);
CREATE INDEX idx_subjects_tenant ON subjects (tenant_id);
CREATE INDEX idx_subjects_grade_level ON subjects (grade_level_id);
CREATE INDEX idx_terms_tenant ON terms (tenant_id);
CREATE INDEX idx_terms_cycle ON terms (cycle_id);
CREATE INDEX idx_terms_level ON terms (level_id);
CREATE INDEX idx_groups_tenant ON groups (tenant_id);
CREATE INDEX idx_groups_cycle ON groups (cycle_id);
CREATE INDEX idx_groups_grade_level ON groups (grade_level_id);

-- --- Row Level Security (tenant isolation) --------------------------------
-- Identical shape to iam/students 0001: USING/WITH CHECK on app.current_tenant,
-- bound per transaction by the config postgres adapter via pkg/authctx.

ALTER TABLE levels ENABLE ROW LEVEL SECURITY;
ALTER TABLE levels FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON levels
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE cycles ENABLE ROW LEVEL SECURITY;
ALTER TABLE cycles FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cycles
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE grade_levels ENABLE ROW LEVEL SECURITY;
ALTER TABLE grade_levels FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON grade_levels
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE subjects ENABLE ROW LEVEL SECURITY;
ALTER TABLE subjects FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON subjects
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE terms ENABLE ROW LEVEL SECURITY;
ALTER TABLE terms FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON terms
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE groups ENABLE ROW LEVEL SECURITY;
ALTER TABLE groups FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON groups
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
