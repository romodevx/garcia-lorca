-- students — a pupil's medical record (task 5).
--
-- Blood type, allergies and relevant conditions. Health data lives in its own
-- table, not beside the name and address, because who may read it is a
-- narrower question: it is served under medical:read/write (iam 0019), not
-- students:read.
--
-- One row per pupil per tenant, written by upsert. Absent = nothing captured.
-- The blood type is the closed ABO/Rh list or empty.
--
-- RLS: ENABLE + FORCE from this first migration, tenant-scoped like every
-- students table. The key includes tenant_id: an FK check does not pass through
-- RLS, so a row keyed on the student id alone could be claimed from another
-- tenant; the repository also refuses a pupil it cannot see.

BEGIN;

CREATE TABLE student_medical (
    tenant_id   uuid NOT NULL,
    student_id  uuid NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    blood_type  text NOT NULL DEFAULT ''
        CHECK (blood_type IN ('', 'A+', 'A-', 'B+', 'B-', 'AB+', 'AB-', 'O+', 'O-')),
    allergies   text NOT NULL DEFAULT '' CHECK (char_length(allergies) <= 2000),
    conditions  text NOT NULL DEFAULT '' CHECK (char_length(conditions) <= 2000),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, student_id)
);

ALTER TABLE student_medical ENABLE ROW LEVEL SECURITY;
ALTER TABLE student_medical FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON student_medical
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
