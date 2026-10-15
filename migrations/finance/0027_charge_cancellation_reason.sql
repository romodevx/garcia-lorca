-- Record the reason a charge was cancelled, independently from any discount.
BEGIN;

ALTER TABLE charges ADD COLUMN cancellation_reason text NOT NULL DEFAULT '';

COMMIT;
