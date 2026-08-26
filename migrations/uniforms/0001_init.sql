-- uniforms (M12) — catalogue and inventory.
--
-- RLS is tenant-scoped (ADR 0001): every policy filters on app.current_tenant,
-- ENABLE + FORCE so it applies even to the table owner. The application role is
-- NOT the owner and NOT BYPASSRLS.
--
-- Sizes are text, not an enum, because they are not homogeneous: trousers run
-- 4/6/8, a jumper runs CH/M/G. size_order is what puts them in human order.
--
-- Stock is kept as two counters rather than one. on_hand is what is physically
-- in the storeroom; reserved is what is already committed to a live order line.
-- What a parent may take is the difference, and it is DERIVED — never stored —
-- so the two can never disagree.
--
-- Run by a privileged migration role. gen_random_uuid() is in core since PG13.

BEGIN;

-- --------------------------------------------------------------------------
-- Catalogue
-- --------------------------------------------------------------------------

CREATE TABLE uniform_items (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL,
    code        text NOT NULL CHECK (length(code) BETWEEN 1 AND 40),
    name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 500),
    -- NULL means the garment applies to every level. A school whose jumper is
    -- the same from kinder to prepa never has to say so four times.
    level       text CHECK (level IN ('kinder', 'primaria', 'secundaria', 'preparatoria')),
    price_cents bigint NOT NULL CHECK (price_cents >= 0),
    active      boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, code)
);

CREATE INDEX idx_uniform_items_active ON uniform_items (tenant_id, active);

-- --------------------------------------------------------------------------
-- Inventory
-- --------------------------------------------------------------------------

CREATE TABLE uniform_stock (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL,
    item_id    uuid NOT NULL REFERENCES uniform_items(id) ON DELETE RESTRICT,
    size_label text NOT NULL CHECK (length(size_label) BETWEEN 1 AND 12),
    size_order integer NOT NULL DEFAULT 0,
    on_hand    integer NOT NULL DEFAULT 0,
    reserved   integer NOT NULL DEFAULT 0,
    min_stock  integer NOT NULL DEFAULT 0 CHECK (min_stock >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, item_id, size_label),
    -- The invariant the whole module rests on: you cannot commit stock you do
    -- not have. Selling a reserved unit must therefore decrement BOTH counters
    -- in one statement — dropping on_hand alone trips this check, and on the
    -- last unit in stock it would abort the very transaction that consumes the
    -- payment event.
    CONSTRAINT uniform_stock_counters CHECK (on_hand >= 0 AND reserved >= 0 AND reserved <= on_hand)
);

CREATE INDEX idx_uniform_stock_item ON uniform_stock (tenant_id, item_id);
-- Powers the "below minimum" tab without scanning the catalogue.
CREATE INDEX idx_uniform_stock_low ON uniform_stock (tenant_id) WHERE on_hand - reserved <= min_stock;

-- --------------------------------------------------------------------------
-- Stock ledger — append only
-- --------------------------------------------------------------------------
--
-- Every movement of every unit, and the only way to audit a physical count
-- against the system months later. Its value depends entirely on nobody being
-- able to rewrite history, so append-only here is a privilege, not a comment:
-- RLS grants SELECT and INSERT and nothing else, and UPDATE/DELETE are revoked
-- from the application role outright so an attempt fails loudly rather than
-- quietly touching zero rows.

CREATE TABLE uniform_stock_moves (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL,
    stock_id      uuid NOT NULL REFERENCES uniform_stock(id) ON DELETE RESTRICT,
    order_line_id uuid,   -- NULL for receipts and adjustments
    kind          text NOT NULL CHECK (kind IN ('receive', 'reserve', 'release', 'sell', 'adjust')),
    -- Signed: a receipt is positive, a sale negative. The sign is part of the
    -- record, so the ledger sums to the current position.
    qty           integer NOT NULL CHECK (qty <> 0),
    actor_user_id uuid,
    note          text NOT NULL DEFAULT '' CHECK (length(note) <= 500),
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_uniform_stock_moves_stock ON uniform_stock_moves (tenant_id, stock_id, created_at);

-- --------------------------------------------------------------------------
-- Row Level Security
-- --------------------------------------------------------------------------

ALTER TABLE uniform_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE uniform_items FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON uniform_items
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE uniform_stock ENABLE ROW LEVEL SECURITY;
ALTER TABLE uniform_stock FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON uniform_stock
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

-- Per-operation policies, deliberately: there is no policy for UPDATE or DELETE,
-- so RLS refuses them even if a privilege were granted by mistake.
ALTER TABLE uniform_stock_moves ENABLE ROW LEVEL SECURITY;
ALTER TABLE uniform_stock_moves FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON uniform_stock_moves FOR SELECT
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);
CREATE POLICY tenant_append ON uniform_stock_moves FOR INSERT
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

-- Belt to that braces: a privilege error is loud, an empty RLS result is not.
-- The default privileges granted at CREATE TABLE time are narrowed here. Guarded
-- on the role existing, because a local database or a CI run may apply the
-- migrations before (or without) the application role from setup_app_role.sql.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app') THEN
        REVOKE UPDATE, DELETE ON uniform_stock_moves FROM app;
    END IF;
END
$$;

COMMIT;
