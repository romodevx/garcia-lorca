-- grades — the months a period was closed with (T7 fix).
--
-- WHAT WAS WRONG. A closed term locked the months config maps into it NOW.
-- Take a month out of the term in config (config is its own service; the
-- gateway now refuses that for a closed term, but nothing below the gateway
-- did) and that month became writable again while its report cards still
-- printed it. The lock followed a mapping the close had already frozen.
--
-- WHAT THIS IS. The close snapshots the term's months into the closure row.
-- A closed term locks the UNION of this snapshot and its current mapping, so
-- neither removing nor adding a month in config reopens anything. A re-close
-- rewrites the snapshot; a reopen keeps it (it only matters while closed).
--
-- BACKFILL. None: the rows that exist predate any shared environment (local
-- only), and '{}' reads as "no snapshot" — the current mapping still locks.
--
-- Same shape as report_cards.months: text[] of "YYYY-MM". RLS is unchanged
-- (the table already has ENABLE + FORCE and its tenant policy). Additive only.

BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE period_closures
    ADD COLUMN months text[] NOT NULL DEFAULT '{}';

COMMIT;
