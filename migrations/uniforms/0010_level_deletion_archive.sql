-- uniforms — its half of the level-deletion trash (ADR 0021).
--
-- When a level is deleted, uniforms files a snapshot of each garment's level before the
-- garment is left "unresolved" (level_id NULL beside the legacy key; ADR 0014) here as JSONB, inside one
-- tenant-bound transaction, and restoring reinserts them. Rows filed under one
-- batch_id; table_name is the home table (or "_batch", the one marker row per
-- batch that makes archive/restore idempotent); position is the step order.
-- Purged after 7 days by the service's hourly sweep.
--
-- RLS: ENABLE + FORCE with the same tenant_isolation policy as every
-- uniforms table. Additive only.

BEGIN;

SET LOCAL lock_timeout = '5s';

CREATE TABLE uniforms_deletion_archive (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL,
    batch_id    uuid NOT NULL,
    table_name  text NOT NULL CHECK (length(table_name) BETWEEN 1 AND 63),
    "row"       jsonb NOT NULL,
    "position"  integer NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- Restore reads one batch table by table; purge reads by age.
CREATE INDEX idx_uniforms_deletion_archive_batch
    ON uniforms_deletion_archive (tenant_id, batch_id, table_name);
CREATE INDEX idx_uniforms_deletion_archive_created
    ON uniforms_deletion_archive (tenant_id, created_at);
-- One marker per batch: two racing first calls cannot both record it.
CREATE UNIQUE INDEX uniforms_deletion_archive_one_marker
    ON uniforms_deletion_archive (tenant_id, batch_id) WHERE table_name = '_batch';

ALTER TABLE uniforms_deletion_archive ENABLE ROW LEVEL SECURITY;
ALTER TABLE uniforms_deletion_archive FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON uniforms_deletion_archive
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
