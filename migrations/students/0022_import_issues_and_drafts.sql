-- students — import issues and draft pupils (PLAN-import-avisos.md, D1/D3).
--
-- student_import_issues: what the legacy import could not keep as the file
-- wrote it (a dropped phone, a birth date taken from the CURP, a missing
-- CURP...). Each row keeps the file's original_value and the stored_value, so
-- nothing is lost or changed in silence. An issue is open until the file is
-- corrected (resolution 'corrected', closed by the save that changed its field)
-- or someone with students:update says it is fine ('accepted'). The three
-- resolution columns move together (the CHECK). intended_group_id is set on a
-- draft's curp issue: the group the import meant the pupil for, so the screen
-- can offer "Inscribir en 2° A" once the CURP is captured.
--
-- Drafts: admission_status gains 'draft' — an imported pupil without a valid
-- CURP, stored with no enrollment. curp becomes NULL-able ONLY for a draft
-- (students_curp_draft_ck). UNIQUE (tenant_id, curp) is untouched: NULLs never
-- collide, so any number of drafts coexist.
--
-- RLS: ENABLE + FORCE from this first migration, one policy per operation on
-- app.current_tenant, as every students table. An FK check does not pass
-- through RLS, so the repository only writes issues inside the transaction that
-- stored (or reads) the tenant's own pupil. Re-runnable.
BEGIN;

SET LOCAL lock_timeout = '5s';

CREATE TABLE IF NOT EXISTS student_import_issues (
    id                uuid PRIMARY KEY,
    tenant_id         uuid NOT NULL,
    student_id        uuid NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    field             text NOT NULL CHECK (field IN (
                          'birth_date', 'phone_home', 'phone_mobile', 'email', 'postal_code', 'state',
                          'bank_reference', 'surnames', 'first_name', 'address', 'curp')),
    code              text NOT NULL CHECK (char_length(code) BETWEEN 1 AND 60),
    original_value    text NOT NULL DEFAULT '' CHECK (char_length(original_value) <= 500),
    stored_value      text NOT NULL DEFAULT '' CHECK (char_length(stored_value) <= 500),
    import_batch_id   uuid NULL,
    intended_group_id uuid NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    resolved_at       timestamptz NULL,
    resolved_by       uuid NULL,
    resolution        text NULL CHECK (resolution IN ('corrected', 'accepted')),
    CONSTRAINT student_import_issues_resolution_ck
        CHECK ((resolved_at IS NULL) = (resolution IS NULL) AND (resolved_by IS NULL OR resolved_at IS NOT NULL))
);

-- The open issues of a pupil: the list's count and the file's alerts.
CREATE INDEX IF NOT EXISTS idx_student_import_issues_open
    ON student_import_issues (tenant_id, student_id)
    WHERE resolved_at IS NULL;
-- Every issue of a pupil (include_resolved), and the ON DELETE CASCADE.
CREATE INDEX IF NOT EXISTS idx_student_import_issues_student
    ON student_import_issues (student_id);

ALTER TABLE student_import_issues ENABLE ROW LEVEL SECURITY;
ALTER TABLE student_import_issues FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_read ON student_import_issues;
CREATE POLICY tenant_read ON student_import_issues FOR SELECT
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);
DROP POLICY IF EXISTS tenant_insert ON student_import_issues;
CREATE POLICY tenant_insert ON student_import_issues FOR INSERT
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);
DROP POLICY IF EXISTS tenant_update ON student_import_issues;
CREATE POLICY tenant_update ON student_import_issues FOR UPDATE
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);
DROP POLICY IF EXISTS tenant_delete ON student_import_issues;
CREATE POLICY tenant_delete ON student_import_issues FOR DELETE
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);

-- Guarded on the role existing: a local database or a CI run may apply the
-- migrations before (or without) the role from setup_app_role.sql.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON student_import_issues TO app;
    END IF;
END
$$;

-- Drafts. Widening the status CHECK is additive; relaxing NOT NULL is guarded
-- by the new CHECK, which every existing row (all with a CURP) satisfies.
ALTER TABLE students DROP CONSTRAINT IF EXISTS students_admission_status_check;
ALTER TABLE students ADD CONSTRAINT students_admission_status_check
    CHECK (admission_status IN ('pending', 'accepted', 'enrolled', 'rejected', 'withdrawn', 'draft'));

ALTER TABLE students ALTER COLUMN curp DROP NOT NULL;
ALTER TABLE students DROP CONSTRAINT IF EXISTS students_curp_draft_ck;
ALTER TABLE students ADD CONSTRAINT students_curp_draft_ck
    CHECK (curp IS NOT NULL OR admission_status = 'draft');

COMMIT;
