# ADR 0014: A pupil's level is the school's own catalog, not a fixed SEP list

- Status: Accepted
- Date: 2026-09-24
- Context: Task 2 of the September adjustments. The owner asked for "permitir
  crear cualquier grado/nivel, incluido Preescolar", and chose the full version
  over a Preescolar→Kínder alias.

## What was actually happening

Config never restricted levels. Its `levels` table is free text, and no grade
ranges exist anywhere. What blocked the school was that **students** (and
**uniforms**, which mirrored it) kept a fixed SEP list as their own level type:

- a proto enum (`students.v1.Level`, `uniforms.v1.Level`: kinder, primaria,
  secundaria, preparatoria, maternal);
- a `CHECK (level IN (...))` on `students.level`, `uniform_items.level` and
  `uniform_orders.student_level`;
- `domain.Level.Valid()`, which also excluded maternal. So maternal pupils
  could not be created at all, even after migration 0004 added it to the CHECK;
- the gateway's `LevelFromName`, and in the web `SEP_LEVELS`/`isSepLevel`
  («…no es un nivel de la SEP…»), plus three hardcoded chip lists;
- a mandatory «clave SEP» in the web's LevelsPanel. The server never required it.

A school could create "Preescolar" in Configuración and then not place a single
pupil in it.

## Decision

Which levels exist is the school's call, made in config. Every other service
**references** a config level instead of owning a list:

- **students**: `level_id uuid` (a config level) plus `level_name text` (its name,
  captured when the pupil is placed, so lists and filters need no call to
  config). `CreateApplicant` takes a `level_id` and resolves it through config's
  `GetLevel` (the `LevelCatalog` port on the existing Connect client). An
  unknown id is `KindInvalid`, and the name always comes from config, never from
  the caller.
- **uniforms**: `Level` is a config level id; empty still means "every level".
  Orders snapshot the pupil's level id and name, read from students.
- **Wire**: purely additive. The enum fields are marked `deprecated` and never
  renumbered; `level_id`/`level_name` are new fields. `buf breaking` is clean.
  JSON: students take `levelId` and answer `level` (the school's name) +
  `levelId`; uniforms use `levelId` and `?levelId=`.
- **Integrity kept**: required fields, and one grade per `(level, ordinal)`.
  Newly added: a grade's name is unique within its level, case-insensitively
  (config 0006).

## Migrations

- `students/0007`, `uniforms/0007`: add the id/name columns and drop the SEP
  CHECKs. The old text columns are left untouched (and nullable) for rollback.
  Existing rows are linked to the config level whose name folds (lowercase,
  no accents) to the old key, **only when exactly one matches**. Anything else
  stays unlinked with a readable name and is never guessed. Because every
  service shares one database and config migrates first, the backfill joins
  `levels` directly.
- `config/0006`: the unique grade name per level; existing duplicates stop it,
  named by id.

## Consequences

- A pupil with no `level_id` (a legacy row that matched nothing) keeps a name
  and reads as "level unknown": uniforms offers them only every-level garments.
- A **scoped** garment whose old key matched nothing would read as "every
  level". The migration NOTICEs any such garment; local and dev data have none.
- The family app shows the school's level name as is. Qualitative grades apply
  to kínder, matched folded, **and to "Preescolar"**, which is kínder's official
  name. This is a decision for the owner to confirm.
- The old `level` columns and the deprecated enum fields are dead weight, to be
  removed by a scheduled cleanup once no rollback to the SEP build is possible.
