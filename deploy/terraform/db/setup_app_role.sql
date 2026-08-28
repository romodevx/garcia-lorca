-- One-time application-role hardening for García Lorca.
--
-- Run this ONCE per environment as an ADMIN connection (Cloud SQL's `postgres`
-- role, or the owner role your provider hands you) AFTER the `app` login role
-- and the `garcia_lorca` database exist. It encodes the non-negotiable RLS
-- posture:
--
--   * `app` is NOT the database owner.
--   * `app` is NOT BYPASSRLS — Row Level Security policies always apply to it.
--   * `app` holds only DML on the application schema, never DDL ownership.
--
-- RLS policies themselves are SQL in /migrations (per the RLS rules); this file
-- only fixes the role attributes and baseline grants.
--
-- ---------------------------------------------------------------------------
-- WHY THIS ASSERTS INSTEAD OF SETTING
--
-- This used to open with four unconditional statements:
--
--   ALTER ROLE app NOBYPASSRLS;
--   ALTER ROLE app NOSUPERUSER;
--
-- Changing the BYPASSRLS or SUPERUSER attributes requires a REAL superuser.
-- Managed Postgres never gives you one: Cloud SQL's `postgres` is
-- `cloudsqlsuperuser` with rolsuper = false, and hosted providers are the same.
-- So both statements errored, and because the migration runner uses
-- ON_ERROR_STOP=1 the script aborted on line 18 — BEFORE a single GRANT ran.
-- The `app` role ended up with no privileges at all and every query returned
-- "permission denied for table", which reads like a broken application rather
-- than a setup script that stopped a third of the way in.
--
-- The intent was always defensive ("make sure nothing flipped this"), and an
-- assertion expresses that intent exactly — while working on a connection that
-- is not a superuser. A freshly created login role is NOBYPASSRLS/NOSUPERUSER by
-- default, so on a clean environment this passes silently. If something ever DID
-- flip it, this fails loudly, which is the whole point. And on the rare
-- connection that IS a real superuser, it repairs the role instead of complaining.
-- ---------------------------------------------------------------------------

DO $$
DECLARE
  r         record;
  am_super  boolean := current_setting('is_superuser') = 'on';
BEGIN
  SELECT rolsuper, rolbypassrls, rolcreatedb, rolcreaterole
    INTO r
    FROM pg_roles
   WHERE rolname = 'app';

  IF NOT FOUND THEN
    RAISE EXCEPTION 'role "app" does not exist. Create it (Terraform google_sql_user.app, or your provider console) before running this script.';
  END IF;

  IF NOT (r.rolbypassrls OR r.rolsuper OR r.rolcreatedb OR r.rolcreaterole) THEN
    RAISE NOTICE 'role "app" posture is correct (no BYPASSRLS, no SUPERUSER, no CREATEDB, no CREATEROLE)';
  ELSIF am_super THEN
    EXECUTE 'ALTER ROLE app NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE';
    RAISE NOTICE 'role "app" held privileges it must not have; tightened them';
  ELSE
    RAISE EXCEPTION 'role "app" holds privileges it must not have (bypassrls=%, superuser=%, createdb=%, createrole=%). RLS is not enforceable against it; repairing this needs a real superuser connection.', r.rolbypassrls, r.rolsuper, r.rolcreatedb, r.rolcreaterole;
  END IF;
END
$$;

-- Connect + use the public schema, but not own it.
GRANT CONNECT ON DATABASE garcia_lorca TO app;
GRANT USAGE ON SCHEMA public TO app;

-- DML only. Tables are created/owned by the migration runner (a separate,
-- privileged role), so `app` can read/write rows but cannot alter structure
-- or disable RLS on a table it does not own.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO app;

-- Apply the same DML grant to tables created by future migrations.
--
-- ALTER DEFAULT PRIVILEGES IS PER-GRANTOR. It only affects objects created by
-- the role that runs this statement. If this script runs as one role and
-- migrations/**.sql run as another, `app` inherits NOTHING and every query fails
-- with "permission denied for table" the first time a new table is used.
-- scripts/migrate-cloud.sh therefore runs BOTH steps on one connection, as one
-- role. If you run them by hand, do the same.
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT USAGE, SELECT ON SEQUENCES TO app;

-- Human-readable confirmation. The DO block above is what enforces it; this is
-- so whoever runs the script can see the posture with their own eyes.
SELECT rolname, rolsuper, rolbypassrls, rolcreatedb, rolcreaterole
FROM pg_roles
WHERE rolname = 'app';
