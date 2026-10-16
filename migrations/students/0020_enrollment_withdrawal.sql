BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE enrollments
    ADD COLUMN withdrawn_on date NULL,
    ADD COLUMN withdrawal_reason text NULL
        CHECK (withdrawal_reason IS NULL OR char_length(withdrawal_reason) <= 500);

CREATE TABLE students_outbox (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL,
    event_type      text NOT NULL CHECK (length(event_type) BETWEEN 1 AND 60),
    payload         bytea NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    published_at    timestamptz NULL,
    attempts        integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_students_outbox_pending ON students_outbox (tenant_id, next_attempt_at, created_at)
    WHERE published_at IS NULL;

ALTER TABLE students_outbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE students_outbox FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON students_outbox
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON students_outbox TO app;

COMMIT;
