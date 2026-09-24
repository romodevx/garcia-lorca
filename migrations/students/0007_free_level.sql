-- students — a pupil's level is a free reference to the school's catalog.
--
-- WHY. `level` was a fixed SEP list (maternal/kinder/primaria/secundaria/
-- preparatoria) enforced three times: a CHECK here, a proto enum, and a list in
-- the web. A school that names its level "Preescolar", or adds one the list
-- never had, could create it in Configuración and then not place a single pupil
-- in it. The catalog already lives in config (`levels`); the pupil now points
-- at it (ADR 0014).
--
-- WHAT CHANGES.
--   * level_id   — the config level's id (NULL only for a legacy pupil whose old
--                  key matched no config level, or matched more than one).
--   * level_name — the level's name as the school wrote it, captured when the
--                  pupil was placed: lists and filters read it without a call to
--                  config for every row.
--   * The SEP CHECK is dropped and the old `level` column becomes nullable. It
--     is kept, untouched, for reference and removed by a later cleanup.
--     It is NOT a rollback path: once a pupil sits in a level outside the old
--     SEP list, the previous build cannot read that row (its column is NULL),
--     so this change rolls forward only. The new code reads the old column as
--     a fallback name, which covers rows the previous revision writes during
--     the deploy window.
--
-- BACKFILL. Every service shares one database and config's migrations run
-- before students' (lexical order), so `levels` is joined directly. A pupil is
-- linked only when EXACTLY ONE config level of its tenant folds (lowercase, no
-- accents) to its old key — "Kínder" ↔ kinder. No match or two matches: the
-- pupil is left unlinked with a readable name ("Maternal"), never attached to a
-- guess. Where `levels` does not exist (this service's own test databases) the
-- backfill is skipped.
--
-- RLS: students is FORCE ROW LEVEL SECURITY and the cloud migrator owns it
-- without BYPASSRLS, so SET LOCAL binds the one school this deployment serves
-- (same pattern as migrations/iam/0017).

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';
SET LOCAL lock_timeout = '5s';

ALTER TABLE students ADD COLUMN level_id uuid NULL;
ALTER TABLE students ADD COLUMN level_name text NOT NULL DEFAULT '';
ALTER TABLE students DROP CONSTRAINT IF EXISTS students_level_check;
ALTER TABLE students ALTER COLUMN level DROP NOT NULL;

DO $$
BEGIN
    IF to_regclass('levels') IS NULL THEN
        RETURN;
    END IF;
    UPDATE students s
       SET level_id = m.id, level_name = m.name
      FROM (SELECT tenant_id,
                   lower(translate(name, 'ÁÉÍÓÚÜáéíóúü', 'AEIOUUaeiouu')) AS folded,
                   (array_agg(id))[1] AS id,
                   (array_agg(name))[1] AS name
              FROM levels
             GROUP BY 1, 2
            HAVING count(*) = 1) m
     WHERE m.tenant_id = s.tenant_id
       AND m.folded = s.level;
END
$$;

UPDATE students
   SET level_name = initcap(level)
 WHERE level_name = '' AND level IS NOT NULL;

CREATE INDEX idx_students_level_id ON students (tenant_id, level_id);

COMMIT;
