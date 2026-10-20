-- C2-015: a uniforms counter sale returned on the day it was sold has its
-- payments voided. A sale's payments are loose rows (deposit_id NULL) with no
-- void columns, and every reader of `payments` counts them as money; so the
-- void moves them OUT of `payments` and keeps each one here, with why, who and
-- when. The cash-up lists them as cancelled on their own day, and a closed
-- day sees the change (DayChangedSince).
BEGIN;

SET LOCAL lock_timeout = '5s';

CREATE TABLE voided_sale_payments (
    id              uuid PRIMARY KEY,           -- the voided payment's own id
    tenant_id       uuid NOT NULL,
    charge_id       uuid NOT NULL REFERENCES charges (id) ON DELETE RESTRICT,
    amount_cents    bigint NOT NULL CHECK (amount_cents > 0),
    method          text NOT NULL,
    card_type       text NOT NULL DEFAULT '',
    reference       text NOT NULL DEFAULT '',
    idempotency_key text NOT NULL DEFAULT '',
    received_by     uuid NULL,
    paid_at         timestamptz NOT NULL,
    paid_on         date NOT NULL,
    reason          text NOT NULL CHECK (btrim(reason) <> '' AND char_length(reason) <= 500),
    voided_by       uuid NULL,
    voided_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_voided_sale_payments_day ON voided_sale_payments (tenant_id, paid_on);
CREATE INDEX idx_voided_sale_payments_charge ON voided_sale_payments (tenant_id, charge_id);

ALTER TABLE voided_sale_payments ENABLE ROW LEVEL SECURITY;
ALTER TABLE voided_sale_payments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON voided_sale_payments
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
