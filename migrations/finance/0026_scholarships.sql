-- finance (T18) — scholarships (becas), computed by the ledger.
--
-- THE PROBLEM. The school grants some families a standing reduction — a share
-- off, a fixed amount off, or a fixed price — on the inscription, the monthly
-- tuition, specific concepts or activities. It has to reach every place a debt
-- is priced (the counter, the parents' app, the statement, the receipts) by
-- ONE computation, and it holds only while the charge is paid by its due date.
--
-- THE SHAPE (ADR 0022).
--
--   * scholarships: name (one per school, whatever its case), kind
--     (percent | fixed_off | fixed_price), value (basis points for a percent,
--     cents otherwise), targets JSONB ["enrollment" | "tuition" |
--     "concept:<id>" | "activity:<id>"], active. Never deleted: a charge
--     settled under one keeps pointing at it.
--   * scholarship_students: a pupil holding a scholarship from starts_on until
--     ends_on (the first day no longer held; NULL = held). At most one open
--     row per pupil and scholarship.
--   * charges.scholarship_cents / scholarship_id: the part a scholarship took
--     off, WRITTEN when the charge is settled under it (by money, or by the
--     scholarship itself by its due date). Until then it is worked out on read,
--     which is why assigning one reaches every pending charge and removing it
--     gives them back their price. Kept apart from discount_cents because a
--     month carries both: the level's discount and, on what is left, the
--     scholarship.
--
-- Additive; reversible by dropping the two columns, their checks and the two
-- tables. RLS: ENABLE + FORCE + tenant_isolation on both new tables.
BEGIN;

SET LOCAL lock_timeout = '5s';

CREATE TABLE scholarships (
    id         uuid PRIMARY KEY,
    tenant_id  uuid NOT NULL,
    name       text NOT NULL CHECK (btrim(name) <> '' AND char_length(name) <= 120),
    kind       text NOT NULL CHECK (kind IN ('percent', 'fixed_off', 'fixed_price')),
    value      bigint NOT NULL CHECK (value >= 0 AND value <= 1000000000000000),
    targets    jsonb NOT NULL CHECK (jsonb_typeof(targets) = 'array' AND jsonb_array_length(targets) BETWEEN 1 AND 50),
    active     boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT scholarships_percent_ck CHECK (kind <> 'percent' OR value BETWEEN 1 AND 10000),
    CONSTRAINT scholarships_fixed_off_ck CHECK (kind <> 'fixed_off' OR value >= 1)
);

-- One name per school, whatever its case (accents: the use case).
CREATE UNIQUE INDEX uq_scholarships_name ON scholarships (tenant_id, lower(btrim(name)));

CREATE TABLE scholarship_students (
    id             uuid PRIMARY KEY,
    tenant_id      uuid NOT NULL,
    scholarship_id uuid NOT NULL REFERENCES scholarships (id) ON DELETE RESTRICT,
    student_id     uuid NOT NULL,
    starts_on      date NOT NULL,
    ends_on        date NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT scholarship_students_range_ck CHECK (ends_on IS NULL OR ends_on >= starts_on)
);

-- At most one open holding per pupil and scholarship.
CREATE UNIQUE INDEX uq_scholarship_students_open
    ON scholarship_students (tenant_id, scholarship_id, student_id)
    WHERE ends_on IS NULL;
-- Pricing reads a pupil's open holdings; the screen reads a scholarship's.
CREATE INDEX idx_scholarship_students_student ON scholarship_students (tenant_id, student_id) WHERE ends_on IS NULL;
CREATE INDEX idx_scholarship_students_scholarship ON scholarship_students (tenant_id, scholarship_id);

ALTER TABLE charges
    ADD COLUMN scholarship_cents bigint NOT NULL DEFAULT 0 CHECK (scholarship_cents >= 0),
    ADD COLUMN scholarship_id uuid NULL REFERENCES scholarships (id) ON DELETE RESTRICT;
ALTER TABLE charges
    ADD CONSTRAINT charges_adjustments_within_amount_ck
        CHECK (discount_cents + scholarship_cents <= amount_cents),
    ADD CONSTRAINT charges_scholarship_named_ck
        CHECK ((scholarship_cents = 0) = (scholarship_id IS NULL));

ALTER TABLE scholarships ENABLE ROW LEVEL SECURITY;
ALTER TABLE scholarships FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON scholarships
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE scholarship_students ENABLE ROW LEVEL SECURITY;
ALTER TABLE scholarship_students FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON scholarship_students
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
