-- finance (M18) — la inscripción deja de ser un número.
--
-- The school's price list does not charge one amount to join. It charges three
-- lines — inscripción, gastos adm y seguro, cuota de la asociación de padres —
-- and the first of them doubles between January and April. The ladder is the
-- whole incentive: joining late costs more, and it goes on costing more until
-- the money arrives.
--
-- JSONB and not a child table, for the same reasons as `fees` (0013) and
-- `schedule` (0010): the list is small, owned entirely by its plan, always read
-- whole and always written whole. Nobody will ask "which levels charge a
-- parents' association fee".
--
-- Dates inside the JSON are stored as 'YYYY-MM-DD' strings, not as timestamps.
-- A tier boundary is a calendar day the school wrote down, and an instant with
-- an offset would invite exactly the confusion that is already costing this
-- service six hours (see the AmountOn comment in domain/enrollment_concept.go).
--
-- The default '[]' is what keeps this migration inert: every plan already
-- configured goes on charging its enrollment_cents, exactly as it does today.
-- The two are mutually exclusive by domain invariant (ADR 0011) — a plan is
-- either one amount or a list of concepts, never both — so nothing has to
-- choose between them at read time.
BEGIN;

ALTER TABLE tuition_plans
    ADD COLUMN enrollment_concepts jsonb NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE tuition_plans
    ADD CONSTRAINT tuition_plans_enrollment_concepts_is_a_list_ck
        CHECK (jsonb_typeof(enrollment_concepts) = 'array');

-- The invariant the domain enforces, enforced here too. Not belt and braces:
-- the run, the enrolment consumer, the grpc handler and the gateway client all
-- still read enrollment_cents, so a row carrying both would price the
-- inscription at one number in the editor and another on the statement, with
-- nothing saying they disagree.
ALTER TABLE tuition_plans
    ADD CONSTRAINT tuition_plans_one_inscription_shape_ck
        CHECK (jsonb_array_length(enrollment_concepts) = 0 OR enrollment_cents = 0);

COMMIT;
