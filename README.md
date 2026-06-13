# García Lorca — Sistema de Gestión Escolar

Plataforma de gestión escolar (11 áreas) para web (Next.js), móvil (React Native/Expo)
y backend de microservicios en Go sobre GCP.

## Stack

| Capa | Tecnología |
|---|---|
| Backend | Go · gRPC/Connect · GORM (solo en adapters) |
| Contratos | Protocol Buffers gestionados con **buf** (`proto/` = fuente de verdad) |
| Base de datos | Cloud SQL (PostgreSQL) con Row-Level Security |
| Mensajería | Cloud Pub/Sub |
| Frontend web | Next.js (App Router) |
| Frontend móvil | React Native (Expo) |
| Cómputo | Cloud Run (gateway + iam siempre calientes; resto scale-to-zero) |
| Monorepo | Turborepo (TS) + Go workspaces (`go.work`) |

## Estructura

```
proto/        Contratos (fuente de verdad). buf genera gen/.
gen/          Código generado Go + TS — NO se edita a mano (gitignored).
services/     Microservicios Go (clean arch donde aplica, thin donde no).
apps/         Frontends: web (Next.js) y mobile (Expo).
packages/     TS compartido: api-client, ui, domain, config.
pkg/          Libs Go compartidas: logger, errors, validator, authctx (RLS), pubsub, pdf.
deploy/       Terraform (GCP) + Cloud Build (CI/CD).
migrations/   SQL versionado por dominio (incluye policies RLS).
docs/adr/     Architecture Decision Records.
```

## Prerrequisitos

- Go ≥ 1.22 · Node ≥ 20 · `buf` · Docker · `gcloud` + Terraform (deploy)

## Setup

```sh
buf generate          # genera gen/ desde proto/
go work sync          # sincroniza workspace Go
corepack enable pnpm  # activa pnpm (gestor de paquetes ÚNICO; npm/yarn prohibidos)
pnpm install          # instala workspaces TS
```

## Reglas de desarrollo

Ver `CLAUDE.md` (TDD estricto, flujo por módulo, feature flags default-OFF,
GORM solo en adapters, RLS, front sin source maps).

## Documentos de proyecto

Memoria, documentos y tareas viven en [`.claude/`](./.claude/README.md)
(la raíz mantiene solo `CLAUDE.md` y `README.md`).

- Backlog: [`.claude/docs/MODULES.md`](./.claude/docs/MODULES.md)
- Feature flags: [`.claude/docs/FLAGS.md`](./.claude/docs/FLAGS.md)
- Plan de sprint: [`.claude/docs/PLAN-sprint-01.md`](./.claude/docs/PLAN-sprint-01.md)
- Referencia de diseño: [`.claude/docs/design/`](./.claude/docs/design/)
