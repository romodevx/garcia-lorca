-- finance (M16) — el outbox: que un evento no se pierda porque el broker no
-- estaba.
--
-- WHAT WAS WRONG. A payment committed and THEN published its event, best-effort,
-- error ignored. That order is right — refusing money because a broker is down
-- would be worse — but the event had no second chance. If the broker was down,
-- or the container died in the milliseconds between the commit and the publish,
-- the event was gone. Nothing retried it and nothing detected it.
--
-- WHAT THAT COST. students turns a pupil from "en trámite" into "inscrito" when
-- it hears ChargeSettled on the inscription charge. Lose that event and the
-- family has paid, the money is in the ledger, the cash-up counts it — and the
-- pupil stays en trámite forever. There is no reconciliation job to notice. You
-- find out when a parent complains.
--
-- WHAT THIS IS. The event is written HERE, in the same transaction as the
-- payment: either both land or neither does. A separate pass publishes and marks
-- them, and a row that fails simply stays unpublished and is tried again.
--
-- PAYLOAD IS JSON, not proto bytes. The bytes on the wire are the adapter's
-- business; what belongs in a durable queue is the fact, in a form a person
-- debugging a stuck row at 9pm can read with psql.
BEGIN;

CREATE TABLE finance_outbox (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL,
    event_type   text NOT NULL CHECK (length(event_type) BETWEEN 1 AND 60),
    payload      jsonb NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz NULL,
    -- How many times publishing was tried. A row whose count keeps climbing is
    -- the signal that something is wrong with the broker, not with the payment.
    attempts     integer NOT NULL DEFAULT 0
);

-- The drain reads exactly this: the unpublished ones, oldest first. Partial, so
-- the index does not grow with every event the school ever emitted.
CREATE INDEX idx_finance_outbox_pending ON finance_outbox (tenant_id, created_at)
    WHERE published_at IS NULL;

ALTER TABLE finance_outbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE finance_outbox FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON finance_outbox
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
