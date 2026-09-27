-- grades — the outbox: a report card's announcement is not lost because the
-- broker was not there (T7 fix, mirrors finance/0012).
--
-- WHAT WAS WRONG. Closing a period committed its report cards and THEN
-- published GradesPublished per card, best-effort, error logged and dropped.
-- A broker that was down, or a container that died between the commit and the
-- publish, lost the event for good: the family never heard "boleta
-- disponible", and nothing retried or noticed.
--
-- WHAT THIS IS. The events are written HERE, in the same transaction as the
-- closure and its report cards: all land or none do. A relay publishes them
-- (right after the commit, and on a periodic sweep), marks the ones that went
-- out, and pushes a failed one's next attempt out with a growing backoff.
--
-- PAYLOAD IS JSON, not proto bytes: what belongs in a durable queue is the
-- fact, readable with psql by whoever debugs a stuck row.
BEGIN;

SET LOCAL lock_timeout = '5s';

CREATE TABLE grades_outbox (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL,
    event_type      text NOT NULL CHECK (length(event_type) BETWEEN 1 AND 60),
    payload         jsonb NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    published_at    timestamptz NULL,
    -- How many times publishing was tried; a count that keeps climbing points
    -- at the broker, not at the period.
    attempts        integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    -- When the relay may try again: now() on insert, pushed out on failure.
    next_attempt_at timestamptz NOT NULL DEFAULT now()
);

-- The relay reads exactly this: the unpublished rows that are due, oldest
-- first. Partial, so the index does not grow with every event ever emitted.
CREATE INDEX idx_grades_outbox_pending ON grades_outbox (tenant_id, next_attempt_at, created_at)
    WHERE published_at IS NULL;

ALTER TABLE grades_outbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE grades_outbox FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON grades_outbox
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
