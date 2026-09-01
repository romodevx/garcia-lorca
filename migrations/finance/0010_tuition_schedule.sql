-- finance (M16) — la colegiatura partida por período.
--
-- A school does not always divide the year into equal parts. "Once meses, pero
-- diciembre a la mitad y la otra mitad en marzo" is a real arrangement —
-- families are short in December — and a plan that can only say "N equal
-- payments" cannot express it.
--
-- Stored as JSONB and not a child table, deliberately. The schedule is a small
-- ordered list owned entirely by its plan: it is always read whole, always
-- written whole, and nobody will ever ask "which plans collect something in
-- March". Keeping it in the row makes replacing a tuition table one atomic
-- write instead of a delete-and-insert dance, and there is no integrity rule
-- here that a foreign key would enforce and the domain does not.
--
-- Empty is the ordinary state: a plan without a schedule bills the uniform year
-- it always billed.
BEGIN;

ALTER TABLE tuition_plans ADD COLUMN schedule jsonb NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE tuition_plans ADD CONSTRAINT tuition_plans_schedule_is_a_list_ck
    CHECK (jsonb_typeof(schedule) = 'array');

COMMIT;
