-- finance (M15) — the tuition table: what each level costs, for a given cycle.
--
-- Keyed by (cycle, level) and not by level alone, because a school raises tuition
-- every year. If the price hung off the level, raising it would rewrite the past:
-- last year's charges would stop reconciling against their own table, and nobody
-- could audit what was charged or why.
--
-- The cycle and the level live in the config service. There is no FK — the
-- established cross-service rule here is that such references are validated by
-- RPC, not by the database, because the two contexts own their own storage.
--
-- Everything is integer cents. Money is never a float in this codebase.

BEGIN;

CREATE TABLE tuition_plans (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          uuid NOT NULL,
    -- cross-service refs (NO FK — validated by RPC, not the DB):
    cycle_id           uuid NOT NULL,   -- config.cycles
    level_id           uuid NOT NULL,   -- config.levels
    enrollment_cents   bigint   NOT NULL CHECK (enrollment_cents >= 0),
    monthly_cents      bigint   NOT NULL CHECK (monthly_cents    >= 0),
    -- How many monthly payments the year has, and which calendar month the first
    -- one falls in. Ten from September is the common Mexican shape (Sept–June),
    -- with August covered by the inscription; a school that bills eleven or
    -- twelve says so here, per level.
    months             smallint NOT NULL CHECK (months BETWEEN 1 AND 12),
    first_billed_month smallint NOT NULL CHECK (first_billed_month BETWEEN 1 AND 12),
    -- Capped at 28 on purpose: a limit on the 31st does not exist in February and
    -- would silently move. It is refused at the boundary instead of adjusted.
    due_day            smallint NOT NULL CHECK (due_day BETWEEN 1 AND 28),
    -- The fixed surcharge added once per completed overdue month. Zero disables it.
    late_fee_cents     bigint   NOT NULL DEFAULT 0 CHECK (late_fee_cents >= 0),
    active             boolean  NOT NULL DEFAULT true,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    -- one table per level per cycle.
    UNIQUE (tenant_id, cycle_id, level_id)
);

CREATE INDEX idx_tuition_plans_cycle ON tuition_plans (tenant_id, cycle_id);

-- Row Level Security (tenant isolation). Unset app.current_tenant ⇒ zero rows.
ALTER TABLE tuition_plans ENABLE ROW LEVEL SECURITY;
ALTER TABLE tuition_plans FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tuition_plans
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
