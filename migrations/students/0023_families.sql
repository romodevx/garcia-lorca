-- students — families (ADR 0041, PLAN-familias.md).
--
-- families: a group of siblings the office confirmed. students.family_id puts a
-- pupil in at most one. guardian_students is NOT touched: it stays the only
-- answer to "which child does this account see" (ADR 0004); the family is the
-- layer that writes those links consistently, and its row is the lock that
-- serialises the "at most two app accounts" count (SELECT ... FOR UPDATE).
--
-- family_suggestion_dismissals: "these pupils are not a family", keyed by the
-- SHA-256 of the sorted ids of the dismissed group. A new sibling changes the
-- signature, so the suggestion comes back with them.
--
-- RLS: ENABLE + FORCE from this first migration, one policy per operation on
-- app.current_tenant, as every students table. Re-runnable.
BEGIN;

SET LOCAL lock_timeout = '5s';

CREATE TABLE IF NOT EXISTS families (
    id         uuid PRIMARY KEY,
    tenant_id  uuid NOT NULL,
    name       text NOT NULL CHECK (char_length(btrim(name)) BETWEEN 1 AND 120),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- A composite FK needs a unique key on the pair: a pupil can never point at
-- another school's family, not even from a writer that bypasses RLS.
CREATE UNIQUE INDEX IF NOT EXISTS uq_families_tenant_id ON families (tenant_id, id);
CREATE INDEX IF NOT EXISTS idx_families_tenant_name ON families (tenant_id, name);

ALTER TABLE students ADD COLUMN IF NOT EXISTS family_id uuid NULL;
-- NO ACTION, not SET NULL: a composite SET NULL would null tenant_id too.
-- Dissolving a family clears its members first, in the same transaction.
ALTER TABLE students DROP CONSTRAINT IF EXISTS students_family_fk;
ALTER TABLE students ADD CONSTRAINT students_family_fk
    FOREIGN KEY (tenant_id, family_id) REFERENCES families (tenant_id, id);
-- The members of a family (GetFamily, the FK check on delete).
CREATE INDEX IF NOT EXISTS idx_students_family
    ON students (tenant_id, family_id)
    WHERE family_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS family_suggestion_dismissals (
    id           uuid PRIMARY KEY,
    tenant_id    uuid NOT NULL,
    signature    text NOT NULL CHECK (signature ~ '^[0-9a-f]{64}$'),
    dismissed_by uuid NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, signature)
);

ALTER TABLE families ENABLE ROW LEVEL SECURITY;
ALTER TABLE families FORCE ROW LEVEL SECURITY;
ALTER TABLE family_suggestion_dismissals ENABLE ROW LEVEL SECURITY;
ALTER TABLE family_suggestion_dismissals FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_read ON families;
CREATE POLICY tenant_read ON families FOR SELECT
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);
DROP POLICY IF EXISTS tenant_insert ON families;
CREATE POLICY tenant_insert ON families FOR INSERT
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);
DROP POLICY IF EXISTS tenant_update ON families;
CREATE POLICY tenant_update ON families FOR UPDATE
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);
DROP POLICY IF EXISTS tenant_delete ON families;
CREATE POLICY tenant_delete ON families FOR DELETE
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);

DROP POLICY IF EXISTS tenant_read ON family_suggestion_dismissals;
CREATE POLICY tenant_read ON family_suggestion_dismissals FOR SELECT
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);
DROP POLICY IF EXISTS tenant_insert ON family_suggestion_dismissals;
CREATE POLICY tenant_insert ON family_suggestion_dismissals FOR INSERT
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);
DROP POLICY IF EXISTS tenant_delete ON family_suggestion_dismissals;
CREATE POLICY tenant_delete ON family_suggestion_dismissals FOR DELETE
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);

-- Guarded on the role existing: a local database or a CI run may apply the
-- migrations before (or without) the role from setup_app_role.sql.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON families TO app;
        GRANT SELECT, INSERT, DELETE ON family_suggestion_dismissals TO app;
    END IF;
END
$$;

COMMIT;
