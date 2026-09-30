-- students — students' half of the level trash (ADR 0021).
--
-- Deleting a level archives, here, the enrollments in its groups (and their
-- transfer history) as JSONB and deletes them; the pupils themselves are KEPT:
-- only their level reference is cleared ("sin nivel, grado ni grupo"), and the
-- previous values are filed as a "students#level" snapshot so a restore can put
-- them back — unless the pupil was given another level meanwhile.
--
--   batch_id    one deletion; every service files its rows under the same id.
--   table_name  the home table of the row, "students#level" for the snapshot,
--               or "_batch" (the one marker row per batch).
--   position    the step's order in the deletion (children first).
--
-- RLS: ENABLE + FORCE with the same tenant_isolation policy as every students
-- table. Additive only.

BEGIN;

SET LOCAL lock_timeout = '5s';

CREATE TABLE students_deletion_archive (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL,
    batch_id    uuid NOT NULL,
    table_name  text NOT NULL CHECK (length(table_name) BETWEEN 1 AND 63),
    "row"       jsonb NOT NULL,
    "position"  integer NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- Restore reads one batch table by table; purge reads by age.
CREATE INDEX idx_students_deletion_archive_batch
    ON students_deletion_archive (tenant_id, batch_id, table_name);
CREATE INDEX idx_students_deletion_archive_created
    ON students_deletion_archive (tenant_id, created_at);
-- One marker per batch: two racing first calls cannot both record it.
CREATE UNIQUE INDEX students_deletion_archive_one_marker
    ON students_deletion_archive (tenant_id, batch_id) WHERE table_name = '_batch';

ALTER TABLE students_deletion_archive ENABLE ROW LEVEL SECURITY;
ALTER TABLE students_deletion_archive FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON students_deletion_archive
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
