-- grades — grades' half of the level trash (ADR 0021).
--
-- When a level is deleted, grades moves its rows here — monthly grades, legacy
-- grade entries, period closures, report cards (+ lines) and the outbox rows of
-- the level's enrollments and terms — as JSONB, and deletes them from their
-- tables, in one transaction. Restoring reinserts them; after 7 days the
-- hourly purge drops them for good. No query elsewhere learns a "deleted"
-- flag: a deleted row is simply not in its table.
--
--   batch_id    one deletion (the same id in every service).
--   table_name  the row's home table, or "_batch" (one marker per batch that
--               makes archive/restore idempotent).
--   position    the step's order in the deletion (children first).
--
-- RLS: ENABLE + FORCE with the same tenant_isolation policy as every grades
-- table. Additive only.

BEGIN;

SET LOCAL lock_timeout = '5s';

CREATE TABLE grades_deletion_archive (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL,
    batch_id    uuid NOT NULL,
    table_name  text NOT NULL CHECK (length(table_name) BETWEEN 1 AND 63),
    "row"       jsonb NOT NULL,
    "position"  integer NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_grades_deletion_archive_batch
    ON grades_deletion_archive (tenant_id, batch_id, table_name);
CREATE INDEX idx_grades_deletion_archive_created
    ON grades_deletion_archive (tenant_id, created_at);
CREATE UNIQUE INDEX grades_deletion_archive_one_marker
    ON grades_deletion_archive (tenant_id, batch_id) WHERE table_name = '_batch';

ALTER TABLE grades_deletion_archive ENABLE ROW LEVEL SECURITY;
ALTER TABLE grades_deletion_archive FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON grades_deletion_archive
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
