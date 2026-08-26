-- uniforms (M12) — idempotency keys for the counter sale.
--
-- The counter sale is the only call in this system that moves physical goods and
-- money in ONE act. A double click at the till, or a client retrying a request
-- whose response was lost, would otherwise sell the same garment twice: two
-- deductions from the shelf, two charges to the family, one garment handed over.
--
-- The key is minted when the till drawer opens rather than when the button is
-- pressed, so a retry of the SAME sale carries the same key while a genuine
-- second sale to the next parent in the queue carries a new one.
--
-- The primary key is the deduplication: a concurrent second attempt loses the
-- insert and is reported as the replay it is. Keeping it in its own table rather
-- than a column on the order means the check costs one indexed lookup and does
-- not widen the row every read of the panel pulls.
--
-- Same shape as payments.idempotency_key in finance (migration 0002), for the
-- same reason, so there is one idea here and not two.

BEGIN;

CREATE TABLE uniform_sale_keys (
    tenant_id       uuid NOT NULL,
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 128),
    order_id        uuid NOT NULL REFERENCES uniform_orders(id) ON DELETE RESTRICT,
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, idempotency_key)
);

CREATE INDEX idx_uniform_sale_keys_order ON uniform_sale_keys (tenant_id, order_id);

ALTER TABLE uniform_sale_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE uniform_sale_keys FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON uniform_sale_keys
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
