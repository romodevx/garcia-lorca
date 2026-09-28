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
  `level_deletions`; 0012 adds the lock), students/0016, attendance/0005, grades/0007,
  finance/0019, uniforms/0010.
- Moving is one statement per table: `WITH moved AS (DELETE … RETURNING *)
  INSERT INTO archive … to_jsonb(moved)` — a row cannot be deleted without
  being filed. Restoring is `INSERT … SELECT r.* FROM archive,
  jsonb_populate_record(NULL::table, row) r … ON CONFLICT DO NOTHING`, parents
  first, with an existence guard per FK parent; skipped = archived − inserted,
  reported per kind with a Spanish reason. **The same statement deletes from
  the archive exactly the rows its INSERT returned** (joined on the table's
  primary key, read from the catalog): a skipped row STAYS archived until the
  purge, so the director can clear the clash and retry, and the retry brings
  back exactly those rows. Column-clearing deletions (pupils, garments)
  snapshot the columns and restore them only where the row is still cleared
  ("keep the new value"); an applied snapshot leaves the archive (that choice
  is final, not a retry).
- Shared code: `pkg/levelarchive` (counts, reports, scope, retention, purge
  loop — framework-free) and `pkg/levelarchive/gormarchive` (the SQL).
- Idempotent per batch (the batch id is every service's idempotency key): a
  marker row records the counts (archive) and the report (restore) under an
  advisory transaction lock. A retried archive answers its first counts. A
  restore **converges**: it retries only what is still archived, accumulates
  what came back, and a batch with nothing left answers its last report.
- **A restore of a batch a service holds no archive for (never archived, or
  purged) is NotFound** (`levelarchive.ErrBatchNotFound`), never an empty
  success; the gateway decides whether that is fine (a step that may never
  have archived) or an error.
- Restore order across services is config → students → uniforms/finance →
  attendance/grades. Students answers the batch's enrollments it could NOT
  restore (`unrestored_enrollment_ids`); grades and attendance receive them as
  `skip_enrollment_ids` and keep those enrollments' rows archived (reported as
  skipped) instead of bringing them back orphaned.
- No query anywhere learned a "deleted" filter: a deleted row is simply not in
  its table.

### One clock, one sweeper

- `restorable_until` is written once, by config, from the **database's
  `now()`** at `BeginLevelDeletion` + 7 days. Config judges the restore window
  on the same clock; no service compares its own `created_at` any more.
- The data services scale to zero, so their hourly tickers were removed. The
  **gateway** (min-instances=1, always warm) runs the sweep hourly
  (`levelarchive.StartPurger`, detached context, logged per run) and lazily
  before the trash is listed, a level is deleted or a batch is restored:
  config's `SweepLevelDeletions` answers the batches past `restorable_until`
  and not yet purged; every service drops them with `PurgeLevelBatch(batch)`;
  only then config's `MarkLevelDeletionPurged` closes the batch (a batch one
  service could not purge is retried next run). The sweep also turns a batch
  stuck in `deleting` for 30 minutes (a gateway that died mid-saga) into
  `failed_partial` with every step pending, so it becomes restorable.
  `PurgeLevelArchives` (age-based) stays in the protos for compatibility,
  unused.

### The lock and the freeze

- `levels.status` (config migration 0012): `active` | `deleting`. The saga's
  FIRST step is config's `BeginLevelDeletion(level, batch, token hash, who)`:
  under the level's row lock it sets `deleting` and records the batch as
  `deleting`. A partial unique index allows one running deletion per level. A
  second request (a double click) gets 409 `DELETION_IN_PROGRESS` — already
  at the preview when the level is `deleting`, or from `BeginLevelDeletion` on
  losing the race — and never runs a second saga.
- While `deleting`, every write that resolves the level is refused with 409
  `LEVEL_BEING_DELETED` ("Este nivel se está eliminando"): in config by
  triggers on the structure tables and the level itself (migration 0012,
  mapped in the repository's single `withTenant`), in grades (monthly and
  legacy capture), attendance (saving a list) and students (enroll,
  enroll-into-grade, transfer, set level) at the config level lookup each
  already made — config's `Level.status` is on the wire. Reads keep working.
  The check-then-write window in the remote services is milliseconds before a
  step that runs after `BeginLevelDeletion` committed; accepted.
- Compensation that fully succeeds calls `CancelLevelDeletion` (batch
  `cancelled`, level `active`); a restore brings the level back `active`
  (the archived row, filed while `deleting`, is patched before reinsert).
  A `failed_partial` batch keeps the level frozen until it is restored.

### The saga (gateway)

- Preview: config resolves the structure ids across all cycles → students
  resolves pupils and enrollments in those groups → grades, attendance,
  finance, uniforms count with the full scope. Finance's `charges`/`payments`
  are reported as `keeps`, never as deletions.
- Delete order: **config lock → grades → attendance → finance → uniforms →
  students → config archive**. Dependents go first and the level itself last, so no visible row
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

**Superseded by the review fixes (2026-09-28):**

- Compensation runs on `context.WithoutCancel` of the saga context with its
  own 5-minute budget (the saga's deadline may be exactly why it failed), in
  the restore order over the steps that ran; NotFound means nothing to undo.
  Fully undone → `CancelLevelDeletion` and 500 "no se borró nada". Otherwise
  config records the batch `failed_partial` with `pending_steps` (steps that
  failed to restore or whose skipped rows stay archived) and `skipped`, the
  trash lists it ("Eliminación incompleta — Restaurar para recuperar lo que se
  alcanzó a borrar") and the answer is 500 `DELETION_INCOMPLETE` naming the
  folio. "Nothing was deleted" is said only when it is true.
- Restore: the name check skips the batch's own level id (a retry after a
  partial restore used to hit `LEVEL_NAME_TAKEN` forever). A `deleted` batch
  asks every step and a missing archive is an error: 410 `TRASH_EXPIRED` when
  nothing came back, 500 `RESTORE_INCOMPLETE` naming the part otherwise. A
  `failed_partial` (or `restored_partial`) batch asks its pending steps and
  tolerates NotFound (a step that never archived). The batch is closed only
  when every step answered: `restored`, or `restored_partial` when rows were
  skipped (it stays in the trash; a retry tries them again until the purge).
- Ledger statuses: `deleting`, `deleted`, `failed_partial`,
  `restored_partial`, `restored`, `cancelled`, `purged`. The trash lists
  `deleted`, `failed_partial`, `restored_partial`.

### Triple validation (server-side)

1. **Token** — the preview returns `confirmToken = <exp>.<nonce>.<HMAC-SHA256(tenant,
   level, every non-zero count, exp, nonce)>`, valid 10 minutes, and
   **single-use**: `BeginLevelDeletion` stores its SHA-256 in
   `level_deletions.token_hash` (unique) and a reuse is 409 `PREVIEW_STALE`.
   The random nonce keeps two previews of the same data distinct. Delete recomputes the
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
  a silent pass. The key travels in the `X-Goog-Api-Key` header, never the
  URL, and transport errors are stripped of the URL so no message can carry
  it (same for the invitation calls). The local hash is NOT consulted in this mode (it may be a
  stale import). Chosen over "a fresh ID token with auth_time ≤ 5 min" because
  it keeps the contract identical in both modes (the client sends a password),
  needs no client-side re-sign-in flow, and cannot be satisfied by a token
  minted for another purpose. No key in this mode fails closed (500).

## Consequences

- Saga steps call every data service regardless of that module's flag: a
  disabled module's rows still belong to the level. A service that is down
  makes the deletion fail cleanly (compensated), not partial.
- New migration: config/0012 (`levels.status`, the ledger's lifecycle,
  token hash, pending steps, skips, `purged_at`, the freeze triggers).
- Pupils come out "sin nivel, grado ni grupo" and keep their admission status;
  the office reassigns them. Finance history stays intact and reconciles.
- Restore is best-effort by design: what now conflicts stays as it is and is
  reported (re-enrolled pupil, pupil with a new level, re-scoped garment,
  teacher who left, a cycle deleted meanwhile).
- `LEVEL_DELETION_TOKEN_KEY` must be shared by every gateway instance (see
  Deploy notes); without it previews go randomly stale behind Cloud Run.

## Deploy notes

- **Gateway `LEVEL_DELETION_TOKEN_KEY`** (HMAC key, >= 32 bytes). Terraform
  generates it (`random_password.level_deletion_token_key`, 64 chars) into the
  Secret Manager secret `garcia-lorca-level-deletion-token-key-<env>`
  (`deploy/terraform/secrets.tf`) and Cloud Run injects it via
  `local.gateway_secret_env` (`deploy/terraform/services.tf`); the gateway
  module depends on the secret version so the first revision never boots
  without it. The runtime SA already holds `secretmanager.secretAccessor`.
  `ignore_changes = [secret_data]`: rotate deliberately with
  `terraform apply -replace=random_password.level_deletion_token_key` — the
  only cost is that previews in flight return PREVIEW_STALE once.
- **Local:** `docker-compose.yml` gives the gateway a fixed, non-secret dev
  value (overridable from `.env`), so previews survive a gateway restart.
- **iam `IDENTITY_PLATFORM_API_KEY`** needs no new wiring: terraform already
  passes `var.identity_platform_api_key` to iam when
  `enable_identity_platform = true`, and compose reads it from `.env` (the same
  variable the web gets as `NEXT_PUBLIC_FIREBASE_API_KEY`). An environment that
  enables Identity Platform but leaves the key empty makes level deletion fail
  closed (500) at the re-authentication step.
- Rollout: the first `terraform apply` after this change creates the secret
  and a new gateway revision; no manual step.
