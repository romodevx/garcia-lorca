-- finance (ADR 0040) — payment vouchers (comprobantes de pago) sent from the
-- family app.
--
-- THE PROBLEM. A family pays by bank transfer and has no way to tell the
-- school; the office finds out weeks later, or never, and the month keeps
-- running surcharges on money already sent.
--
-- THE SHAPE.
--
--   * payment_vouchers: one file (photo or PDF of the transfer) a guardian
--     sent for one pupil, with what they SAY they paid and when — both
--     optional and never money: the office confirms the amount and registers
--     the payment through the existing deposit path. The file lives in the
--     PRIVATE receipts bucket (prefix vouchers/); this row holds the key, the
--     type the bytes proved to be, the size and the SHA-256.
--     pending -> applied (linked to the deposit that registered it) or
--     rejected (with the reason the family reads). A void hands an applied
--     voucher back to pending and counts it in release_count, so its next
--     application derives an idempotency key of its own.
--   * payment_voucher_events: the append-only history (money audit).
--
-- THE INVARIANT. Never a double payment: a voucher pays at most one deposit
-- (its single deposit_id column) and a deposit is paid by at most one voucher
-- (uq_payment_vouchers_deposit). The deposit path locks the pupil's charges,
-- then the voucher rows, so both writers serialise on the same rows.
--
-- Schema only, additive; reversible by dropping the two tables.
-- RLS: ENABLE + FORCE + tenant_isolation on both tables.
BEGIN;

SET LOCAL lock_timeout = '5s';

CREATE TABLE payment_vouchers (
    id                   uuid PRIMARY KEY,
    tenant_id            uuid NOT NULL,
    student_id           uuid NOT NULL,
    uploaded_by          uuid NOT NULL,
    uploaded_by_name     text NOT NULL,
    claimed_amount_cents bigint NULL CHECK (claimed_amount_cents IS NULL OR claimed_amount_cents > 0),
    -- A civil day (ADR 0035), never an instant.
    claimed_paid_on      date NULL,
    file_key             text NOT NULL CHECK (file_key <> ''),
    file_content_type    text NOT NULL CHECK (file_content_type IN ('image/jpeg', 'image/png', 'application/pdf')),
    file_size_bytes      integer NOT NULL CHECK (file_size_bytes > 0 AND file_size_bytes <= 5242880),
    file_sha256          bytea NOT NULL CHECK (octet_length(file_sha256) = 32),
    status               text NOT NULL CHECK (status IN ('pending', 'applied', 'rejected')),
    deposit_id           uuid NULL REFERENCES deposits (id),
    decided_by           uuid NULL,
    decided_by_name      text NULL,
    decided_at           timestamptz NULL,
    rejection_reason     text NULL
        CHECK (rejection_reason IS NULL OR char_length(btrim(rejection_reason)) BETWEEN 3 AND 300),
    release_count        integer NOT NULL DEFAULT 0 CHECK (release_count >= 0),
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT payment_vouchers_shape_ck CHECK (
        (status = 'pending' AND deposit_id IS NULL AND rejection_reason IS NULL AND decided_at IS NULL)
        OR (status = 'applied' AND deposit_id IS NOT NULL AND rejection_reason IS NULL AND decided_at IS NOT NULL)
        OR (status = 'rejected' AND deposit_id IS NULL AND rejection_reason IS NOT NULL AND decided_at IS NOT NULL)
    )
);

-- THE invariant: a deposit is paid by at most one voucher.
CREATE UNIQUE INDEX uq_payment_vouchers_deposit ON payment_vouchers (deposit_id) WHERE deposit_id IS NOT NULL;
-- The same file sent twice for the same pupil (a retry, a double tap) is the
-- same voucher; once rejected, the family may send it again.
CREATE UNIQUE INDEX uq_payment_vouchers_file ON payment_vouchers (tenant_id, student_id, file_sha256)
    WHERE status <> 'rejected';
-- The office's queue, oldest first.
CREATE INDEX ix_payment_vouchers_queue ON payment_vouchers (tenant_id, created_at) WHERE status = 'pending';
-- A pupil's vouchers (family list, the deposit path's candidates).
CREATE INDEX ix_payment_vouchers_student ON payment_vouchers (tenant_id, student_id, status);

CREATE TABLE payment_voucher_events (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL,
    voucher_id  uuid NOT NULL REFERENCES payment_vouchers (id),
    kind        text NOT NULL CHECK (kind IN ('uploaded', 'applied', 'auto_linked', 'rejected',
                                              'released_by_void', 'moved_by_correction')),
    deposit_id  uuid NULL,
    actor_id    uuid NULL,
    actor_name  text NULL,
    reason      text NULL,
    -- clock_timestamp, not now(): several events of one transaction keep
    -- their order.
    at          timestamptz NOT NULL DEFAULT clock_timestamp()
);

CREATE INDEX ix_payment_voucher_events_voucher ON payment_voucher_events (tenant_id, voucher_id, at);

ALTER TABLE payment_vouchers ENABLE ROW LEVEL SECURITY;
ALTER TABLE payment_vouchers FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON payment_vouchers
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE payment_voucher_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE payment_voucher_events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON payment_voucher_events
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
