-- finance (M15) — charge concepts learn what they ARE, not just what they are called.
--
-- Concepts were told apart by name, and the name is editable by the school. The
-- monthly billing run has to find "the tuition concept" every time it runs, and
-- a school that renames "Colegiatura" to "Mensualidad" must not thereby stop
-- being billed.
--
-- kind is the stable identity:
--   custom     — anything the school invented; there may be many
--   tuition    — the monthly fee
--   enrollment — the once-per-cycle inscription
--   late_fee   — the fixed surcharge added per overdue month
--
-- The partial unique index is the invariant that matters: a school may have any
-- number of custom concepts and AT MOST ONE of each system kind. Two tuition
-- concepts would make "the" tuition concept ambiguous, and the run would have to
-- guess.

BEGIN;

ALTER TABLE charge_concepts
    ADD COLUMN kind text NOT NULL DEFAULT 'custom'
        CHECK (kind IN ('custom', 'tuition', 'enrollment', 'late_fee'));

CREATE UNIQUE INDEX uq_charge_concepts_system_kind
    ON charge_concepts (tenant_id, kind)
    WHERE kind <> 'custom';

COMMIT;
