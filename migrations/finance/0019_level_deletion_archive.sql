-- finance — finance's half of the level trash (ADR 0021).
--
-- When a level is deleted, finance moves ONLY the level's tuition plans (the
-- tabulador) here, as JSONB, and deletes them, in one transaction. Money is
-- never touched: charges, payments, deposits, receipts and concepts stay as
-- they are — no charge references a tuition plan, so none depends on a row
-- that leaves. Restoring reinserts the plans; after 7 days the hourly purge
-- drops them for good.
--
--   batch_id    one deletion (the same id in every service).
--   table_name  the row's home table, or "_batch" (one marker per batch that
--               makes archive/restore idempotent).
--   position    the step's order in the deletion (children first).
--
-- RLS: ENABLE + FORCE with the same tenant_isolation policy as every finance
-- table. Additive only.

BEGIN;

SET LOCAL lock_timeout = '5s';

CREATE TABLE finance_deletion_archive (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL,
    batch_id    uuid NOT NULL,
    table_name  text NOT NULL CHECK (length(table_name) BETWEEN 1 AND 63),
    "row"       jsonb NOT NULL,
    "position"  integer NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_finance_deletion_archive_batch
    ON finance_deletion_archive (tenant_id, batch_id, table_name);
CREATE INDEX idx_finance_deletion_archive_created
    ON finance_deletion_archive (tenant_id, created_at);
CREATE UNIQUE INDEX finance_deletion_archive_one_marker
    ON finance_deletion_archive (tenant_id, batch_id) WHERE table_name = '_batch';

ALTER TABLE finance_deletion_archive ENABLE ROW LEVEL SECURITY;
ALTER TABLE finance_deletion_archive FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON finance_deletion_archive
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
