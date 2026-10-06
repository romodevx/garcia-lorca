-- config — a level's name is unique per school once FOLDED (ADR 0033).
--
-- WHY. levels was UNIQUE (tenant_id, name), which is case- and accent-
-- sensitive: "Preparatoria", "PREPARATORIA" and "Preparatória" were three
-- levels. The legacy student import (EnsurePlacements) matches a spreadsheet's
-- level by name and creates it when nothing matches; with a sensitive key two
-- imports — or one import and a hand-made level — would split one school level
-- in two, and pupils with it. The folded key makes "find or create" a single
-- INSERT … ON CONFLICT (tenant_id, config_fold_name(name)) DO NOTHING, which
-- cannot race.
--
-- THE FOLD (config_fold_name): the Spanish accents and ñ in either case become
-- their base letter, ASCII is lower-cased, runs of whitespace collapse to one
-- space, and the ends are trimmed. lower() runs under COLLATE "C" on purpose:
-- the result must not depend on the database's locale (an index built under
-- one locale and probed under another would be corrupt), and IMMUTABLE must be
-- true for the expression index. Accents outside áéíóúüñ are left as they are.
-- The Go twin is domain.FoldName; the two must stay identical.
--
-- If existing levels already fold alike, the migration STOPS and changes
-- nothing, naming the rows by id (same pattern as 0006): which one is the real
-- level is the school's call, and a level may already hold grades and pupils.
--
-- RLS: levels is FORCE ROW LEVEL SECURITY and the cloud migrator owns it
-- without BYPASSRLS, so SET LOCAL binds the one school this deployment serves.
-- The unique index covers every row of every tenant.

BEGIN;

SET LOCAL app.current_tenant = '00000000-0000-0000-0000-000000000001';
SET LOCAL lock_timeout = '5s';

CREATE FUNCTION config_fold_name(p_name text) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE AS
$$ SELECT btrim(regexp_replace(
       lower(translate(p_name, 'áéíóúüñÁÉÍÓÚÜÑ', 'aeiouunaeiouun') COLLATE "C"),
       '\s+', ' ', 'g')) $$;

DO $$
DECLARE
    clashes text;
BEGIN
    SELECT string_agg(id::text, ', ' ORDER BY folded, id)
      INTO clashes
      FROM (SELECT id, config_fold_name(name) AS folded,
                   count(*) OVER (PARTITION BY tenant_id, config_fold_name(name)) AS spellings
              FROM levels) l
     WHERE spellings > 1;
    IF clashes IS NOT NULL THEN
        RAISE EXCEPTION 'config: level names repeat once case, accents and spacing are ignored; nothing was changed. Rename or merge these levels by hand and re-run: %', clashes
            USING ERRCODE = 'unique_violation';
    END IF;
END
$$;

CREATE UNIQUE INDEX levels_folded_name_key
    ON levels (tenant_id, config_fold_name(name));

COMMIT;
