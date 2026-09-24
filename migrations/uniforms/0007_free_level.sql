-- uniforms — garments and orders follow the school's own levels.
--
-- WHY. Same fixed SEP list as students (ADR 0014): a garment could not be
-- scoped to Preescolar, and an order could not snapshot a pupil placed in a
-- level the list lacked. The shop now speaks config level ids.
--
-- WHAT CHANGES.
--   * uniform_items.level_id — the config level a garment is scoped to; NULL
--     still means "every level", the common case.
--   * uniform_orders.student_level_id / student_level_name — the pupil's level
--     at order time (a snapshot, as student_name already is).
--   * Both SEP CHECKs are dropped. The old text columns are kept, untouched,
--     for reference and removed by a later cleanup.
--     It is NOT a rollback path: once a pupil sits in a level outside the old
--     SEP list, the previous build cannot read that row (its column is NULL),
--     so this change rolls forward only. The new code reads the old column as
--     a fallback name, which covers rows the previous revision writes during
--     the deploy window.
--
-- BACKFILL. As in migrations/students/0007: a row is linked only when EXACTLY
-- ONE config level of its tenant folds (lowercase, no accents) to its old key.
-- An order that matches nothing keeps a readable name and no id. A SCOPED
-- garment that matches nothing is the one real risk: with no level_id it reads
-- as "every level". It is not guessed; the NOTICE below counts such garments so
-- the school re-scopes them in Inventario. (Local and dev data have none: every
-- stored garment is unscoped.) Where `levels` does not exist the linking is
-- skipped.
--
-- RLS: both tables are FORCE ROW LEVEL SECURITY and the cloud migrator owns
-- them without BYPASSRLS, so SET LOCAL binds the one school this deployment
-- serves (same pattern as migrations/iam/0017).

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';
SET LOCAL lock_timeout = '5s';

ALTER TABLE uniform_items ADD COLUMN level_id uuid NULL;
ALTER TABLE uniform_items DROP CONSTRAINT IF EXISTS uniform_items_level_check;

ALTER TABLE uniform_orders ADD COLUMN student_level_id uuid NULL;
ALTER TABLE uniform_orders ADD COLUMN student_level_name text NOT NULL DEFAULT '';
ALTER TABLE uniform_orders DROP CONSTRAINT IF EXISTS uniform_orders_student_level_check;

DO $$
DECLARE
    unlinked bigint;
BEGIN
    IF to_regclass('levels') IS NULL THEN
        RETURN;
    END IF;

    CREATE TEMP TABLE level_keys ON COMMIT DROP AS
    SELECT tenant_id,
           lower(translate(name, 'ÁÉÍÓÚÜáéíóúü', 'AEIOUUaeiouu')) AS folded,
           (array_agg(id))[1] AS id,
           (array_agg(name))[1] AS name
      FROM levels
     GROUP BY 1, 2
    HAVING count(*) = 1;

    UPDATE uniform_items i
       SET level_id = k.id
      FROM level_keys k
     WHERE k.tenant_id = i.tenant_id AND k.folded = i.level;

    UPDATE uniform_orders o
       SET student_level_id = k.id, student_level_name = k.name
      FROM level_keys k
     WHERE k.tenant_id = o.tenant_id AND k.folded = o.student_level;

    SELECT count(*) INTO unlinked
      FROM uniform_items
     WHERE level IS NOT NULL AND level_id IS NULL;
    IF unlinked > 0 THEN
        RAISE NOTICE 'uniforms: % scoped garment(s) matched no config level; re-scope them in Inventario', unlinked;
    END IF;
END
$$;

UPDATE uniform_orders
   SET student_level_name = initcap(student_level)
 WHERE student_level_name = '' AND student_level IS NOT NULL;

CREATE INDEX idx_uniform_orders_level_id ON uniform_orders (tenant_id, student_level_id);

COMMIT;
