-- students (M16) — admit the Maternal level.
--
-- The school teaches maternal (nursery, below kinder) and could not enrol a
-- single one of those pupils: the CHECK in 0001 listed only the four SEP levels,
-- so every attempt was rejected at the database. The front end even carried a
-- comment saying "un nivel llamado 'Maternal' no tiene dónde poner uno" — the
-- gap was known and worked around instead of closed.
--
-- Widening a CHECK is additive: every existing row still satisfies the new
-- constraint, so this cannot fail on data and needs no backfill.
--
-- NOTE for whoever adds the next level: this is the THIRD place that enumerates
-- levels (students, uniforms×2) alongside the config.levels table, which is
-- free-form. They disagree by construction. Consolidating them is real work and
-- is deliberately not attempted here; adding a level today means editing all
-- three plus two proto enums.
BEGIN;

ALTER TABLE students DROP CONSTRAINT IF EXISTS students_level_check;
ALTER TABLE students ADD CONSTRAINT students_level_check
    CHECK (level IN ('maternal', 'kinder', 'primaria', 'secundaria', 'preparatoria'));

COMMIT;
