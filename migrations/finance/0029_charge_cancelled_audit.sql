-- C2-005: a cancellation records when and who, beside its reason (0027).
-- Rows cancelled before this migration keep NULL: nobody wrote it down then,
-- and inventing a time or an author would be worse than admitting it.
BEGIN;

ALTER TABLE charges
    ADD COLUMN cancelled_at timestamptz,
    ADD COLUMN cancelled_by uuid;

COMMIT;
