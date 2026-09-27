-- config — a trimester is a SET OF MONTHS per cycle and level (T7).
--
-- Terms already carry starts_on/ends_on, but a school thinks of a trimester as
-- "September, October, November", and the boleta groups monthly grades by
-- month. term_months is that set, configured per cycle+level (not hard-coded):
--
--   month       the first day of the calendar month (CHECK day = 1).
--   PRIMARY KEY (tenant_id, term_id, month)            a month once per term.
--   UNIQUE      (tenant_id, cycle_id, level_id, month) a month belongs to at
--               most one term of the same cycle and level.
--
-- cycle_id/level_id are denormalised from the term so the UNIQUE can say that;
-- a composite FK to terms (id, tenant_id, cycle_id, level_id) keeps them from
-- ever disagreeing with the term they belong to. Deleting a term (or its cycle,
-- which cascades to terms) removes its months.
--
-- RLS: ENABLE + FORCE, same tenant_isolation policy as every config table.
-- Additive only.

BEGIN;

SET LOCAL lock_timeout = '5s';

-- The composite FK target. id alone is already unique, so this adds no rule;
-- it only lets term_months reference the (cycle, level) of its term.
ALTER TABLE terms
    ADD CONSTRAINT terms_id_scope_key UNIQUE (id, tenant_id, cycle_id, level_id);

CREATE TABLE term_months (
    tenant_id  uuid NOT NULL,
    term_id    uuid NOT NULL,
    cycle_id   uuid NOT NULL,
    level_id   uuid NOT NULL,
    month      date NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT term_months_pkey PRIMARY KEY (tenant_id, term_id, month),
    CONSTRAINT term_months_first_day_ck CHECK (EXTRACT(DAY FROM month) = 1),
    CONSTRAINT term_months_one_term_per_month
        UNIQUE (tenant_id, cycle_id, level_id, month),
    CONSTRAINT term_months_term_fk
        FOREIGN KEY (term_id, tenant_id, cycle_id, level_id)
        REFERENCES terms (id, tenant_id, cycle_id, level_id) ON DELETE CASCADE
);

-- The batched read (months of a page of terms) goes by term_id.
CREATE INDEX idx_term_months_term ON term_months (term_id);

ALTER TABLE term_months ENABLE ROW LEVEL SECURITY;
ALTER TABLE term_months FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON term_months
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

COMMIT;
