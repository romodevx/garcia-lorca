# ADR 0001 — iam RLS is tenant-scoped, not per-user

- Status: Accepted
- Date: 2026-07-01
- Context: M01 (iam), the auth backbone. Tables `users`, `roles`, `user_roles`.

## Decision

Row Level Security on the iam tables is **tenant-scoped**: policies filter on
`tenant_id = current_setting('app.current_tenant')::uuid`, with `ENABLE + FORCE
ROW LEVEL SECURITY`. The iam postgres adapter binds `app.current_tenant` per
transaction (`set_config(..., true)` = `SET LOCAL`), the same pattern
`pkg/authctx` uses for `app.current_user`.

## Why not per-user (`app.current_user`)

The repo-wide RLS note says user/tenant tables scope on `app.current_user`. That
is correct for downstream tables a user reads about *themselves* (students,
grades). It does **not** work for the iam auth backbone:

- **Login is a chicken-and-egg.** `Authenticate` looks up a user *by email*
  BEFORE any user is authenticated — there is no `app.current_user` yet. A policy
  `id = current_setting('app.current_user')` would make the login lookup return
  zero rows, so no one could ever log in. The tenant, by contrast, IS known at
  login time (it is not a secret), so it can be set before the email lookup.
- **Per-user visibility is application authorization, not RLS.** "A user may only
  see their own record / only an admin may list users" is enforced by the
  permission checks in the use case (`CheckPermission`, effective permissions).
  RLS is defense-in-depth *below* that, and the threat it defends here is
  cross-tenant leakage (tenant A's admin must never read tenant B's users).

## Consequences

- `app.current_tenant` is the RLS identity for iam tables. The adapter sets it in
  every transaction; unset ⇒ `current_setting(..., true)` is NULL ⇒ zero rows
  (fail-closed).
- The MVP is effectively single-tenant (one school); `tenant_id` is real but
  constant today. RLS is implemented properly anyway (mandated from the first
  migration, and future-ready for multi-tenant).
- The isolation test proves an app-role connection scoped to tenant A cannot read
  tenant B's rows, with `FORCE` applying RLS even to the table owner.
- `pkg/authctx` (per-`app.current_user`) is unchanged; it remains the primitive
  for downstream per-student/-tenant tables. iam adds its own tenant binding
  rather than widening the shared helper (smaller blast radius for M01).
