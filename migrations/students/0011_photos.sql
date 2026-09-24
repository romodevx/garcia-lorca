-- students — where a pupil's photo is stored (task 5).
--
-- The photo itself lives in a PRIVATE object store (a GCS bucket; fake-gcs
-- locally). This table keeps only the reference: the object key, the content
-- type the bytes proved to be (JPEG/PNG) and the size. The bucket is never
-- exposed; the gateway serves the bytes after its own permission check.
--
-- One photo per pupil per tenant. RLS: ENABLE + FORCE, tenant-scoped; the key
-- includes tenant_id and the repository refuses a pupil the tenant cannot see
-- (an FK check does not pass through RLS).

BEGIN;

CREATE TABLE student_photos (
    tenant_id    uuid NOT NULL,
    student_id   uuid NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    object_key   text NOT NULL CHECK (object_key <> ''),
    content_type text NOT NULL CHECK (content_type IN ('image/jpeg', 'image/png')),
    size_bytes   integer NOT NULL CHECK (size_bytes > 0 AND size_bytes <= 2097152),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, student_id)
);

ALTER TABLE student_photos ENABLE ROW LEVEL SECURITY;
ALTER TABLE student_photos FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON student_photos
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
