-- finance (M12 uniforms) — charges can now carry an external reference.
--
-- 0001 gave `charges` UNIQUE (tenant_id, student_id, cycle_id, concept_id) so
-- that bulk tuition generation is idempotent: re-running "charge concept X to
-- everyone enrolled in cycle Y" hits the key and does nothing the second time.
--
-- That rule reads as "a student may owe a given concept ONCE per cycle", and it
-- is right for tuition. It is wrong for anything a family buys more than once in
-- a school year. A uniform order needs one charge per delivery (an order split
-- into -A and -B is two charges), and a family that comes back in January for a
-- second sweater needs another. Under the old key every one of those failed with
-- a conflict.
--
-- The fix is NOT to drop the key. CreateChargesBulk leans on it through
-- `ON CONFLICT DO NOTHING` **with no target** (repository.go), so removing the
-- constraint would stop that clause deduplicating SILENTLY and the next run of
-- GenerateCharges would re-bill the entire school with no error at all.
--
-- Instead the key becomes PARTIAL and a second partial key is added, exactly the
-- shape 0002 used for payment idempotency:
--
--   external_ref = ''   -> the school's own charges (tuition, enrolment…).
--                          Still one per (student, cycle, concept). Unchanged.
--   external_ref <> ''  -> caused by a row in another module. Identified by the
--                          reference itself, so it may repeat the concept freely
--                          while a retried creation is still deduplicated.
--
-- The column is NOT NULL DEFAULT '' so every existing charge keeps its shape and
-- lands on the tuition side of the split. RLS is unchanged: the tenant_isolation
-- policy from 0001 covers the table and tenant_id leads both indexes.

BEGIN;

ALTER TABLE charges ADD COLUMN external_ref text NOT NULL DEFAULT '';

-- Replace the total key with its partial twin. Dropping and creating inside one
-- transaction means there is no window where keyless charges are unprotected.
ALTER TABLE charges DROP CONSTRAINT charges_tenant_id_student_id_cycle_id_concept_id_key;

CREATE UNIQUE INDEX uq_charges_concept_per_cycle
    ON charges (tenant_id, student_id, cycle_id, concept_id)
    WHERE external_ref = '';

CREATE UNIQUE INDEX uq_charges_external_ref
    ON charges (tenant_id, external_ref)
    WHERE external_ref <> '';

COMMIT;
