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

### M03 — Students / Admisión ✅ COMPLETO (`module_students`) — rama `feature/m03-students-grpc-pubsub`
**Ambos gates pasados:** tech-lead **PASS** (canon, sin bloqueantes) · security **APPROVED**
(flag cleared para entornos compartidos). Cobertura de negocio ≥80% (protoconv 100%, grpc 86.5%,
handler gateway 92.1%, middleware 100%, domain 88.5%, usecase 87.5%). Fixes de gate aplicados:
`protoconv_test.go` + `students_map_test.go` (table-driven), wrap `%w` en publisher, `errors.Is`
en gateway main, y test de regresión no-fuga (error interno→mensaje genérico).
Hecho: **proto congelado**, **domain** (máquina de admisión, CURP, nivel — TDD),
**usecase** `StudentService` + `HealthService`, **postgres adapter + RLS + testcontainers**.
Añadido en esta sesión (TDD estricto, todo verde + gofmt/vet limpios):
- **`internal/adapters/grpc/students_handler.go`** — implementa el `StudentsServiceHandler` generado;
  mapeo de errores **por-RPC** (transición ilegal `KindConflict`→`FailedPrecondition` por contrato
  congelado; CURP duplicado→`AlreadyExists`). 8 tests.
- **`internal/adapters/pubsub/publisher.go`** — `EventPublisher` → topic **`student-events`** (uno por
  dominio, estilo `payment-events`), payload proto-marshaled, attrs `type`/`tenant_id`. 2 tests.
- **`internal/adapters/protoconv/`** — mappers Level/AdmissionStatus dominio↔proto compartidos por
  ambos adapters (DRY; el dominio no importa proto).
- **`cmd/main.go`** — composition root (repo pg + eventos + StudentService + Connect handler, h2c).
- **gateway**: `StudentsClient` (Connect), `StudentsHandler` (JSON, 4 tests), ruta
  **`POST /api/v1/students`** detrás de `ModuleGuard(students)` → `RequireAuth` →
  `RequirePermission(students:create)`; `httpStatusFor` extendido (409 para AlreadyExists/FailedPrecondition).
- **Test de integración `newMux`** ✅ (`services/gateway/cmd/main_test.go`) — monta stubs Connect de iam +
  students y ejercita la cadena de seguridad completa: flag OFF→404, sin token→401, sin permiso→403,
  con `students:create`→201. Blinda el orden guard→auth→permiso.
- **Docs** ✅ — `services/students/CLAUDE.md` (propósito, arquitectura, invariantes, eventos, RLS, flag, palette).
- **Command Palette** ✅ — students ya registrado en `registry.ts` (nav + `create.student`, gated por flag).

**Diferido explícitamente (NO bloquea M03):**
- **Transporte Cloud Pub/Sub** real → a **M10 (comms)**, cuando exista consumidor (así lo indica el TODO de
  `pkg/pubsub`). Hoy `devBus` loguea solo metadatos; el port `EventPublisher` no cambia cuando aterrice.

**Fixes aplicados en `/review` (revisión de 4 agentes: testing, maintainability, security, adversarial):**
`MaxBytesReader` en POST students + `/auth/*`; timeouts (ReadHeader/Read/Write/Idle) en ambos
`http.Server`; 4 aserciones de test añadidas (mapeo de filtro+NextPageToken, ChangeAdmission not-found,
Reason round-trip, default→500); comentario de invariante single-tenant en el composition root.

**BLOQUEANTES antes de encender `module_students` con tráfico real** (hallazgos adversariales,
en código pre-existente `usecase/student.go` + `postgres/repository.go`, NO en este PR; el flag OFF los
hace no explotables hoy):
- **P1 — race read-modify-write en la máquina de admisión**: `FindByID` + `Update` en transacciones
  separadas sin lock ni CAS; `Update` es overwrite `WHERE id=?`. Dos transiciones concurrentes → lost
  update + evento espurio (`StudentEnrolled` de un alumno que la DB marca `withdrawn`). **Fix:** CAS
  `Update(...).Where("id=? AND admission_status=?", id, previous)`, `RowsAffected==0` → `KindConflict`.
- **P2 — dual-write sin outbox**: `repo.Update` commitea y luego `pub.Publish*`; si el publish falla el
  evento se pierde (hoy `devBus` lo enmascara; muerde con el transporte real Cloud Pub/Sub de M10).
  **Fix:** outbox transaccional o transiciones idempotentes. Va junto con el transporte Cloud Pub/Sub (M10).

**Otras condiciones del gate de security (CI):** tests de aislamiento RLS verdes (testcontainers) +
`govulncheck ./...`. **Follow-up cross-cutting:** interceptor de logging de errores internos (iam/gateway/
students no lo tienen). Encender vía `ENABLED_MODULES` (default OFF) → habilita `preset_spine`.

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
