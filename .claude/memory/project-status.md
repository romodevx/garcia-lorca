# Estado del proyecto — progreso real (snapshot 2026-07-02)

Foto del avance para retomar sin releer todo el git log. Fuente de verdad del backlog:
`.claude/docs/MODULES.md` · flags: `.claude/docs/FLAGS.md` · plan: `.claude/docs/PLAN-sprint-01.md`.

> Convención: cada módulo sigue backend (TDD) → frontend → flag. Spine primero
> (`iam → gateway → students`), desplegado end-to-end antes de ensanchar. Gotchas en [[build-lessons]].

## Fase 0 — Cimientos ✅
Monorepo scaffolding, `buf` + `gen/`, `go.work`, pnpm + Turborepo, `.claude/` workspace.
Docker + Make local stack (`make run` / `run-new` / `stop` / `restart` / `logs` / `psql`).
Diseños importados: web (admin+maestros) y mobile (padres, Expo/RN). Ver [[web-frontend-design]], [[mobile-app-design]].

## Spine (vertical slice)

### M01 — IAM ✅ COMPLETO (`module_iam`)
Clean Arch completa. Domain (auth/authz TDD) → usecase `AuthService`
(login/refresh/check-permission/assign-role) → adapters: **bcrypt** hasher,
**JWT HS256** (con `WithValidMethods` anti alg-confusion) → **postgres + RLS**
(migraciones SQL puras, testcontainers isolation como rol `app` no-owner) →
**gRPC/Connect** generado, end-to-end. Login/ValidateToken probados sobre Postgres real.
RLS es **tenant-scoped** (no per-user) — ver `ADR 0001`, porque login lee por email antes de autenticar.

### M02 — Gateway / BFF ✅ COMPLETO (`module_gateway`)
Edge Connect (HTTP/JSON) → gRPC interno. Implementado: **login proxy** + `RequireAuth`
contra iam real, **authz** (permission gate + refresh proxy), **middleware de feature flags**
(módulo OFF → 404, frontera de seguridad), y **`GET /api/v1/features`** como fuente
autoritativa de flags (config-driven vía `ENABLED_MODULES`). El web ya consume `/features`
(dropeado el DEMO hardcoded). `gateway` e `iam` = `min-instances=1` (calientes).

### M03 — Students / Admisión 🚧 EN PROGRESO (`module_students`)
Hecho: **proto congelado**, **domain** (máquina de estados de admisión, CURP, nivel — TDD),
**usecase** `StudentService` (crear aspirante + transiciones de admisión), **postgres adapter
+ RLS migration + testcontainers isolation**.
**Falta (siguiente tarea):**
- `internal/adapters/grpc/` — handler gRPC (hoy solo `doc.go` placeholder)
- `internal/adapters/pubsub/` — emitir `StudentEnrolled` / `AdmissionStatusChanged` (hoy solo `doc.go`)
- `cmd/main.go` — cablear todo (hoy esqueleto)
- **gateway**: ruta `POST /api/v1/students` detrás de flag + auth + permiso
- Al cerrar esto se **enciende `preset_spine`** (iam+gateway+students end-to-end).

## Módulos posteriores — NO iniciados
M04 config, M05 finance, M06 attendance, M07 grades (Must) · M08 becas, M09 docs, M10 comms (Should)
· M12 uniforms, M13 reports (Could). Todos con servicio+proto scaffold y flag registrado (default OFF).
**M11 billing (CFDI/IEDU) = fuera de MVP**: solo proto stub congelado + flag OFF + frontera reservada.

## Cross-cutting entregado
- **`@garcia-lorca/i18n`** ✅ — catálogos ICU planos es-MX(default)+en, binding web (next-intl) y
  mobile (react-i18next+i18next-icu). Ver [[i18n-bilingual]].
- **Command Palette CP-1** ✅ — shell `cmdk` + Layer A (navegación, mín. clics). Contrato: cada
  módulo registra sus comandos. Ver [[command-palette]], [[ux-minimum-clicks]].
- **Docker + Make** ✅ — stack local completo (backend + web) contenerizado.
- **Logo real del colegio** ✅ — web + mobile + favicon.
- **Documentos (M09) desactivado del panel** ✅ — `hidden`, no borrado. Ver [[module-docs-deactivated]].

## Próximo paso al retomar
Terminar M03 (grpc handler + pubsub emit + cmd wiring + ruta gateway) → validar `preset_spine`
end-to-end → seguir con M04/M05. Mantener TDD estricto (test rojo primero) y el flujo de PR
(worktree → verde → PR → GitGuardian → squash-merge). Ver [[build-lessons]].
