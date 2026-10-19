-- students — find every move OUT of a group in one indexed read (CC-301).
--
-- A past day's class list is who sat in the group THAT day: attendance reads
-- every enrollment ever in the group and every move into or out of it. 0015
-- serves the into side; this one serves the (tenant, from_group_id) lookup,
-- both for the OR of the moves and for the moved-out enrollments' subquery.
BEGIN;

SET LOCAL lock_timeout = '5s';

CREATE INDEX IF NOT EXISTS idx_enrollment_transfers_from_group
    ON enrollment_transfers (tenant_id, from_group_id, moved_at);

COMMIT;
