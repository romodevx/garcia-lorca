-- comms (M10) initial schema — notification feed, push devices, idempotency ledger.
--
-- RLS is tenant-scoped (same posture as iam/students/grades/attendance, ADR 0001):
-- policy filters on app.current_tenant, ENABLE + FORCE so it applies even to the
-- table owner. The application role is NOT the owner and NOT BYPASSRLS.
--
-- student_id (students.students) and user_id (iam.users, the future guardian) are
-- cross-service UUIDs validated by RPC at the use-case layer — deliberately NO
-- foreign keys (they live in the students/iam services' databases logically).
--
-- processed_events is the idempotency ledger: Pub/Sub delivers at-least-once, so a
-- redelivered event must not produce a second notification. The consumer inserts
-- (tenant_id, event_key) inside the handling transaction; the composite primary key
-- makes the duplicate insert fail and the redelivery a no-op.
--
-- Run by a privileged migration role. gen_random_uuid() is in core since PG13.

BEGIN;

CREATE TABLE notifications (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL,
    student_id    uuid NOT NULL,   -- students.students (RPC-validated, NOT a FK)
    type          text NOT NULL,   -- source event, e.g. 'AbsenceRecorded', 'PaymentRegistered'
    title         text NOT NULL,
    body          text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    read_at       timestamptz NULL   -- NULL ⇒ unread (the badge count)
);

CREATE INDEX idx_notifications_student ON notifications (tenant_id, student_id);
CREATE INDEX idx_notifications_created ON notifications (tenant_id, created_at);

CREATE TABLE devices (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL,
    user_id       uuid NOT NULL,   -- iam.users, future guardian (RPC-validated, NOT a FK)
    fcm_token     text NOT NULL,
    -- '' is the unknown/unreported platform: registration must not fail on it.
    platform      text NOT NULL DEFAULT '' CHECK (platform IN ('', 'ios', 'android', 'web')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    -- One row per token (the re-registration/upsert key): a device that reinstalls
    -- gets a new token, and the same token never fans out twice.
    UNIQUE (tenant_id, fcm_token)
);

CREATE INDEX idx_devices_user ON devices (tenant_id, user_id);

CREATE TABLE processed_events (
    tenant_id     uuid NOT NULL,
    event_key     text NOT NULL,   -- '<event type>:<event id>'
    processed_at  timestamptz NOT NULL DEFAULT now(),
    -- The idempotency key itself: a redelivered event conflicts and is skipped.
    PRIMARY KEY (tenant_id, event_key)
);

-- Row Level Security (tenant isolation). Unset app.current_tenant ⇒ zero rows.
ALTER TABLE notifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE notifications FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON notifications
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE devices ENABLE ROW LEVEL SECURITY;
ALTER TABLE devices FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON devices
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE processed_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE processed_events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON processed_events
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
