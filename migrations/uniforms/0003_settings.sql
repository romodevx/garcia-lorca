-- uniforms (M12) — per-school settings.
--
-- One row per tenant, created lazily on first read. Everything here is a number
-- the school is allowed to argue about, which is exactly why none of it is a
-- constant in the code.
--
-- The school calendar is deliberately NOT here. Which days the school does not
-- work is a fact about the school, not about the uniform shop: it lives in the
-- config service so attendance can read the same one, rather than each module
-- keeping a calendar that drifts from the others.

BEGIN;

CREATE TABLE uniform_settings (
    tenant_id                   uuid PRIMARY KEY,
    -- How long a parent has to come and pay once a delivery is ready, counted
    -- in working days.
    pickup_window_business_days integer NOT NULL DEFAULT 10
        CHECK (pickup_window_business_days BETWEEN 1 AND 90),
    -- Lets a school stop the automatic release entirely — during the
    -- back-to-school weeks, say, when nobody wants holds evaporating.
    auto_cancel_enabled         boolean NOT NULL DEFAULT true,
    -- How many extensions staff may grant on one delivery before the answer has
    -- to be "no". Without a ceiling a hold never really expires.
    max_extensions              integer NOT NULL DEFAULT 2 CHECK (max_extensions BETWEEN 0 AND 5),
    -- The two ceilings that stop one family from reserving a whole size range
    -- for ten working days at no cost.
    max_open_orders_per_student integer NOT NULL DEFAULT 5 CHECK (max_open_orders_per_student BETWEEN 1 AND 50),
    max_lines_per_order         integer NOT NULL DEFAULT 20 CHECK (max_lines_per_order BETWEEN 1 AND 100),
    max_units_per_line          integer NOT NULL DEFAULT 5 CHECK (max_units_per_line BETWEEN 1 AND 20),
    -- How many times one line may be reserved and let lapse before it stops
    -- being reservable.
    max_line_expiries           integer NOT NULL DEFAULT 2 CHECK (max_line_expiries BETWEEN 0 AND 10),
    updated_at                  timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE uniform_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE uniform_settings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON uniform_settings
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
