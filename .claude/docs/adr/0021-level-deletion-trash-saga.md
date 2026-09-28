# ADR 0021: Eliminar niveles — papelera por archivo, saga en gateway, finanzas intactas

- Status: Accepted
- Date: 2026-09-28
- Context: the director needs to delete an education level (a mistaken
  "Maternal", a level the school closed). A level's data lives in six services
  (config, students, attendance, grades, finance, uniforms), with no
  cross-service FKs. The owner decided: only the director; a level with data
  loses everything ACADEMIC across all cycles, but money and pupils are kept;
  triple validation on the server; a 7-day, fully restorable trash.

## Decisions

### Who

- New permission `levels:delete`, in iam's `ownerOnlyPermissions`: the admin
  grant guard refuses it to any other role. Migration `iam/0024` grants it to
  the owner by fixed id (`…0a1`), like 0021–0023.
- Four gateway routes, all behind `module_config` → auth → rotation →
  `levels:delete`: preview, delete, trash list, restore.

### What a deletion reaches (every cycle)

| service | archived (moved to the trash) | kept |
|---|---|---|
| config | level, grade levels, subjects, groups, terms, term months, teaching assignments, school events aimed at the level or its groups | teachers, cycles, whole-school events |
| students | enrollments in the level's groups (+ their transfers); pupils' `level_id/level_name/level/grade_group` are snapshotted and cleared | the pupil, guardians, contacts, notes, medical, photo, card, `admission_status` |
| attendance | marks (by enrollment, group or subject) and pass-list sessions | — |
| grades | monthly grades, legacy grade entries, period closures, report cards (+ lines), outbox rows of those enrollments/terms | — |
| finance | the level's tuition plans | charges, payments, deposits, receipts, concepts, outbox — never read for writing, never altered |
| uniforms | garments' level: snapshotted, then left in the existing **unresolved** state (ADR 0014: `level_id NULL` beside legacy key `nivel-eliminado`), offered to nobody | the garment, stock, orders (snapshots) |

Kept rows that pointed at deleted ones: charges have no FK to tuition plans
(ids are not referenced), so nothing kept dangles in finance; enrollment
transfers of pupils who left the level for another one keep their historical
`from_group_id` (a plain uuid, no FK).

An **empty** level (no grade levels, no pupils, no enrollments) goes through the
exact same path — its terms, tuition plans and events go to the trash with it —
so there is one code path and an empty deletion is restorable too. The preview
flags it with `empty: true`.

### The trash: per-service archive tables, not `deleted_at`

- Each service owns `<service>_deletion_archive (id, tenant_id, batch_id,
  table_name, row jsonb, position, created_at)`, RLS ENABLE+FORCE, plus a
  one-marker-per-batch partial unique index. Migrations: config/0011 (with
  `level_deletions`), students/0016, attendance/0005, grades/0007,
  finance/0019, uniforms/0010.
- Moving is one statement per table: `WITH moved AS (DELETE … RETURNING *)
  INSERT INTO archive … to_jsonb(moved)` — a row cannot be deleted without
  being filed. Restoring is `INSERT … SELECT r.* FROM archive,
  jsonb_populate_record(NULL::table, row) r … ON CONFLICT DO NOTHING`, parents
  first, with an existence guard per FK parent; skipped = archived − inserted,
  reported per kind with a Spanish reason. Column-clearing deletions (pupils,
  garments) snapshot the columns and restore them only where the row is still
  cleared ("keep the new value").
- Shared code: `pkg/levelarchive` (counts, reports, scope, retention, purge
  loop — framework-free) and `pkg/levelarchive/gormarchive` (the SQL).
- Idempotent per batch: a marker row records the counts (archive) and the
  report (restore); a retried call answers the first result under an advisory
  transaction lock. An unknown batch restores nothing.
- Purge: every service sweeps archives older than 7 days hourly
  (`levelarchive.StartPurger`, detached context, logged); config also marks
  `level_deletions` purged.
- No query anywhere learned a "deleted" filter: a deleted row is simply not in
  its table.

### The saga (gateway)

- Preview: config resolves the structure ids across all cycles → students
  resolves pupils and enrollments in those groups → grades, attendance,
  finance, uniforms count with the full scope. Finance's `charges`/`payments`
  are reported as `keeps`, never as deletions.
- Delete order: **grades → attendance → finance → uniforms → students →
  config**. Dependents go first and the level itself last, so no visible row
  ever points at a structure that is gone. Config's step also records
  `level_deletions` in the same transaction.
- Any failure restores every step that ran — the failing one included, since
  its commit may have landed before the error — newest first, on a context
  detached from the request (a closed tab must not leave half a level). The
  answer is 500 "no se borró nada"; if compensation also fails, the sentence
  carries the batch id and the log has each failed step.
- Restore order is the reverse with config FIRST (structure before what points
  at it), then `MarkLevelDeletionRestored`. A partial restore answers 500 and
  leaves the batch in the trash; retrying finishes it without duplicates.
- Pre-checks: 404 unknown/restored, 410 `TRASH_EXPIRED` (past 7 days or
  purged), 409 `LEVEL_NAME_TAKEN` (trim + case-insensitive; config also
  refuses an exact clash and restores nothing).

### Triple validation (server-side)

1. **Token** — the preview returns `confirmToken = <exp>.<HMAC-SHA256(tenant,
   level, every non-zero count, exp)>`, valid 10 minutes. Delete recomputes the
   preview and verifies the signature against the FRESH counts: any change, an
   expiry, another level or school → 409 `PREVIEW_STALE`. Counts are signed,
   not carried. Key: `LEVEL_DELETION_TOKEN_KEY` (≥ 32 bytes); unset, a random
   per-process key (the gateway runs min-instances=1; a restart only makes an
   open preview stale, never lets one through).
2. **Name** — `confirmName` equals the level name, trimmed, case-insensitive →
   400 `NAME_MISMATCH`.
3. **Password** — iam `VerifyPassword(access_token, password)`; the subject is
   the verified token's, never the body's. 401 `REAUTH_FAILED`. The route runs
   on its own rate limiter, same size as login's (a separate bucket so a
   director's typos cannot lock the school out of signing in).

### Re-authentication in both auth modes

- **Password mode** (no `IDENTITY_PLATFORM_PROJECT`): bcrypt against the hash
  iam stores — exactly what `Login` checks.
- **Identity Platform mode**: iam calls `accounts:signInWithPassword` with the
  web API key (`IDENTITY_PLATFORM_API_KEY`, already wired for invitations),
  `returnSecureToken: false`, and requires the answer's email to be the
  principal's. Credential refusals (INVALID_PASSWORD, INVALID_LOGIN_CREDENTIALS,
  USER_DISABLED, TOO_MANY_ATTEMPTS…) → 401; a bad key or an outage → 500, never
  a silent pass. The local hash is NOT consulted in this mode (it may be a
  stale import). Chosen over "a fresh ID token with auth_time ≤ 5 min" because
  it keeps the contract identical in both modes (the client sends a password),
  needs no client-side re-sign-in flow, and cannot be satisfied by a token
  minted for another purpose. No key in this mode fails closed (500).

## Consequences

- Saga steps call every data service regardless of that module's flag: a
  disabled module's rows still belong to the level. A service that is down
  makes the deletion fail cleanly (compensated), not partial.
- Pupils come out "sin nivel, grado ni grupo" and keep their admission status;
  the office reassigns them. Finance history stays intact and reconciles.
- Restore is best-effort by design: what now conflicts stays as it is and is
  reported (re-enrolled pupil, pupil with a new level, re-scoped garment,
  teacher who left, a cycle deleted meanwhile).
- `LEVEL_DELETION_TOKEN_KEY` should be set in shared environments (Secret
  Manager) before scaling the gateway past one instance.
