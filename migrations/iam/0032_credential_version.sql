-- iam (security hardening, CC-302) — exact refresh-token revocation.
--
-- 0011 revokes a refresh token whose issued-at predates password_changed_at,
-- compared at whole seconds (a JWT iat is Unix seconds). Every token minted in
-- the change's own second survived — and a stolen token rotated more than once
-- a second always had one, so it outlived every password change.
--
-- users.credential_version counts the rotations: every write that stamps
-- password_changed_at bumps it, and the refresh token carries the version it
-- was minted under (claim "cv"). RefreshToken honors a token only under the
-- CURRENT version, so revocation no longer depends on any clock. Existing rows
-- start at 0, which is what a token minted before this column reads as, so the
-- first rotation after the deploy revokes those exactly too.
--
-- No new policy: users already has its tenant RLS policy, and a column added
-- to a table falls under it. Table-level grants cover the new column.
--
-- Plain SQL only (no psql variables): applied by both the psql runner and the
-- Go integration test.

BEGIN;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS credential_version bigint NOT NULL DEFAULT 0
        CHECK (credential_version >= 0);

COMMIT;
