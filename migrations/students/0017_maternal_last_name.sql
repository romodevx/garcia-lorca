-- students — the maternal surname gets its own field, and lists read by surname (T3).
--
-- The school reads its lists "Apellido paterno Apellido materno Nombre(s)" and
-- sorts them by surname. Until now there was one surname column; from here on
-- `last_name` holds the PATERNAL surname and `maternal_last_name` the maternal
-- one. Existing rows are NOT transformed (the owner's call): a `last_name` that
-- already holds both surnames stays as it is and is corrected by editing the
-- pupil. NULL means "not captured".
--
-- `es_primary` compares at ICU strength 1: accents and case do not move a pupil
-- ("Álvarez" and "alvarez" are the same surname for ordering), while ñ stays its
-- own letter after n, as Spanish sorts it. Nondeterministic, so it is only used
-- to ORDER — never for uniqueness. pkg/studentname sorts the same way in Go.
--
-- The index serves ListStudents' order — surnames then first name, id as the
-- tie-break that keeps keyset pagination stable. Its expressions must match the
-- ORDER BY in services/students/internal/adapters/postgres/repository.go.
-- Additive only.
BEGIN;

SET LOCAL lock_timeout = '5s';

CREATE COLLATION IF NOT EXISTS es_primary (
    provider = icu, locale = 'es-u-ks-level1', deterministic = false
);

ALTER TABLE students ADD COLUMN IF NOT EXISTS maternal_last_name text;

CREATE INDEX IF NOT EXISTS idx_students_surname_order
    ON students (
        tenant_id,
        (last_name COLLATE es_primary),
        (COALESCE(maternal_last_name, '') COLLATE es_primary),
        (first_name COLLATE es_primary),
        id
    );

COMMIT;
