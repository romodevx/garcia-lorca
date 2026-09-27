-- grades — a report card freezes its term's order in the cycle (T8, boletas en
-- la app de padres).
--
-- The family lists its child's report cards newest period first: by cycle,
-- then by the term's ordinal (3.er trimestre above 1.er). Issue date is not
-- that order — a reopened and re-closed 1.er trimestre is issued AFTER the
-- 2.º — and the term's name is free text. The ordinal is copied from config at
-- close time, like the names, so a later renumbering never reorders an issued
-- card.
--
-- Cards issued before this column read 0 ("unknown") and sort below the
-- numbered terms of their cycle until the period is closed again. The terms
-- live in config (another database), so there is nothing to backfill from
-- here. Additive only.
BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE report_cards
    ADD COLUMN term_ordinal int NOT NULL DEFAULT 0 CHECK (term_ordinal >= 0);

COMMIT;
