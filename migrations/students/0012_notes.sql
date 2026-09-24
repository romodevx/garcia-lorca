-- students — notes on a pupil (task 6).
--
-- Staff write short notes about a pupil ("llegó con fiebre; se avisó a la
-- mamá"). A note is never edited: a history that can be rewritten is not a
-- history. A wrong one is VOIDED — voided_at/voided_by set, the row kept — so
-- what the school once wrote about a child can always be accounted for. The
-- author may void within 15 minutes; after that only whoever holds
-- students:notes_void (the owner, iam 0020). Those rules live in the domain;
-- the CHECK below only keeps the two void columns consistent.
--
-- author_name is the author's name AS IT READ when the note was written, so the
-- history does not change when an account is renamed or removed.
--
-- RLS: ENABLE + FORCE from this first migration. tenant_id is part of every
-- read path; the repository also refuses a pupil the tenant cannot see, since
-- an FK check does not pass through RLS.

BEGIN;

CREATE TABLE student_notes (
    id              uuid PRIMARY KEY,
    tenant_id       uuid NOT NULL,
    student_id      uuid NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    body            text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 2000),
    author_user_id  uuid NOT NULL,
    author_name     text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL,
    voided_at       timestamptz NULL,
    voided_by       uuid NULL,
    CONSTRAINT student_notes_void_ck CHECK ((voided_at IS NULL) = (voided_by IS NULL))
);

-- The history of one pupil, newest first.
CREATE INDEX idx_student_notes_student ON student_notes (tenant_id, student_id, created_at DESC);

ALTER TABLE student_notes ENABLE ROW LEVEL SECURITY;
ALTER TABLE student_notes FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON student_notes
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
