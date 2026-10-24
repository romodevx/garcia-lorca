-- students — families from an import (PLAN-familias-importacion.md, ADR 0041).
--
-- family_import_links: which import (batch_id = students.import_batch_id of
-- the commit) put which pupil in which family, and whether the import created
-- that family. It is what "Deshacer" undoes: each linked pupil still in that
-- family leaves it, and a family the import created that is left with fewer
-- than two pupils is dissolved. One row per (batch, pupil): building twice
-- records nothing new.
--
-- The composite FKs keep a link inside its tenant even for a writer that
-- bypasses RLS. ON DELETE CASCADE deletes the link with its family or pupil
-- (a dissolved family has nothing left to undo); being a DELETE, not a SET
-- NULL, it never touches tenant_id.
--
-- RLS: ENABLE + FORCE from this first migration, one policy per operation on
-- app.current_tenant, as every students table. No UPDATE: a link is written
-- once and deleted. Re-runnable.
BEGIN;

SET LOCAL lock_timeout = '5s';

CREATE TABLE IF NOT EXISTS family_import_links (
    id             uuid PRIMARY KEY,
    tenant_id      uuid NOT NULL,
    batch_id       uuid NOT NULL,
    family_id      uuid NOT NULL,
    student_id     uuid NOT NULL,
    created_family boolean NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, batch_id, student_id)
);

-- Outside CREATE TABLE so a re-run converges on the composite FKs.
ALTER TABLE family_import_links DROP CONSTRAINT IF EXISTS family_import_links_family_fk;
ALTER TABLE family_import_links ADD CONSTRAINT family_import_links_family_fk
    FOREIGN KEY (tenant_id, family_id) REFERENCES families (tenant_id, id) ON DELETE CASCADE;
ALTER TABLE family_import_links DROP CONSTRAINT IF EXISTS family_import_links_student_fk;
ALTER TABLE family_import_links ADD CONSTRAINT family_import_links_student_fk
    FOREIGN KEY (tenant_id, student_id) REFERENCES students (tenant_id, id) ON DELETE CASCADE;

-- An undo reads one batch's links by (tenant_id, batch_id): the UNIQUE's own
-- index leads with exactly those columns, so a second one would only cost
-- writes.
--
-- The cascades from families and students look links up by these.
CREATE INDEX IF NOT EXISTS idx_family_import_links_family
    ON family_import_links (tenant_id, family_id);
CREATE INDEX IF NOT EXISTS idx_family_import_links_student
    ON family_import_links (tenant_id, student_id);

ALTER TABLE family_import_links ENABLE ROW LEVEL SECURITY;
ALTER TABLE family_import_links FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_read ON family_import_links;
CREATE POLICY tenant_read ON family_import_links FOR SELECT
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);
DROP POLICY IF EXISTS tenant_insert ON family_import_links;
CREATE POLICY tenant_insert ON family_import_links FOR INSERT
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);
DROP POLICY IF EXISTS tenant_delete ON family_import_links;
CREATE POLICY tenant_delete ON family_import_links FOR DELETE
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);

-- Guarded on the role existing: a local database or a CI run may apply the
-- migrations before (or without) the role from setup_app_role.sql.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app') THEN
        GRANT SELECT, INSERT, DELETE ON family_import_links TO app;
    END IF;
END
$$;

COMMIT;
