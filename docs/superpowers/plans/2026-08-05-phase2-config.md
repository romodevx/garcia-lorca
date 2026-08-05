# Phase 2 — `config` Service (Academic Catalog) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Stand up the `config` microservice — the academic-structure catalog the whole spine hangs off: **cycles → levels → grade_levels → groups → subjects → terms**, with the hybrid cycle model (per-level `enrollment_granularity`). Backend-first (TDD), then gateway routes behind `module_config`, then admin screens.

**Architecture:** Clean Architecture, mirroring `iam`/`students` (`cmd/main.go` + `internal/{domain,usecase,ports,adapters/{postgres,grpc}}`). Domain imports no framework; GORM only in `adapters/postgres`; RLS bound via the consolidated `pkg/authctx.WithTenantContext` (`app.current_tenant`). Single-tenant M-phase (tenant fixed at construction, like iam/students). All six entities live in ONE service/DB, so intra-catalog references are **real FKs** (no cross-service RPC within config).

**Tech Stack:** Go 1.24, GORM (adapters only), Connect/gRPC, Postgres 16 + RLS, buf, pnpm/Next.js (web sub-phase), testcontainers-go.

**Design source:** `docs/superpowers/specs/2026-07-31-academic-spine-design.md` §5.1 (this plan turns that model into DDL/proto/code).

---

## Delivery model

Integration branch `feature/phase2-config` (off `dev`, already created). Three sub-phases, **each its own branch off the integration branch, merged back with `--no-ff`**, with a checkpoint:

| Sub-phase | Branch | Content |
|-----------|--------|---------|
| **2.1 Backend** | `feature/phase2-config-be` | Service scaffold + proto + migration/RLS + domain/usecase/ports/adapters for the 6 entities (TDD, integration + RLS isolation). |
| **2.2 Gateway** | `feature/phase2-config-gw` | Config client + REST routes behind `module_config` (RequireAuth → RequirePasswordChanged → permission); flag wiring. |
| **2.3 Web** | `feature/phase2-config-web` | Admin catalog screens (fewest-clicks), FlagsProvider gating, Cmd+K registration. |

**Per-sub-phase DoD:** gofmt/vet clean; unit + integration (testcontainers, RLS isolation) green; buf lint + breaking (vs `dev`) clean; flag-ON/OFF where relevant; no secret literals (test fixtures use neutral literals; `.gitguardian.yaml` already excludes `*_test.go`); tech-lead + (for the flag) security gates. `gen/` and `go.work.sum` never staged. buf runs via Docker; base branch is `dev`.

---

## Relational model → DDL (sub-phase 2.1)

Cross-cycle **catalog**: `levels`, `grade_levels`, `subjects`. Per-cycle: `cycles`, `terms`, `groups`. All tenant-scoped, `ENABLE + FORCE RLS`, policy `tenant_isolation` on `app.current_tenant` (identical shape to iam/students `0001`). Intra-config FKs are real.

```
levels (catalog)                         cycles (per-cycle umbrella)
  id, tenant_id, name, sep_code,           id, tenant_id, name "2026-2027",
  enrollment_granularity ('CYCLE'|'TERM'), status ('draft'|'active'|'closed'),
  created_at, updated_at                   starts_on, ends_on, created_at, updated_at
  UNIQUE(tenant_id, name)                  UNIQUE(tenant_id, name)
   │1─N
   ├── grade_levels (grados, catalog)
   │     id, tenant_id, level_id→levels ON DELETE RESTRICT,
   │     name "1° primaria", ordinal, created_at, updated_at
   │     UNIQUE(tenant_id, level_id, ordinal)
   │      │1─N
   │      └── subjects (materias, catalog)
   │            id, tenant_id, grade_level_id→grade_levels ON DELETE RESTRICT,
   │            name "Matemáticas", created_at, updated_at
   │            UNIQUE(tenant_id, grade_level_id, name)
   │
   └── terms (periodos, per (cycle, level))
         id, tenant_id, cycle_id→cycles ON DELETE CASCADE, level_id→levels ON DELETE RESTRICT,
         name "T1"/"2026-A", ordinal, starts_on, ends_on, created_at, updated_at
         UNIQUE(tenant_id, cycle_id, level_id, ordinal)

groups (grupos, per (cycle, grade_level))
  id, tenant_id, cycle_id→cycles ON DELETE CASCADE, grade_level_id→grade_levels ON DELETE RESTRICT,
  name "A", capacity int, created_at, updated_at
  UNIQUE(tenant_id, cycle_id, grade_level_id, name)
```

Notes: `ON DELETE RESTRICT` on catalog FKs (can't drop a level that has grades/terms); `CASCADE` from `cycles` (closing/removing a draft cycle removes its groups/terms). All FKs are `(tenant_id, ...)`-safe because ids are globally unique UUIDs. `teachers`/`teaching_assignments` are **Phase 3, not here**.

---

## Proto contract (sub-phase 2.1)

`proto/garcialorca/config/v1/config.proto`, package `garcialorca.config.v1`, ONE `ConfigService`. Per entity: `Create<E>`, `Get<E>`, `List<E>s` (list scoped to parent where natural). Plus `Health`. Enums: `EnrollmentGranularity {UNSPECIFIED, CYCLE, TERM}`, `CycleStatus {UNSPECIFIED, DRAFT, ACTIVE, CLOSED}`. Timestamps via `google/protobuf/timestamp.proto`. Mark it a frozen contract once merged.

List RPCs and their parent scoping:
- `ListCycles()` — all cycles (tenant).
- `ListLevels()` — all levels (catalog).
- `ListGradeLevels(level_id)` — grades of a level.
- `ListSubjects(grade_level_id)` — subjects of a grade.
- `ListTerms(cycle_id)` — terms of a cycle (optionally filter by level_id).
- `ListGroups(cycle_id)` — groups of a cycle (optionally filter by grade_level_id).

---

# Sub-phase 2.1 — Backend (`feature/phase2-config-be`)

Branch: `git checkout -b feature/phase2-config-be feature/phase2-config`.

## Task 2.1.0: Scaffold the `config` module

**Files:**
- Create: `services/config/go.mod`, `services/config/cmd/main.go`, `services/config/internal/{domain,usecase,ports,adapters/{postgres,grpc}}/doc.go`
- Modify: `go.work`, `docker-compose.yml`

- [ ] **Step 1: go.mod + workspace**

Create `services/config/go.mod`:
```
module github.com/garcia-lorca/services/config

go 1.24

require github.com/garcia-lorca/pkg v0.0.0
```
Add `./services/config` to the `use (...)` block in `go.work` (alphabetical, before `docs`). Run `go work sync`.

- [ ] **Step 2: doc.go stubs** — add a one-line `package <name>` doc.go in each `internal/{domain,usecase,ports,adapters/postgres,adapters/grpc}` (mirror iam's `doc.go` style) so the tree compiles empty.

- [ ] **Step 3: compose service** — in `docker-compose.yml`, add a `config` service using the shared `*go-service` anchor (mirror `students`), next free port `8101`:
```yaml
  config:
    <<: *go-service
    environment: { <<: *go-env, SERVICE: config }
    ports: ["8101:8080"]
```
(The `migrate` one-shot already auto-applies `migrations/config/*.sql`; no compose change needed for migrations.)

- [ ] **Step 4: verify skeleton builds** — `cd services/config && go build ./... ` (empty packages compile). Commit:
```bash
git add services/config go.work docker-compose.yml
git commit -m "feat(config): scaffold Clean-Arch service module + compose entry"
```

## Task 2.1.1: Migration + RLS

**Files:** Create `migrations/config/0001_init.sql`

- [ ] **Step 1: write the migration** — the 6 tables above, each with `ENABLE + FORCE ROW LEVEL SECURITY` and a `tenant_isolation` policy identical in shape to `migrations/iam/0001_init.sql` (`USING/WITH CHECK (tenant_id = current_setting('app.current_tenant', true)::uuid)`). Wrap in `BEGIN; … COMMIT;`. Include the CHECK constraints for enums (`enrollment_granularity IN ('CYCLE','TERM')`, `status IN ('draft','active','closed')`), the UNIQUE constraints, FKs with the ON DELETE rules above, and indexes on every FK + `(tenant_id)`. No seed data.

- [ ] **Step 2: apply + verify** — `docker compose down -v && docker compose up -d postgres && docker compose run --rm migrate`, then `docker compose exec -T postgres psql -U app -d garcia_lorca -c "\dt"` shows `cycles, grade_levels, groups, levels, subjects, terms`. Confirm RLS with a quick tenant-scoped insert/select as `app` (unset tenant → 0 rows). Tear down.

- [ ] **Step 3: commit** — `git add migrations/config/0001_init.sql && git commit -m "feat(config): schema + RLS for cycles/levels/grade_levels/groups/subjects/terms"`

## Task 2.1.2: Proto contract + generate

**Files:** Create `proto/garcialorca/config/v1/config.proto`

- [ ] **Step 1: write the proto** — `ConfigService` with `Health` + Create/Get/List for the 6 entities and the enums/messages per the "Proto contract" section. Field numbering contiguous; parent-id fields on nested Create/List. Doc-comment each RPC.
- [ ] **Step 2: generate + lint** — `docker run --rm -v "$PWD":/work -w /work bufbuild/buf:latest generate`, then `... lint`, then `... breaking --against '.git#branch=dev'` (new file → no breaking). Build gen: `cd gen/go && go build ./...`.
- [ ] **Step 3: commit** (proto only; `gen/` is gitignored) — `git add proto/garcialorca/config/v1/config.proto && git commit -m "feat(config): proto contract — academic catalog ConfigService"`

## Task 2.1.3: Domain — the `Cycle` entity (TEMPLATE, fully worked)

The other five entities follow this exact shape; 2.1.4 specifies each one's fields/invariants.

**Files:** Create `services/config/internal/domain/cycle.go`, `cycle_test.go`, and value objects as needed.

- [ ] **Step 1: failing test** (`cycle_test.go`, package `domain_test`):
```go
func TestNewCycleValidates(t *testing.T) {
	_, err := domain.NewCycle("", "t1", "2026-2027", domain.CycleStatusDraft, someDate, laterDate)
	if glerrors.KindOf(err) != glerrors.KindInvalid { t.Fatalf("empty id should be Invalid, got %v", glerrors.KindOf(err)) }

	c, err := domain.NewCycle("c1", "t1", "2026-2027", domain.CycleStatusDraft, someDate, laterDate)
	if err != nil { t.Fatalf("valid cycle: %v", err) }
	if c.Name() != "2026-2027" || c.Status() != domain.CycleStatusDraft { t.Error("accessors") }

	_, err = domain.NewCycle("c1", "t1", "2026-2027", domain.CycleStatusDraft, laterDate, someDate)
	if glerrors.KindOf(err) != glerrors.KindInvalid { t.Error("ends_on before starts_on must be Invalid") }
}
```
- [ ] **Step 2: run → fail.**
- [ ] **Step 3: implement** `cycle.go`: a `CycleStatus` type (Draft/Active/Closed) with parse/String; a `Cycle` struct with unexported fields + accessors; `NewCycle(id, tenantID, name string, status CycleStatus, startsOn, endsOn time.Time) (Cycle, error)` validating: id/tenantID/name non-empty (`glerrors.KindInvalid`), `endsOn` not before `startsOn`. No framework imports.
- [ ] **Step 4: run → pass.** Commit with 2.1.4.

## Task 2.1.4: Domain — the remaining five entities

Follow the `Cycle` template (unexported fields + accessors + `New<E>` validator returning `glerrors.KindInvalid` on bad input; no framework imports). Each with its own `<entity>.go` + `<entity>_test.go`. Exact specs:

- [ ] **`Level`** (`level.go`): `NewLevel(id, tenantID, name, sepCode string, gran EnrollmentGranularity)`; `EnrollmentGranularity` type (Cycle/Term) with parse/String; validate id/tenantID/name non-empty and `gran` known. Accessors incl. `Granularity()`.
- [ ] **`GradeLevel`** (`grade_level.go`): `NewGradeLevel(id, tenantID, levelID, name string, ordinal int)`; validate ids/name non-empty, `ordinal >= 1`.
- [ ] **`Subject`** (`subject.go`): `NewSubject(id, tenantID, gradeLevelID, name string)`; validate non-empty.
- [ ] **`Term`** (`term.go`): `NewTerm(id, tenantID, cycleID, levelID, name string, ordinal int, startsOn, endsOn time.Time)`; validate non-empty, `ordinal >= 1`, `endsOn` not before `startsOn`.
- [ ] **`Group`** (`group.go`): `NewGroup(id, tenantID, cycleID, gradeLevelID, name string, capacity int)`; validate non-empty, `capacity >= 0`.

Each: failing test first (valid construct + one invalid case per rule), then implement, then green. Commit:
```bash
git add services/config/internal/domain
git commit -m "feat(config): domain entities + invariants (cycle/level/grade_level/subject/term/group)"
```

## Task 2.1.5: Ports + usecase (CRUD)

**Files:** `services/config/internal/ports/ports.go`; `services/config/internal/usecase/catalog.go` + `catalog_test.go`; `internal/usecase/health.go` (copy iam's).

- [ ] **Step 1: ports** — one repository interface per aggregate OR a single `CatalogRepository` with `Create<E>/Get<E>/List<E>` methods (prefer a single interface; it's one service). Methods take `ctx` + domain entities/ids, return domain types + `error`. Parent-existence checks: `CreateGradeLevel` etc. validate the parent exists (repo `Get<Parent>` or a FK error surfaced as `KindInvalid`/`KindNotFound`).
- [ ] **Step 2: usecase tests first** (`catalog_test.go`, fakes implementing the repo): for each entity — create returns it; create with a missing parent → `KindNotFound`/`KindInvalid`; list returns parent-scoped items; duplicate (unique violation from repo) → `KindConflict`. Use neutral test literals.
- [ ] **Step 3: implement** `catalog.go` — a `CatalogService` orchestrating validation + repo calls (mirror `students` usecase style). Generate ids via an injected `func() string` (uuid), `now func() time.Time` injected for determinism.
- [ ] **Step 4: run → pass.** Commit:
```bash
git add services/config/internal/ports services/config/internal/usecase
git commit -m "feat(config): catalog use cases (CRUD + parent validation)"
```

## Task 2.1.6: Postgres adapter + RLS integration tests

**Files:** `services/config/internal/adapters/postgres/{models.go,mapper.go,repository.go,txrunner.go,repository_integration_test.go}`

- [ ] **Step 1: txrunner.go** — copy `services/iam/internal/adapters/postgres/txrunner.go` verbatim (the `gormTxRunner`/`gormTx`/`txGorm` adapter for `authctx`), package `postgres`.
- [ ] **Step 2: models + mapper** — GORM row structs (`cycleModel`, `levelModel`, …) with `TableName()`; `mapper.go` translating rows ↔ domain (double model). Enum columns are text; map to/from the domain enum types.
- [ ] **Step 3: repository** — a `Repository{db, tenantID}` whose `withTenant` delegates to `authctx.WithTenantContext(ctx, gormTxRunner{db}, tenantID, …)` (mirror iam). Implement Create/Get/List per entity; unique-violation → `glerrors.KindConflict`; not-found → `KindNotFound`; explicit `Preload`/joins for parent-scoped lists (no N+1).
- [ ] **Step 4: integration test** (`//go:build integration`, mirror iam's harness: testcontainers-go, apply `migrations/config/*.sql`, connect as non-owner `app`): round-trip each entity; a `TestRLSIsolatesTenants` proving tenant A can't see/write tenant B for `cycles` (representative); a parent-FK test (creating a group for a nonexistent grade_level fails). Use neutral literals.
- [ ] **Step 5: run** — `cd services/config && go test ./... && go test -count=1 -tags=integration ./internal/adapters/postgres/`. Commit:
```bash
git add services/config/internal/adapters/postgres
git commit -m "feat(config): postgres adapter (authctx RLS) + integration & isolation tests"
```

## Task 2.1.7: gRPC handler + composition root

**Files:** `services/config/internal/adapters/grpc/{config_handler.go,config_handler_test.go,health.go}`; `services/config/cmd/main.go`

- [ ] **Step 1: handler** — implement the generated `configv1connect.ConfigServiceHandler`: translate proto ↔ usecase, map `glerrors.Kind` → Connect codes (copy iam's `connectErr`). Compile-time `var _ configv1connect.ConfigServiceHandler = (*ConfigHandler)(nil)`.
- [ ] **Step 2: handler tests** — table-driven: each Create/Get/List happy path + one error mapping (e.g. missing parent → CodeNotFound, duplicate → CodeAlreadyExists) using usecase-level fakes.
- [ ] **Step 3: main.go** — composition root mirroring iam: `DATABASE_URL`, `CONFIG_TENANT_ID` (default `00000000-0000-0000-0000-000000000001`), open GORM, wire repo→usecase→handler, mount the Connect service under h2c, plus `/healthz`.
- [ ] **Step 4: run** — `cd services/config && go test ./... && go build ./...`. Commit:
```bash
git add services/config/internal/adapters/grpc services/config/cmd
git commit -m "feat(config): gRPC handler + composition root"
```

**Checkpoint 2.1** — merge `feature/phase2-config-be` into `feature/phase2-config` (`--no-ff`).

---

# Sub-phase 2.2 — Gateway (`feature/phase2-config-gw`)

Branch off the updated integration branch. Expose the catalog through the edge, flag-gated.

**Files:** `services/gateway/internal/client/config.go`; `services/gateway/internal/handler/config.go` + test; `services/gateway/cmd/main.go`.

- [ ] **Task 2.2.1: config client** — `ConfigClient` over the generated `configv1connect.ConfigServiceClient` (mirror `client/students.go`), small methods per RPC returning gateway-friendly structs.
- [ ] **Task 2.2.2: handler** — REST/JSON handlers for the catalog (Create/List per entity), translating JSON ↔ client, `httpStatusFor` mapping. TDD with a fake client.
- [ ] **Task 2.2.3: routes** — in `newMux`, add a `CONFIG_BASE_URL` client and register the routes, each behind `guard.Protect("config", requireAuthChanged(RequirePermission("config:write"|"config:read")(...)))`. Reads use `config:read`, writes `config:write`. All under `guard.Protect("config", …)` so `module_config` OFF ⇒ 404. Add `config` to the compose gateway `depends_on`/env (`CONFIG_BASE_URL: http://config:8080`).
- [ ] **Task 2.2.4: permissions** — add a migration `migrations/iam/0003_config_permissions.sql` appending `config:read`,`config:write` to the seeded owner role (`UPDATE roles SET permissions = array_cat(permissions, ARRAY['config:read','config:write']) WHERE name='owner' AND tenant_id='…0001'` — idempotent via a `WHERE NOT permissions @> …` guard). Owner needs these to use the catalog.
- [ ] **Verify** — gateway `go test ./...`; E2E: with `ENABLED_MODULES` including `config`, owner logs in → creates a cycle via the gateway → lists it; with `config` absent → route 404s (flag boundary). buf n/a. Commit per task.

**Checkpoint 2.2** — merge `feature/phase2-config-gw` into `feature/phase2-config`.

---

# Sub-phase 2.3 — Web (`feature/phase2-config-web`)

Admin catalog UI, fewest-clicks, gated by `module_config` (UX-only; gateway is the boundary).

**Files:** `apps/web/app/config/api.ts` (+test); catalog screens under `apps/web/app/_screens/config/` (or App Router routes under `app/(admin)/config/`); wire into `AppShell`/nav + Cmd+K.

- [ ] **Task 2.3.1: api client** — fetch helpers (`credentials:"include"`) for the catalog endpoints; typed results; throw on non-ok. Vitest with mocked fetch.
- [ ] **Task 2.3.2: screens** — a catalog manager: pick/create cycle → manage levels → grade_levels → groups/subjects → terms, each an inline list + "add" form with sensible defaults (auto-suggest group name A/B/C; prefill active cycle). Failing component test first (Vitest/RTL) per screen. Gate the surface behind `useFlag("module_config")` (friendly stub when OFF).
- [ ] **Task 2.3.3: command palette + nav** — register catalog nav + "create cycle/level/group…" commands (repo DoD: a module absent from the palette isn't done).
- [ ] **Verify** — `pnpm --filter @garcia-lorca/web test && tsc --noEmit && lint`; production build has no source maps (repo rule). Commit per task.

**Checkpoint 2.3** — merge `feature/phase2-config-web` into `feature/phase2-config`.

---

## Final gates (after 2.1–2.3 merged into `feature/phase2-config`)
- Full suite (Go race + iam/students/config integration incl. RLS isolation; web) green.
- buf lint + breaking vs `dev` (only additive config proto).
- Security gate: RLS audit on the 6 new tables (isolation tests), flag boundary (`module_config` OFF ⇒ 404), permission gating (`config:read/write`), no secret literals.
- Tech-lead gate: canon review.
- Update `.claude/docs/MODULES.md` (M04 → in progress/done) + `FLAGS.md` note; then hand `feature/phase2-config` to the user for review (do NOT merge to `dev`).

## Out of scope (later phases)
`teachers` + `teaching_assignments` (Phase 3), student `enrollments` (Phase 4), grade capture (Phase 5). This plan builds ONLY the six catalog entities.

## Testing strategy
Unit (domain invariants, usecase orchestration, handler mapping), integration (testcontainers, RLS isolation per new table), contract (endpoints vs proto/OpenAPI), web (Vitest/RTL), flag-ON/OFF for the gateway routes. Deterministic (injected id/clock). Neutral test literals only (`.gitguardian.yaml` excludes `*_test.go`).
