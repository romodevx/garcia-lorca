-- finance — every deposit gets the folio its receipt prints (task 12).
--
-- Finance had no folio: the screen showed a cut uuid. A deposit now carries
-- P-<year>-<six digits>, one run per school and year, like the uniforms folio
-- (U-…). The year is the capture's, as created_at records it (UTC, as finance
-- counts days — see "school day is UTC day").
--
-- A counter row per (tenant, year), bumped with INSERT … ON CONFLICT … RETURNING
-- in the deposit's own transaction: a SEQUENCE is per database and ignores RLS,
-- so two schools would share one run and read each other's volume in the gaps.
--
-- Existing deposits get their folio here, in capture order. Following
-- students/0006: the table is FORCE RLS and its owner has no
-- BYPASSRLS, so SET LOCAL binds the one school this deployment serves; the
-- backfill is partitioned by tenant all the same, for a role that bypasses RLS.
BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';

CREATE TABLE deposit_folio_counters (
    tenant_id uuid NOT NULL,
    year      integer NOT NULL CHECK (year BETWEEN 2000 AND 3000),
    next_seq  integer NOT NULL DEFAULT 1 CHECK (next_seq >= 1),
    PRIMARY KEY (tenant_id, year)
);

ALTER TABLE deposit_folio_counters ENABLE ROW LEVEL SECURITY;
ALTER TABLE deposit_folio_counters FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON deposit_folio_counters
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid);

ALTER TABLE deposits ADD COLUMN folio text;

WITH numbered AS (
    SELECT id, tenant_id,
           EXTRACT(YEAR FROM created_at AT TIME ZONE 'UTC')::int AS year,
           row_number() OVER (
               PARTITION BY tenant_id, EXTRACT(YEAR FROM created_at AT TIME ZONE 'UTC')
               ORDER BY created_at, id) AS seq
      FROM deposits
)
UPDATE deposits d
   SET folio = 'P-' || lpad(n.year::text, 4, '0') || '-' || lpad(n.seq::text, 6, '0')
  FROM numbered n
 WHERE d.id = n.id;

INSERT INTO deposit_folio_counters (tenant_id, year, next_seq)
SELECT tenant_id, EXTRACT(YEAR FROM created_at AT TIME ZONE 'UTC')::int, count(*) + 1
  FROM deposits
 GROUP BY tenant_id, EXTRACT(YEAR FROM created_at AT TIME ZONE 'UTC');

ALTER TABLE deposits ALTER COLUMN folio SET NOT NULL;
ALTER TABLE deposits ADD CONSTRAINT deposits_folio_shape CHECK (folio ~ '^P-[0-9]{4}-[0-9]{6}$');
ALTER TABLE deposits ADD CONSTRAINT deposits_folio_unique UNIQUE (tenant_id, folio);

COMMIT;
