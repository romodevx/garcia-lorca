-- uniforms (M12) — orders, deliveries and the lines between them.
--
-- Three tables because an order and a delivery are not the same thing.
--
--   uniform_orders       what the family asked for. A container. It has NO
--                        status column: its label is derived from its lines and
--                        deliveries, and a stored copy would be a second truth
--                        to keep in step.
--   uniform_order_lines  one garment in one size. This is what holds stock.
--   uniform_fulfillments a delivery. THIS is where the state machine lives: a
--                        delivery is what gets picked, charged, expires and is
--                        handed over. An order of three garments where only two
--                        are in stock becomes one delivery now and another when
--                        the third arrives, each with its own charge and its own
--                        deadline.
--
-- student_id is a cross-service UUID validated by RPC — deliberately NO foreign
-- key. student_name, student_level and student_grade_group are SNAPSHOTS, taken
-- when the order is placed: they let the panel filter by level and search by
-- name without fanning out to the students service on every page load. The cost
-- is that a student transferred to another level keeps the old snapshot on old
-- orders, which is the correct reading of history anyway.

BEGIN;

-- --------------------------------------------------------------------------
-- Folio allocation
-- --------------------------------------------------------------------------
--
-- A Postgres SEQUENCE is per database, not per tenant, and does not obey RLS —
-- two schools would share one run of numbers and each could infer the other's
-- volume. A counter row per (tenant, year), bumped with UPDATE … RETURNING under
-- its own row lock, is per tenant, gapless enough, and tenant-isolated like
-- everything else.

CREATE TABLE uniform_folio_counters (
    tenant_id uuid NOT NULL,
    year      integer NOT NULL CHECK (year BETWEEN 2000 AND 3000),
    next_seq  integer NOT NULL DEFAULT 1 CHECK (next_seq >= 1),
    PRIMARY KEY (tenant_id, year)
);

-- --------------------------------------------------------------------------
-- Orders
-- --------------------------------------------------------------------------

CREATE TABLE uniform_orders (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL,
    folio               text NOT NULL CHECK (folio ~ '^U-[0-9]{4}-[0-9]{6}$'),
    -- NULL is a counter sale to someone who is not a student (an uncle, an
    -- alumnus). It still moves stock and still takes money; it just cannot land
    -- on anybody's account statement.
    student_id          uuid,
    student_name        text NOT NULL DEFAULT '' CHECK (length(student_name) <= 200),
    student_level       text CHECK (student_level IN ('kinder', 'primaria', 'secundaria', 'preparatoria')),
    student_grade_group text NOT NULL DEFAULT '' CHECK (length(student_grade_group) <= 40),
    cycle_id            uuid,
    created_by_user_id  uuid,
    source              text NOT NULL CHECK (source IN ('family', 'counter', 'staff')),
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    closed_at           timestamptz,
    UNIQUE (tenant_id, folio),
    -- A sale to nobody cannot be charged to anybody, so it may not claim a
    -- cycle either; and an order for a student always names the cycle its
    -- charge belongs to.
    CONSTRAINT uniform_orders_student_cycle CHECK (
        (student_id IS NULL AND cycle_id IS NULL) OR (student_id IS NOT NULL AND cycle_id IS NOT NULL)
    )
);

-- The panel's three access patterns: newest first (stable by id for paging),
-- by level for the filter tabs, and by student for a family's history.
CREATE INDEX idx_uniform_orders_recent ON uniform_orders (tenant_id, created_at DESC, id);
CREATE INDEX idx_uniform_orders_level ON uniform_orders (tenant_id, student_level);
CREATE INDEX idx_uniform_orders_student ON uniform_orders (tenant_id, student_id);

-- --------------------------------------------------------------------------
-- Deliveries (the state machine)
-- --------------------------------------------------------------------------

CREATE TABLE uniform_fulfillments (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL,
    order_id          uuid NOT NULL REFERENCES uniform_orders(id) ON DELETE RESTRICT,
    -- 1, 2, 3 … rendered as the -A, -B, -C suffix of the order's folio. A number
    -- rather than a letter because a letter would cap an order at 26 deliveries.
    seq               integer NOT NULL CHECK (seq >= 1),
    status            text NOT NULL CHECK (status IN (
                          'picking',            -- staff pulled it; the charge is being raised
                          'ready_for_pickup',   -- the clock runs
                          'paid',               -- money in; stock deducted
                          'dispatched',         -- handed over. terminal
                          'expired',            -- nobody came. stock released. terminal
                          'cancelled',          -- called off by staff. terminal
                          'sold_at_counter')),  -- sold and handed over in one act. terminal
    amount_cents      bigint NOT NULL DEFAULT 0 CHECK (amount_cents >= 0),
    finance_charge_id uuid,
    ready_at          timestamptz,
    pickup_deadline   date,
    -- true when the deadline was computed without the school calendar because
    -- config was unreachable. The sweep recomputes those, and may only ever
    -- EXTEND them: shortening a deadline already told to a parent is indefensible.
    deadline_estimated boolean NOT NULL DEFAULT false,
    extended_days     integer NOT NULL DEFAULT 0 CHECK (extended_days >= 0),
    extension_seq     integer NOT NULL DEFAULT 0 CHECK (extension_seq >= 0),
    -- Set when something happened that no rule can settle on its own: a payment
    -- arriving for a delivery that already expired, for instance. The panel has
    -- a filter for it. Silence is the one outcome not allowed.
    requires_attention boolean NOT NULL DEFAULT false,
    attention_reason  text NOT NULL DEFAULT '' CHECK (length(attention_reason) <= 200),
    paid_at           timestamptz,
    dispatched_at     timestamptz,
    cancelled_at      timestamptz,
    cancel_reason     text NOT NULL DEFAULT '' CHECK (length(cancel_reason) <= 200),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, order_id, seq),
    -- One charge answers to one delivery, which is what makes the finance
    -- external_ref a safe idempotency key for raising it (ADR 0008).
    UNIQUE (tenant_id, finance_charge_id)
);

CREATE INDEX idx_uniform_fulfillments_status ON uniform_fulfillments (tenant_id, status);
CREATE INDEX idx_uniform_fulfillments_order ON uniform_fulfillments (tenant_id, order_id);
-- The expiry sweep reads exactly this slice, so it stays a range scan however
-- large the history grows.
CREATE INDEX idx_uniform_fulfillments_due ON uniform_fulfillments (tenant_id, pickup_deadline)
    WHERE status = 'ready_for_pickup';
-- So does the "needs a human" filter.
CREATE INDEX idx_uniform_fulfillments_attention ON uniform_fulfillments (tenant_id)
    WHERE requires_attention;

-- --------------------------------------------------------------------------
-- Lines
-- --------------------------------------------------------------------------

CREATE TABLE uniform_order_lines (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid NOT NULL,
    order_id         uuid NOT NULL REFERENCES uniform_orders(id) ON DELETE RESTRICT,
    item_id          uuid NOT NULL REFERENCES uniform_items(id) ON DELETE RESTRICT,
    -- Snapshots, like the student's: the catalogue may be renamed or repriced
    -- afterwards and an issued order must not change under the family.
    item_name        text NOT NULL CHECK (length(item_name) BETWEEN 1 AND 120),
    size_label       text NOT NULL CHECK (length(size_label) BETWEEN 1 AND 12),
    qty              integer NOT NULL CHECK (qty BETWEEN 1 AND 20),
    unit_price_cents bigint NOT NULL CHECK (unit_price_cents >= 0),
    status           text NOT NULL CHECK (status IN (
                         'awaiting_stock',  -- the school owes it. holds nothing
                         'reserved',        -- holds a unit of uniform_stock.reserved
                         'sold',            -- terminal. the unit left
                         'released',        -- terminal. the hold lapsed, unit returned
                         'cancelled')),     -- terminal. called off before it ever held
    -- Cleared whenever the line stops belonging to a delivery, so a released
    -- line never points at the delivery that let it go.
    fulfillment_id   uuid REFERENCES uniform_fulfillments(id) ON DELETE RESTRICT,
    -- How many times this line has been reserved and then let lapse. Without a
    -- ceiling a family could hold the last unit of a size indefinitely, for
    -- free, by never coming to pay.
    expiry_count     integer NOT NULL DEFAULT 0 CHECK (expiry_count >= 0),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    -- Only a line that belongs to a delivery may be sold; a line that belongs to
    -- none may not.
    CONSTRAINT uniform_order_lines_sold_needs_fulfillment CHECK (
        status <> 'sold' OR fulfillment_id IS NOT NULL
    )
);

CREATE INDEX idx_uniform_order_lines_order ON uniform_order_lines (tenant_id, order_id);
CREATE INDEX idx_uniform_order_lines_fulfillment ON uniform_order_lines (tenant_id, fulfillment_id);
-- FIFO allocation when stock arrives reads the queue of unmet demand for one
-- garment and size, oldest first.
CREATE INDEX idx_uniform_order_lines_waiting ON uniform_order_lines (tenant_id, item_id, size_label, created_at, id)
    WHERE status = 'awaiting_stock';

-- --------------------------------------------------------------------------
-- Row Level Security
-- --------------------------------------------------------------------------

ALTER TABLE uniform_folio_counters ENABLE ROW LEVEL SECURITY;
ALTER TABLE uniform_folio_counters FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON uniform_folio_counters
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE uniform_orders ENABLE ROW LEVEL SECURITY;
ALTER TABLE uniform_orders FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON uniform_orders
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE uniform_fulfillments ENABLE ROW LEVEL SECURITY;
ALTER TABLE uniform_fulfillments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON uniform_fulfillments
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE uniform_order_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE uniform_order_lines FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON uniform_order_lines
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
