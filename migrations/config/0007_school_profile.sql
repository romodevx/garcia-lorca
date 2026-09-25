-- config — the school's details, as a receipt prints them (task 12).
--
-- One row per school: name (required), legal name, RFC, CCT, address, phone and
-- a legend for the foot of the receipt. The logo stays the static one. Shapes
-- (RFC, CCT, ten-digit phone) are checked by the domain; the table keeps the
-- lengths and the one-row rule.
BEGIN;

CREATE TABLE school_profile (
    tenant_id   uuid PRIMARY KEY,
    name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    legal_name  text NOT NULL DEFAULT '' CHECK (length(legal_name) <= 200),
    rfc         text NOT NULL DEFAULT '' CHECK (length(rfc) <= 13),
    cct         text NOT NULL DEFAULT '' CHECK (length(cct) <= 10),
    address     text NOT NULL DEFAULT '' CHECK (length(address) <= 300),
    phone       text NOT NULL DEFAULT '' CHECK (phone = '' OR phone ~ '^[0-9]{10}$'),
    legend      text NOT NULL DEFAULT '' CHECK (length(legend) <= 500),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE school_profile ENABLE ROW LEVEL SECURITY;
ALTER TABLE school_profile FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON school_profile
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
