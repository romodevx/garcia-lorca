-- students — a note can be edited, and every edit keeps the text it replaced (T6).
--
-- 0012 made notes append-only: a wrong one was voided. The school also needs to
-- correct a note ("fiebre de 38", not "fiebre"), from the pupil's sheet, the
-- list and the payment counter. An edit replaces `body`, and the text it
-- replaced is kept in student_note_revisions with who replaced it and when, so
-- what the school once wrote about a child can still be accounted for.
--
-- updated_at / updated_by / updated_by_name are the LAST edit (NULL / '' until
-- the note is edited). updated_by_name is the editor's name AS IT READ then,
-- like author_name. Who may edit is the domain's rule: the author at any time,
-- anyone else's note only a holder of students:notes_void (the owner).
--
-- Revisions are append-only, and the database says so: RLS has a SELECT and an
-- INSERT policy only, and UPDATE and DELETE are revoked from the app role (the
-- default privileges grant them at CREATE TABLE), so a rewrite is a loud
-- privilege error rather than a silent no-op — the uniforms ledger's pattern
-- (uniforms 0001). Rows leave
-- only with their note (ON DELETE CASCADE: referential actions are not subject
-- to RLS). Nothing reads them yet — they are the audit trail for the owner.
-- RLS: ENABLE + FORCE from this first migration, tenant_id on every row.
--
-- Additive only: existing notes are untouched (they read as never edited).
BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE student_notes ADD COLUMN IF NOT EXISTS updated_at      timestamptz NULL;
ALTER TABLE student_notes ADD COLUMN IF NOT EXISTS updated_by      uuid NULL;
ALTER TABLE student_notes ADD COLUMN IF NOT EXISTS updated_by_name text NOT NULL DEFAULT '';

ALTER TABLE student_notes DROP CONSTRAINT IF EXISTS student_notes_edit_ck;
ALTER TABLE student_notes ADD CONSTRAINT student_notes_edit_ck
    CHECK ((updated_at IS NULL) = (updated_by IS NULL));

CREATE TABLE IF NOT EXISTS student_note_revisions (
    id              uuid PRIMARY KEY,
    tenant_id       uuid NOT NULL,
    note_id         uuid NOT NULL REFERENCES student_notes(id) ON DELETE CASCADE,
    previous_body   text NOT NULL,
    edited_by       uuid NOT NULL,
    edited_by_name  text NOT NULL DEFAULT '',
    edited_at       timestamptz NOT NULL
);

-- One note's history of edits, newest first.
CREATE INDEX IF NOT EXISTS idx_student_note_revisions_note
    ON student_note_revisions (tenant_id, note_id, edited_at DESC);

ALTER TABLE student_note_revisions ENABLE ROW LEVEL SECURITY;
ALTER TABLE student_note_revisions FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_read ON student_note_revisions;
CREATE POLICY tenant_read ON student_note_revisions FOR SELECT
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);
DROP POLICY IF EXISTS tenant_append ON student_note_revisions;
CREATE POLICY tenant_append ON student_note_revisions FOR INSERT
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);
-- Guarded on the role existing: a local database or a CI run may apply the
-- migrations before (or without) the role from setup_app_role.sql.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app') THEN
        REVOKE UPDATE, DELETE ON student_note_revisions FROM app;
    END IF;
END
$$;

COMMIT;
