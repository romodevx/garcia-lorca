# García Lorca — Sistema de Gestión Escolar (monorepo)

Monorepo: Go (gRPC/Connect + GORM) backend · Next.js + React Native/Expo frontend ·
GCP Cloud Run + Cloud SQL (Postgres) + Pub/Sub · contratos en `proto/` vía buf.

> Las reglas globales del equipo (`~/.claude/CLAUDE.md` y `~/.claude/rules/`) aplican
> siempre. Esto es el contexto **específico del repo**.

## Reglas no negociables (de PLAN-sprint-01.md)

1. **TDD estricto.** Nunca código de producción sin un test que falle primero
   (Red → Green → Refactor). Sin excepción.
2. **Flujo por módulo (orden fijo):** backend (con tests) → frontend (con tests) →
   feature flag. No se saltan pasos.
3. **Feature flags:** todo módulo nace detrás de un flag `module_<name>` en **default OFF**.
   El flag se valida en el **backend** (módulo OFF ⇒ endpoints responden NotFound/PermissionDenied),
   no solo en el front.
4. **GORM solo en `adapters/postgres`.** El dominio (`internal/domain/`) jamás importa GORM
   ni ningún framework. Doble modelo + `mapper.go`.
5. **RLS en tablas sensibles** (`students`, `grades`, `attendance`, `finance_accounts`,
   `invoices`). Siempre `SET LOCAL` (`set_config(..., true)`) dentro de transacción
   vía el helper `pkg/authctx` (`WithTenantContext`, enlaza `app.current_tenant`).
   Las policies RLS son SQL puro en `migrations/` y filtran por
   `current_setting('app.current_tenant')`.
6. **Front blindado:** build de producción sin source maps. CI falla si aparecen `*.map`
   o `sourceMappingURL` en artefactos de cliente.
7. **Gestor de paquetes = `pnpm` (ÚNICO).** `npm` y `yarn` están **prohibidos** en este
   proyecto. Las dependencias se instalan solo con `pnpm` (`pnpm install`, `pnpm add`,
   `pnpm --filter <pkg>`). No se commitea `package-lock.json` ni `yarn.lock`; la única
   lockfile es `pnpm-lock.yaml`. Activar pnpm con `corepack enable pnpm`. Un guard
   `preinstall` (`only-allow pnpm`) bloquea instalaciones con npm/yarn.

## Regla de oro del monorepo

**Nadie edita `gen/`.** Cambias un `.proto` → `buf generate` → Go y TS reciben los tipos.
`gen/` es 100% generado y está en `.gitignore` (se regenera en CI).

## Convenciones compartidas (todas las toolchains dependen de esto)

- **Módulo base Go:** `github.com/garcia-lorca`
  - libs compartidas → `github.com/garcia-lorca/pkg/<lib>`
  - código generado → `github.com/garcia-lorca/gen/go`
  - cada servicio → `github.com/garcia-lorca/services/<name>` (módulo propio, en `go.work`)
- **Scope npm:** `@garcia-lorca/<package>` (workspaces de **pnpm** vía `pnpm-workspace.yaml` + Turborepo)
- **Paquetes proto:** `garcialorca.<domain>.v1` en `proto/garcialorca/<domain>/v1/<domain>.proto`
- **Flag keys:** snake_case `module_<name>`. Registro en `FLAGS.md` + migración.
- **Commits:** Conventional Commits. Ramas `feature/ bugfix/ hotfix/ release/`.
- Código y comentarios en inglés.

## Arquitectura por servicio

- **Clean Architecture completa** (dominio rico): `finance`, `billing`, `grades`,
  `scholarships`, `students`, `iam`, **`uniforms`**.
  `cmd/main.go` + `internal/{domain,usecase,ports,adapters/{postgres,grpc,pubsub}}`.
  Dependencias: `adapters → usecase → domain`.
- **Layout thin** (CRUD + I/O): `attendance`, `docs`, `comms`.
  `handler gRPC → service → repo`. La arquitectura se gana, no se impone.
- `uniforms` nació clasificado como thin en FASE 0 y **se movió** al construirse:
  máquina de estados de 7 estados, invariantes de inventario que cruzan tablas,
  aritmética de días hábiles y asignación FIFO no son CRUD (**ADR 0007**). La
  clasificación de FASE 0 es una hipótesis, no un contrato.
- **gateway/BFF:** edge Connect (HTTP/JSON) hacia front; gRPC interno hacia servicios;
  middleware de feature flags; read models para dashboard.

## Cómputo / costo (Cloud Run)

- `gateway` e `iam`: `min-instances=1` (siempre calientes). Todo lo demás: scale-to-zero.

## Estado del proyecto

> **Convención `.claude/`:** la raíz solo contiene `CLAUDE.md` y `README.md` como `.md`.
> Memoria, documentos y tareas del proyecto viven en `.claude/` (ver `.claude/README.md`).

- **Backlog:** `.claude/docs/MODULES.md` · **Flags:** `.claude/docs/FLAGS.md`
  · **Plan:** `.claude/docs/PLAN-sprint-01.md`
- **Memoria del proyecto:** `.claude/memory/` (índice en `MEMORY.md`)
- **ADRs:** `.claude/docs/adr/` · **Diseño:** `.claude/docs/design/`
- **Fuera del MVP (future-ready):** `billing` (CFDI/IEDU) — solo proto stub + flag OFF +
  frontera reservada, sin implementar.

## Comandos

- `buf generate` — regenera `gen/` desde `proto/`
- `buf lint` / `buf breaking` — valida contratos
- `go work sync` · `go test ./...`
- `pnpm install` (NUNCA `npm install`) · `pnpm run build` / `pnpm turbo run build|test|lint`
