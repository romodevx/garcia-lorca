-- attendance — mark revision (the version of a stored mark).
--
-- A correction updates a mark IN PLACE, keeping its id, so the id alone cannot
-- tell two changes of the same mark apart. comms dedupes events on
-- (record_id, version): without a version, late → excused, or the second absence
-- of absent → excused → absent, reused an already-claimed key and the family was
-- never told. `revision` is 1 when a row is inserted and the reconcile upsert
-- bumps it on every in-place write; the upsert's RETURNING reports it, and it
-- travels on AttendanceMarked/AbsenceRecorded as mark_version.
--
-- ADD COLUMN with a constant DEFAULT is metadata-only (PG11+): existing rows read
-- as revision 1 with no table rewrite. RLS is unchanged: the column lives on the
-- already-protected `attendance` table.

BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE attendance
    ADD COLUMN revision bigint NOT NULL DEFAULT 1
        CONSTRAINT attendance_revision_positive CHECK (revision >= 1);

COMMIT;
