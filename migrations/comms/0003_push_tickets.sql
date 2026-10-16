-- Expo's push tickets bridge the send response and its asynchronous receipt.
-- The device token is retained only until a receipt is handled or retries end.
BEGIN;

CREATE TABLE push_tickets (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL,
    ticket_id     text NOT NULL,
    device_token  text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    checked_at    timestamptz NULL,
    attempts      integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    UNIQUE (tenant_id, ticket_id)
);

CREATE INDEX idx_push_tickets_receipt ON push_tickets (tenant_id, created_at, id);

ALTER TABLE push_tickets ENABLE ROW LEVEL SECURITY;
ALTER TABLE push_tickets FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON push_tickets
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
