-- config — deleting a level, with a 7-day trash (ADR 0021).
--
-- config_deletion_archive is config's half of the trash: when a level is
-- deleted, its rows (the level, grade levels, subjects, groups, terms, term
-- months, teaching assignments and the school events aimed at it) are copied
-- here as JSONB and deleted from their tables, in one transaction. Restoring
-- reinserts them. Nothing else in config learns a "deleted" flag: a deleted row
-- is simply not in its table, so no query has to remember to filter it.
--
--   batch_id    one deletion; every service files its rows under the same id.
--   table_name  the home table of the row (or "_batch", the one marker row per
--               batch that makes archive/restore idempotent).
--   position    the step's order in the deletion (children first).
--
-- level_deletions is the trash as the director sees it: which level, who, when,
-- until when it can come back, and what it held. Status moves deleted →
-- restored, or deleted → purged once restorable_until has passed.
--
-- RLS: ENABLE + FORCE with the same tenant_isolation policy as every config
-- table. Additive only.

BEGIN;

SET LOCAL lock_timeout = '5s';

CREATE TABLE config_deletion_archive (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL,
    batch_id    uuid NOT NULL,
    table_name  text NOT NULL CHECK (length(table_name) BETWEEN 1 AND 63),
    "row"       jsonb NOT NULL,
    "position"  integer NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- Restore reads one batch table by table; purge reads by age.
CREATE INDEX idx_config_deletion_archive_batch
    ON config_deletion_archive (tenant_id, batch_id, table_name);
CREATE INDEX idx_config_deletion_archive_created
    ON config_deletion_archive (tenant_id, created_at);
-- One marker per batch: two racing first calls cannot both record it.
CREATE UNIQUE INDEX config_deletion_archive_one_marker
    ON config_deletion_archive (tenant_id, batch_id) WHERE table_name = '_batch';

ALTER TABLE config_deletion_archive ENABLE ROW LEVEL SECURITY;
ALTER TABLE config_deletion_archive FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON config_deletion_archive
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

CREATE TABLE level_deletions (
    batch_id          uuid PRIMARY KEY,
    tenant_id         uuid NOT NULL,
    level_id          uuid NOT NULL,
    level_name        text NOT NULL CHECK (length(level_name) BETWEEN 1 AND 200),
    counts            jsonb NOT NULL DEFAULT '{}'::jsonb,
    deleted_by        uuid NOT NULL,
    deleted_by_name   text NOT NULL CHECK (length(deleted_by_name) BETWEEN 1 AND 200),
    deleted_at        timestamptz NOT NULL,
    restorable_until  timestamptz NOT NULL,
    status            text NOT NULL DEFAULT 'deleted'
        CHECK (status IN ('deleted', 'restored', 'purged')),
    updated_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT level_deletions_window_ck CHECK (restorable_until > deleted_at)
);

-- The trash screen lists the live batches, newest first.
CREATE INDEX idx_level_deletions_status
    ON level_deletions (tenant_id, status, deleted_at DESC);

ALTER TABLE level_deletions ENABLE ROW LEVEL SECURITY;
ALTER TABLE level_deletions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON level_deletions
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
