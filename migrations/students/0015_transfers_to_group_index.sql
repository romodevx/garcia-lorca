-- students — find every move INTO a group in one indexed read (T6).
--
-- The monthly grade sheet asks, for a whole class list at once, when each pupil
-- joined the group by transfer (months before that belong to the previous
-- group). The existing index is keyed by enrollment; this one serves the
-- (tenant, to_group_id) lookup ordered by moved_at.
BEGIN;

SET LOCAL lock_timeout = '5s';

CREATE INDEX IF NOT EXISTS idx_enrollment_transfers_to_group
    ON enrollment_transfers (tenant_id, to_group_id, moved_at);

COMMIT;
