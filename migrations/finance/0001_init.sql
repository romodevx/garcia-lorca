-- finance (Phase 7) initial schema — per-student debt & payment ledger.
--
-- Money: every monetary column is an integer amount of **cents** (bigint).
-- Never float, never numeric-with-scale — cents are the unit of account here and
-- rounding is the caller's problem, not the database's.
--
-- RLS is tenant-scoped (same posture as iam/students/grades, ADR 0001): policy
-- filters on app.current_tenant, ENABLE + FORCE so it applies even to the table
-- owner. The application role is NOT the owner and NOT BYPASSRLS.
--
-- Bounded-context discipline: student_id / cycle_id are cross-service UUIDs
-- (students service, config.cycles) validated by RPC at the use-case layer —
-- deliberately NOT foreign keys. Only same-service references (concept_id,
-- charge_id) are real FKs, and they are ON DELETE RESTRICT: a ledger row is
-- never silently erased by deleting its parent.
--
-- Run by a privileged migration role. gen_random_uuid() is in core since PG13.

BEGIN;

-- Dynamic catalog of billable concepts (colegiatura, inscripción, uniforme…).
-- The school defines these; nothing here is hardcoded in the application.
CREATE TABLE charge_concepts (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            uuid NOT NULL,
    name                 text NOT NULL,
    default_amount_cents bigint NOT NULL CHECK (default_amount_cents >= 0),
    recurrence           text NOT NULL DEFAULT 'once'
        CHECK (recurrence IN ('once', 'monthly')),
    active               boolean NOT NULL DEFAULT true,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    -- concept names are unique per school.
    UNIQUE (tenant_id, name)
);

CREATE INDEX idx_charge_concepts_tenant ON charge_concepts (tenant_id);

-- Row Level Security (tenant isolation). Unset app.current_tenant ⇒ zero rows.
ALTER TABLE charge_concepts ENABLE ROW LEVEL SECURITY;
ALTER TABLE charge_concepts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON charge_concepts
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

-- A debt owed by one student for one concept in one cycle.
CREATE TABLE charges (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL,
    -- cross-service refs (NO FK — validated by RPC, not the DB):
    student_id      uuid NOT NULL,   -- students service (RPC-validated)
    cycle_id        uuid NOT NULL,   -- config.cycles    (RPC-validated)
    -- same-service ref (real FK):
    concept_id      uuid NOT NULL REFERENCES charge_concepts(id) ON DELETE RESTRICT,
    description     text NOT NULL,
    amount_cents    bigint NOT NULL CHECK (amount_cents >= 0),
    discount_cents  bigint NOT NULL DEFAULT 0 CHECK (discount_cents >= 0),
    due_date        date NOT NULL,
    status          text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'partially_paid', 'paid', 'cancelled')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    -- a discount can never exceed what is being charged.
    CHECK (discount_cents <= amount_cents),
    -- makes the bulk "charge concept X to everyone enrolled in cycle Y" idempotent:
    -- re-running it with ON CONFLICT DO NOTHING skips students already charged.
    UNIQUE (tenant_id, student_id, cycle_id, concept_id)
);

CREATE INDEX idx_charges_student ON charges (tenant_id, student_id);
CREATE INDEX idx_charges_cycle ON charges (tenant_id, cycle_id);
CREATE INDEX idx_charges_status ON charges (tenant_id, status);
CREATE INDEX idx_charges_due_date ON charges (tenant_id, due_date);

-- Row Level Security (tenant isolation). Unset app.current_tenant ⇒ zero rows.
ALTER TABLE charges ENABLE ROW LEVEL SECURITY;
ALTER TABLE charges FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON charges
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

-- Money received against a charge. Append-only in spirit: a charge may be paid
-- in several installments, and the charge's status is derived from the sum.
CREATE TABLE payments (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL,
    -- same-service ref (real FK):
    charge_id    uuid NOT NULL REFERENCES charges(id) ON DELETE RESTRICT,
    amount_cents bigint NOT NULL CHECK (amount_cents > 0),
    method       text NOT NULL CHECK (method IN ('cash', 'card', 'transfer')),
    reference    text NOT NULL DEFAULT '',   -- "con qué se pagó" / folio
    received_by  uuid NULL,                  -- reserved for cashier attribution (unused until cashier auth)
    paid_at      timestamptz NOT NULL DEFAULT now(),
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_payments_charge ON payments (tenant_id, charge_id);

-- Row Level Security (tenant isolation). Unset app.current_tenant ⇒ zero rows.
ALTER TABLE payments ENABLE ROW LEVEL SECURITY;
ALTER TABLE payments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON payments
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
