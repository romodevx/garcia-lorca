-- C2-015: a counter sale may be returned, whole (owner, 2026-10-19).
--
--   * the delivery goes sold_at_counter → returned, its lines sold → returned;
--   * each garment comes back to the shelf with a 'return' move; one that came
--     back damaged is written off at once with an 'adjust' move, so the ledger
--     says both that it came back and that it cannot be sold;
--   * uniform_returns keeps why, who, and how the money went back: the same
--     day the sale's payments are voided in finance ('void'); another day the
--     refund is an expense of the day it is handed back ('expense'), and a
--     closed cash-up of the sale's day is never rewritten.
BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE uniform_fulfillments DROP CONSTRAINT uniform_fulfillments_status_check;
ALTER TABLE uniform_fulfillments ADD CONSTRAINT uniform_fulfillments_status_check CHECK (status IN (
    'picking', 'ready_for_pickup', 'paid', 'dispatched', 'expired', 'cancelled', 'sold_at_counter', 'returned'
));

ALTER TABLE uniform_order_lines DROP CONSTRAINT uniform_order_lines_status_check;
ALTER TABLE uniform_order_lines ADD CONSTRAINT uniform_order_lines_status_check CHECK (status IN (
    'awaiting_stock', 'reserved', 'sold', 'released', 'cancelled', 'returned'
));

ALTER TABLE uniform_stock_moves DROP CONSTRAINT uniform_stock_moves_kind_check;
ALTER TABLE uniform_stock_moves ADD CONSTRAINT uniform_stock_moves_kind_check CHECK (kind IN (
    'receive', 'reserve', 'release', 'sell', 'adjust', 'return'
));

CREATE TABLE uniform_returns (
    fulfillment_id uuid PRIMARY KEY REFERENCES uniform_fulfillments (id) ON DELETE RESTRICT,
    tenant_id      uuid NOT NULL,
    order_id       uuid NOT NULL REFERENCES uniform_orders (id) ON DELETE RESTRICT,
    reason         text NOT NULL CHECK (btrim(reason) <> '' AND length(reason) <= 200),
    refund_mode    text NOT NULL CHECK (refund_mode IN ('void', 'expense', 'none')),
    refund_method  text NOT NULL DEFAULT '' CHECK (refund_method IN ('', 'cash', 'card', 'transfer')),
    refund_cents   bigint NOT NULL CHECK (refund_cents >= 0),
    -- done: the money side is settled; pending: the expense is still to be
    -- recorded (a retry finishes it); the delivery is flagged meanwhile.
    refund_status  text NOT NULL CHECK (refund_status IN ('done', 'pending')),
    expense_id     uuid NULL,
    damaged_units  integer NOT NULL DEFAULT 0 CHECK (damaged_units >= 0),
    actor_user_id  uuid NULL,
    actor_name     text NOT NULL DEFAULT '',
    returned_at    timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT uniform_returns_expense_shape CHECK (
        (refund_mode = 'expense') = (refund_method <> '')
        AND (expense_id IS NULL OR refund_mode = 'expense')
    )
);

CREATE INDEX idx_uniform_returns_order ON uniform_returns (tenant_id, order_id);

ALTER TABLE uniform_returns ENABLE ROW LEVEL SECURITY;
ALTER TABLE uniform_returns FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON uniform_returns
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
