#!/bin/sh
# Local Postgres bootstrap for García Lorca. Runs ONCE on first volume init
# (docker-entrypoint-initdb.d), as the superuser. Mirrors the production RLS
# posture from deploy/terraform/db/setup_app_role.sql:
#
#   * `app` is the application login role used by every service.
#   * `app` is NOT the database owner.
#   * `app` is NOT BYPASSRLS — Row Level Security always applies to it.
#   * `app` gets DML only; DDL/ownership stays with the superuser/migrator.
#
# The password comes from $APP_DB_PASSWORD (compose passes it from .env), so no
# credential is ever hardcoded in a committed file.
set -e

: "${APP_DB_PASSWORD:?APP_DB_PASSWORD must be set (see .env.example)}"

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<SQL
CREATE ROLE app LOGIN PASSWORD '${APP_DB_PASSWORD}' NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE;

GRANT CONNECT ON DATABASE ${POSTGRES_DB} TO app;
GRANT USAGE ON SCHEMA public TO app;

-- DML only on current + future tables (migrations create/own the tables).
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT USAGE, SELECT ON SEQUENCES TO app;

-- Sanity: must be rolsuper=f, rolbypassrls=f.
DO \$\$
DECLARE bypass boolean; super boolean;
BEGIN
  SELECT rolbypassrls, rolsuper INTO bypass, super FROM pg_roles WHERE rolname = 'app';
  IF bypass OR super THEN
    RAISE EXCEPTION 'app role must be NOBYPASSRLS and NOSUPERUSER (got bypassrls=%, super=%)', bypass, super;
  END IF;
END \$\$;
SQL

echo "[postgres-init] app role created (NOBYPASSRLS, NOSUPERUSER)"
