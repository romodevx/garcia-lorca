-- students — the legacy import (ADR 0033, PLAN-student-import.md).
--
-- bank_reference: the pupil's BBVA reference ("Matrícula Interna" in the
-- previous system, O1). Optional; when present it is unique per school, so a
-- deposit names exactly one pupil. The partial index lets any number of pupils
-- have none. The matrícula (enrollment_number) is untouched.
--
-- import_batch_id: the import a pupil came in (one uuid per commit), for audit.
-- NULL for every pupil captured by hand.
--
-- enrollments.origin: how a placement was made. 'import' means the pupil was
-- already enrolled and paid in the previous system: finance never bills its
-- inscription (ADR 0033). Every existing placement is 'manual'.
--
-- students and enrollments are already ENABLE + FORCE RLS (0001, 0002): the new
-- columns are covered by the existing tenant_isolation policies. Additive only.
BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE students ADD COLUMN IF NOT EXISTS bank_reference text NULL;
ALTER TABLE students ADD COLUMN IF NOT EXISTS import_batch_id uuid NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_students_bank_reference
    ON students (tenant_id, bank_reference)
    WHERE bank_reference IS NOT NULL;

ALTER TABLE enrollments ADD COLUMN IF NOT EXISTS origin text NOT NULL DEFAULT 'manual';
ALTER TABLE enrollments ADD CONSTRAINT enrollments_origin_ck CHECK (origin IN ('manual', 'import'));

COMMIT;
