-- uniforms — an article has no clave (task 3).
--
-- WHY. The office had to invent a unique code for every article before it could
-- sell it, and nothing used it: orders point at uniform_items.id, the ledger at
-- the stock row, finance at the order. The code was a label that only got in the
-- way (checked before removing: no FK, join, search or lookup reads it).
--
-- WHAT CHANGES. The column stops being required and unique, and is no longer
-- written or read by the service. It is KEPT, with the values already typed, and
-- dropped by a later cleanup migration.
--
-- THE DEFAULT IS '-' AND NOT ''. During the deploy window the previous revision
-- still reads every row through a constructor that requires a code of 1 to 40
-- characters; a new article with an empty code would make its catalogue fail.
-- '-' satisfies it and means nothing. Uniqueness is dropped, so any number of
-- articles can carry it.
--
-- Schema only: no row is read or written, so no tenant needs binding.

BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE uniform_items DROP CONSTRAINT IF EXISTS uniform_items_tenant_id_code_key;
ALTER TABLE uniform_items DROP CONSTRAINT IF EXISTS uniform_items_code_check;
ALTER TABLE uniform_items ALTER COLUMN code SET DEFAULT '-';

COMMIT;
