# iam — Identity & Access (M01)

> Per-service context. Global team rules (`~/.claude/`) and the repo root
> `CLAUDE.md` always apply. This file is the template the other services copy.

## Purpose
Authentication (JWT issuance/validation), and the role + permission model every
other service authorizes against. iam is the spine — gate it carefully; nothing
downstream works without it. Always-warm on Cloud Run (`min-instances=1`).

## Architecture — Clean (rich domain)
`cmd/main.go` + `internal/{domain,usecase,ports,adapters/{postgres,grpc,pubsub}}`.
Dependency direction: `adapters → usecase → domain`. The domain imports no
framework (no GORM, no Connect). GORM lives ONLY in `adapters/postgres`, mapping
domain entities ↔ GORM models (double model + `mapper.go`).

## Domain invariants
- A `User` has a stable identity and exactly one credential subject; email is
  unique per tenant.
- Authorization is permission-based: roles are bundles of permissions; checks
  resolve a user's effective permission set, never bare role-name comparisons.
- A `Role` cannot be deleted while assigned (`user_roles` references it).
- Tokens: short-lived access + verifiable claims; iam is the only issuer.

## Events
- Published: none (M01 is foundational; downstream services authorize against it
  synchronously rather than via events).

## RLS-sensitive tables
`users`, `roles`, `user_roles` — `ENABLE + FORCE ROW LEVEL SECURITY` from the
first migration. All access goes through `pkg/authctx.WithUserContext`, which
sets `app.current_user` via `SET LOCAL` inside the transaction.

## Feature flag
`module_iam` — default OFF. Disabled ⇒ gateway returns 404 for iam routes.

## FASE 0 status
Health/`Ping` is hand-written JSON in `adapters/grpc` to validate the pipeline
before `buf generate`. Replace with the generated `garcialorca.iam.v1` Connect
handler once contracts exist; the `usecase.HealthService` seam stays unchanged.
