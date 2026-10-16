-- config — levels list in school order and can be reordered (T8, ADR 0036).
--
-- WHY. ListLevels ordered by name, so a school saw "Bachillerato, Kínder,
-- Maternal, Primaria, Secundaria": alphabetical, not the order a child goes
-- through. levels.sort_order is the level's place (1 = first);
-- ReorderLevels rewrites it, ListLevels answers by (sort_order, name).
--
-- BACKFILL. Existing levels are numbered per tenant by their folded name
-- (config_fold_name, 0019): maternal → kínder/kinder/preescolar/jardín →
-- primaria → secundaria → preparatoria/prepa/bachillerato, then anything else,
-- ties by folded name and id. A prefix match, so "Primaria alta" is primaria.
--
-- NEW LEVELS. A BEFORE INSERT trigger puts a level that arrives without a
-- place (sort_order = 0, the column default) after the last one of its
-- tenant. A trigger, not application code, so every insert path gets it —
-- CreateLevel and the legacy import's INSERT … ON CONFLICT alike. Two levels
-- created at the same instant can draw the same number; the tie is broken by
-- name, and the next reorder separates them. A row that carries its place (a
-- level-trash restore) keeps it.
--
-- LEVEL TRASH. An archived level is rebuilt from its JSONB with
-- jsonb_populate_record, where a missing key comes back NULL: a level trashed
-- before this column would no longer restore (NOT NULL violation, the whole
-- restore rolls back — same trap as grades/0009). Archived levels without the
-- key get a place after every live level of their tenant.
--
-- FREEZE. A level being deleted refuses every change but its lifecycle
-- (0012/0013). Its place in the list is presentation, not structure, so
-- sort_order joins the columns a frozen level may still change: reordering
-- the school's levels must not fail because one of them is in the trash saga.
--
-- RLS: levels and the archive are FORCE ROW LEVEL SECURITY and the cloud
-- migrator owns them without BYPASSRLS, so SET LOCAL binds the one school this
-- deployment serves (same pattern as 0005, 0014, 0016, 0019). ENABLE/FORCE
-- and the policies are untouched.

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';
SET LOCAL lock_timeout = '5s';

ALTER TABLE levels ADD COLUMN sort_order integer NOT NULL DEFAULT 0
    CHECK (sort_order >= 0);

WITH ranked AS (
    SELECT id,
           row_number() OVER (
               PARTITION BY tenant_id
               ORDER BY CASE
                            WHEN f LIKE 'maternal%' THEN 1
                            WHEN f LIKE 'kinder%' OR f LIKE 'preescolar%' OR f LIKE 'pre-escolar%'
                              OR f LIKE 'jardin%' THEN 2
                            WHEN f LIKE 'primaria%' THEN 3
                            WHEN f LIKE 'secundaria%' THEN 4
                            WHEN f LIKE 'prepa%' OR f LIKE 'bachillerato%' THEN 5
                            ELSE 6
                        END,
                        f, id) AS place
      FROM (SELECT id, tenant_id, config_fold_name(name) AS f FROM levels) l
)
UPDATE levels
   SET sort_order = ranked.place
  FROM ranked
 WHERE levels.id = ranked.id;

CREATE INDEX idx_levels_sort_order ON levels (tenant_id, sort_order, name);

CREATE FUNCTION config_level_sort_order_last() RETURNS trigger
    LANGUAGE plpgsql AS
$$
BEGIN
    IF NEW.sort_order = 0 THEN
        SELECT COALESCE(max(sort_order), 0) + 1 INTO NEW.sort_order
          FROM levels WHERE tenant_id = NEW.tenant_id;
    END IF;
    RETURN NEW;
END
$$;

CREATE TRIGGER levels_sort_order_last BEFORE INSERT ON levels
    FOR EACH ROW EXECUTE FUNCTION config_level_sort_order_last();

UPDATE config_deletion_archive a
   SET "row" = a."row" || jsonb_build_object('sort_order',
       (SELECT COALESCE(max(l.sort_order), 0) + 1 FROM levels l WHERE l.tenant_id = a.tenant_id))
 WHERE a.table_name = 'levels' AND NOT (a."row" ? 'sort_order');

CREATE OR REPLACE FUNCTION config_refuse_deleting_level_update() RETURNS trigger
    LANGUAGE plpgsql AS
$$
BEGIN
    IF OLD.status = 'deleting' AND NEW.status = 'deleting'
       AND (to_jsonb(NEW) - 'deleting_batch_id' - 'updated_at' - 'sort_order')
           IS DISTINCT FROM (to_jsonb(OLD) - 'deleting_batch_id' - 'updated_at' - 'sort_order') THEN
        RAISE EXCEPTION 'LEVEL_BEING_DELETED' USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END
$$;

COMMIT;
