-- attendance — the group whose list stored a mark.
--
-- A transfer rewrites enrollments.group_id IN PLACE (students keeps the move in
-- enrollment_transfers), so one enrollment can sit on two groups' rosters for
-- the same past day: group A's list before the move, and — edited afterwards —
-- group B's. Without knowing which list stored a mark, B's retroactive save of a
-- day before the move saw A's absence as its own and cleared it (publishing a
-- bogus correction to the family).
--
-- Rule (enforced by the repository, documented in repo/postgres.go):
--   * a mark belongs to the list (group) that stored it; a list reads and
--     reconciles only its own marks and legacy ones (group_id IS NULL);
--   * the one-mark key stays (tenant, enrollment, date, subject): a pupil·day·
--     subject held by another group's list is frozen for this list — never
--     cleared, updated nor re-marked from it. The first list that stored an
--     exception owns it; the family view reads it by enrollment either way.
--
-- group_id is a cross-service UUID (config.groups), NO FK, as in 0001/0002.
-- Legacy rows keep NULL (no backfill: which list stored them is unknowable);
-- the first correction of one stamps the correcting list's group.
--
-- ADD COLUMN without a default is metadata-only. RLS is unchanged: the column
-- lives on the already-protected `attendance` table. The index serves the
-- list-scoped reads (enrollment, date, group).

BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE attendance ADD COLUMN group_id uuid NULL;

CREATE INDEX idx_attendance_group_date ON attendance (tenant_id, group_id, date);

COMMIT;
