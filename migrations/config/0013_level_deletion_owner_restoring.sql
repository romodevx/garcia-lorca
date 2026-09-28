-- config — the level deletion's freeze gets ONE owner, and a restore in
-- flight becomes visible (ADR 0021, confirmation review).
--
-- levels.deleting_batch_id names the batch that froze the level. Only that
-- batch lifts the freeze (compare-and-set): an old batch closing — a restore
-- retried, a purge — never unfreezes a level a newer deletion holds. A frozen
-- level whose owner was lost (a pre-0013 freeze no batch claims) may be lifted
-- by any of its batches once no OTHER batch of it is deleting/failed_partial.
--
-- level_deletions grows `restoring`: a restore marks its batch (compare-and-set
-- from deleted | failed_partial | restored_partial) before its first step and
-- closes it at the end. The sweep never purges a restoring batch; one stuck
-- there longer than 30 minutes (a gateway that died mid-restore) falls back to
-- restoring_from. restoring_since is the database's clock.
--
-- Additive only; RLS already ENABLE + FORCE on both tables.

BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE levels ADD COLUMN deleting_batch_id uuid;

-- While a level is being deleted only its lifecycle may change: its status, or
-- the batch that owns the freeze. Anything else is LEVEL_BEING_DELETED.
CREATE OR REPLACE FUNCTION config_refuse_deleting_level_update() RETURNS trigger
    LANGUAGE plpgsql AS
$$
BEGIN
    IF OLD.status = 'deleting' AND NEW.status = 'deleting'
       AND (to_jsonb(NEW) - 'deleting_batch_id' - 'updated_at')
           IS DISTINCT FROM (to_jsonb(OLD) - 'deleting_batch_id' - 'updated_at') THEN
        RAISE EXCEPTION 'LEVEL_BEING_DELETED' USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END
$$;

-- Backfill: a frozen level is owned by its newest running or failed batch.
UPDATE levels l
   SET deleting_batch_id = (
       SELECT d.batch_id FROM level_deletions d
        WHERE d.tenant_id = l.tenant_id AND d.level_id = l.id
          AND d.status IN ('deleting', 'failed_partial')
        ORDER BY d.deleted_at DESC LIMIT 1)
 WHERE l.status = 'deleting';

ALTER TABLE levels
    ADD CONSTRAINT levels_owner_only_while_deleting
        CHECK (status = 'deleting' OR deleting_batch_id IS NULL);

ALTER TABLE level_deletions DROP CONSTRAINT level_deletions_status_check;
ALTER TABLE level_deletions
    ADD CONSTRAINT level_deletions_status_check CHECK (status IN (
        'deleting', 'deleted', 'failed_partial', 'restored_partial', 'restoring',
        'restored', 'cancelled', 'purged'));
ALTER TABLE level_deletions
    ADD COLUMN restoring_from  text CHECK (restoring_from IN ('deleted', 'failed_partial', 'restored_partial')),
    ADD COLUMN restoring_since timestamptz,
    ADD CONSTRAINT level_deletions_restoring_ck CHECK (
        (status = 'restoring') = (restoring_from IS NOT NULL AND restoring_since IS NOT NULL));

COMMIT;
