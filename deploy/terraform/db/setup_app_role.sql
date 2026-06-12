-- One-time application-role hardening for García Lorca.
--
-- Run this ONCE per environment as an OWNER/admin connection (e.g. the Cloud SQL
-- `postgres` superuser) AFTER Terraform has created the `app` login role and the
-- `garcia_lorca` database. It encodes the non-negotiable RLS posture:
--
--   * `app` is NOT the database owner.
--   * `app` is NOT BYPASSRLS — Row Level Security policies always apply to it.
--   * `app` holds only DML on the application schema, never DDL ownership.
--
-- RLS policies themselves are SQL in /migrations (per the RLS rules); this file
-- only fixes the role attributes and baseline grants.

\set app_role app
\set db_name garcia_lorca

-- Defensive: ensure the role can never bypass RLS, even if a tool flipped it.
ALTER ROLE app NOBYPASSRLS;
ALTER ROLE app NOSUPERUSER;
ALTER ROLE app NOCREATEDB;
ALTER ROLE app NOCREATEROLE;

-- Connect + use the public schema, but not own it.
GRANT CONNECT ON DATABASE garcia_lorca TO app;
GRANT USAGE ON SCHEMA public TO app;

-- DML only. Tables are created/owned by the migration runner (a separate,
-- privileged role), so `app` can read/write rows but cannot alter structure
-- or disable RLS on a table it does not own.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO app;

-- Apply the same DML grant to tables created by future migrations.
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT USAGE, SELECT ON SEQUENCES TO app;

-- Sanity check: this must report rolbypassrls = f and rolsuper = f.
SELECT rolname, rolsuper, rolbypassrls, rolcreatedb, rolcreaterole
FROM pg_roles
WHERE rolname = 'app';
