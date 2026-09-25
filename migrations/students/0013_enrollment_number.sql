-- students — every pupil's matrícula (task 13).
--
-- <year>-<five digits>, the year the pupil was registered, one run per school
-- and year, given when the pupil is stored. It is what the virtual card and the
-- office show; the card's QR carries none of it (an opaque token, 0014).
--
-- A counter row per (tenant, year), bumped with INSERT … ON CONFLICT … RETURNING
-- in the insert's own transaction: a SEQUENCE is per database and ignores RLS.
-- Existing pupils are numbered here in registration order (created_at, UTC).
-- students is FORCE RLS and its owner has no BYPASSRLS, so SET LOCAL binds the
-- one school this deployment serves (as 0006 did); the backfill is partitioned
-- by tenant all the same, for a role that bypasses RLS.
BEGIN;

-- The backfill and the constraints lock students; never wait long behind a
-- read the previous revision is running (as 0006 does).
SET LOCAL lock_timeout = '5s';
SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

CREATE TABLE student_number_counters (
    tenant_id uuid NOT NULL,
    year      integer NOT NULL CHECK (year BETWEEN 2000 AND 3000),
    next_seq  integer NOT NULL DEFAULT 1 CHECK (next_seq >= 1),
    PRIMARY KEY (tenant_id, year)
);

ALTER TABLE student_number_counters ENABLE ROW LEVEL SECURITY;
ALTER TABLE student_number_counters FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON student_number_counters
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE students ADD COLUMN enrollment_number text;

WITH numbered AS (
    SELECT id,
           EXTRACT(YEAR FROM created_at AT TIME ZONE 'UTC')::int AS year,
           row_number() OVER (
               PARTITION BY tenant_id, EXTRACT(YEAR FROM created_at AT TIME ZONE 'UTC')
               ORDER BY created_at, id) AS seq
      FROM students
)
UPDATE students s
   SET enrollment_number = lpad(n.year::text, 4, '0') || '-' || lpad(n.seq::text, 5, '0')
  FROM numbered n
 WHERE s.id = n.id;

INSERT INTO student_number_counters (tenant_id, year, next_seq)
SELECT tenant_id, EXTRACT(YEAR FROM created_at AT TIME ZONE 'UTC')::int, count(*) + 1
  FROM students
 GROUP BY tenant_id, EXTRACT(YEAR FROM created_at AT TIME ZONE 'UTC');

-- A writer that does not know the column — the previous revision, mid-deploy —
-- still stores a pupil: the number is given here when it comes without one.
-- The application gives it itself (the same counter), so this only ever fills
-- the gap.
CREATE FUNCTION students_assign_enrollment_number() RETURNS trigger
LANGUAGE plpgsql
SET search_path = public, pg_temp
AS $$
DECLARE
    y   integer;
    seq integer;
BEGIN
    IF NEW.enrollment_number IS NULL THEN
        y := EXTRACT(YEAR FROM now() AT TIME ZONE 'UTC')::int;
        INSERT INTO student_number_counters (tenant_id, year, next_seq)
        VALUES (NEW.tenant_id, y, 2)
        ON CONFLICT (tenant_id, year)
        DO UPDATE SET next_seq = student_number_counters.next_seq + 1
        RETURNING next_seq - 1 INTO seq;
        NEW.enrollment_number := lpad(y::text, 4, '0') || '-' || lpad(seq::text, 5, '0');
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER students_enrollment_number
    BEFORE INSERT ON students
    FOR EACH ROW EXECUTE FUNCTION students_assign_enrollment_number();

ALTER TABLE students ALTER COLUMN enrollment_number SET NOT NULL;
ALTER TABLE students ADD CONSTRAINT students_enrollment_number_shape CHECK (enrollment_number ~ '^[0-9]{4}-[0-9]{5}$');
ALTER TABLE students ADD CONSTRAINT students_enrollment_number_unique UNIQUE (tenant_id, enrollment_number);

COMMIT;
