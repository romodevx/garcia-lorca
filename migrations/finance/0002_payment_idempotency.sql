-- finance (Phase 8b) — payment idempotency.
--
-- A retried RegisterPayment used to post the money twice: a network retry after
-- a successful commit inserted a second payment, and a partial payment
-- duplicated silently. The client may now send an idempotency key with a
-- payment; a repeat with the same key returns the original payment instead of
-- collecting again.
--
-- The column is NOT NULL DEFAULT '' so every existing payment keeps its shape
-- and a caller that sends no key keeps the old behaviour (one call, one
-- payment). That is exactly why the unique index is PARTIAL: a plain
-- UNIQUE (tenant_id, idempotency_key) would let only ONE keyless payment exist
-- per tenant, so the blank key is excluded from the constraint and only real
-- keys are deduplicated.
--
-- The index is the backstop, not the mechanism: the application dedupes inside
-- the payment transaction, under the charge's FOR UPDATE row lock. The
-- constraint is what stops two concurrent requests on DIFFERENT charges (which
-- do not share that lock) from both inserting the same key.
--
-- RLS is unchanged: the tenant_isolation policy from 0001 already covers the
-- table, and tenant_id leads the index so the dedup lookup stays tenant-scoped.

BEGIN;

ALTER TABLE payments ADD COLUMN idempotency_key text NOT NULL DEFAULT '';

CREATE UNIQUE INDEX uq_payments_idempotency ON payments (tenant_id, idempotency_key)
    WHERE idempotency_key <> '';

COMMIT;
