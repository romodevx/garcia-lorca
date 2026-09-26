-- attendance (T3) — per-class lists and list sessions.
--
-- 1. attendance.subject_id: a list is now keyed by (group, date, subject). A NULL
--    subject is the DAILY list (one per group per day — every row written before
--    this migration keeps NULL, so no data is transformed); a subject is a
--    PER_CLASS list (one per group, subject and day). Which of the two a group
--    uses is its level's attendance_mode (config), enforced at the gateway.
--    The one-mark key widens from (tenant, enrollment, date) to
--    (tenant, enrollment, date, subject) with NULLS NOT DISTINCT (PG15+), so a
--    daily list still holds exactly one mark per student per day: two NULL
--    subjects collide instead of being "distinct unknowns".
--
-- 2. attendance_sessions: the record that a list was TAKEN. Marks are stored as
--    exceptions only, so without it an all-present day and a day nobody took
--    look identical. taken_by / taken_by_name / taken_at are the FIRST save of the
--    list; updated_at is bumped by every later correction. group_id and
--    taken_by are cross-service UUIDs (config.groups, iam.users) — no FKs, as in
--    0001. taken_by_name is a display-name snapshot so the session reads without
--    a call to iam.
--
-- RLS: same shape as `attendance` (ENABLE + FORCE, tenant policy on
-- app.current_tenant). Privileges for the app role come from the ALTER DEFAULT
-- PRIVILEGES in deploy/terraform/db/setup_app_role.sql, exactly as for
-- `attendance` (0001 grants nothing either).

BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE attendance ADD COLUMN subject_id uuid NULL;

ALTER TABLE attendance DROP CONSTRAINT attendance_tenant_id_enrollment_id_date_key;
ALTER TABLE attendance
    ADD CONSTRAINT attendance_tenant_enrollment_date_subject_key
    UNIQUE NULLS NOT DISTINCT (tenant_id, enrollment_id, date, subject_id);

CREATE TABLE attendance_sessions (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL,
    group_id      uuid NOT NULL,   -- config.groups (RPC-validated, NOT a FK)
    date          date NOT NULL,
    subject_id    uuid NULL,       -- NULL = the daily list
    taken_by      uuid NOT NULL,   -- iam.users of the first save
    taken_by_name text NOT NULL CHECK (length(taken_by_name) BETWEEN 1 AND 200),
    taken_at      timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT attendance_sessions_list_key
        UNIQUE NULLS NOT DISTINCT (tenant_id, group_id, date, subject_id)
);

ALTER TABLE attendance_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE attendance_sessions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON attendance_sessions
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
