-- iam (security hardening) — refresh-token revocation on password change.
--
-- Adds users.password_changed_at: the instant the credential was last rotated.
-- iam's RefreshToken rejects any refresh token whose issued-at predates this
-- value, so changing a password revokes every refresh token minted before it —
-- a stolen refresh token stops working the moment the user rotates their
-- password (previously it stayed valid for its full 24h TTL).
--
-- NOT NULL DEFAULT now() so existing rows get a sane floor (their creation-era
-- instant) and inserts that omit the column are stamped automatically. The app
-- role writes now() on every UpdatePassword; reads hydrate the domain User.
--
-- Plain SQL only (no psql variables): this file must apply BOTH via the psql
-- runner AND via the Go integration test, which execs the SQL through GORM.
--
-- No SET LOCAL / tenant context: ADD COLUMN is DDL, run as the table owner, and
-- row-level security never gates schema changes — it filters rows, not DDL.

BEGIN;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS password_changed_at timestamptz NOT NULL DEFAULT now();

COMMIT;
