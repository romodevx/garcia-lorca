-- finance — a sale to a pupil with no cycle yet (task 10).
--
-- The office sells uniforms to pupils registered but not yet placed in a group:
-- they have no cycle, and every charge named one. Such a sale is charged to the
-- pupil with no cycle, and answers to its external ref instead (the uniforms
-- delivery that caused it). The school's own charges — tuition, inscription,
-- hand-typed — still name their cycle: a charge with neither would be a debt
-- nothing could explain, and the CHECK refuses it.
--
-- A NULL cycle matches no "cycle_id = ?" read, so billing runs, cycle ledgers
-- and overdue-by-cycle never see these rows; the pupil's account (by student)
-- does. The per-cycle partial unique index covers only ref-less rows, which
-- always have a cycle, so it is unaffected.

BEGIN;

ALTER TABLE charges ALTER COLUMN cycle_id DROP NOT NULL;

ALTER TABLE charges ADD CONSTRAINT charges_cycle_or_sale_ref
    CHECK (cycle_id IS NOT NULL OR external_ref <> '');

COMMIT;
