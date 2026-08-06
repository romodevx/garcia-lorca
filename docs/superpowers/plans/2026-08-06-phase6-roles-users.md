# Phase 6 — Roles & Users (auth expansion) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]` checkboxes.

**Goal:** Turn the single seeded owner into real multi-user identity: an admin can **create users**, **create/edit roles** (bundles of `resource:action` permissions), **assign roles to users**, suspend/reactivate accounts, and **link a `config.teachers` record to an iam user** (the reserved `teachers.user_id`). This is the admin CRUD iam deliberately left un-frozen in M01, plus the teacher↔user link that later unlocks per-teacher scoping.

**Architecture:** EXTEND the existing `iam` service (Clean Arch). The permission-based authz engine already exists (Phase 1: `RequirePermission` reads the principal's effective permissions from the JWT). Phase 6 adds the **admin surface** (user/role CRUD RPCs) + gateway routes + web admin, and one small `config` addition (`SetTeacherUser`). New users get a temp password + `must_change_password=true` (reuse Phase 1's forced-change flow). Same `pkg/authctx` RLS.

**Tech Stack:** Go 1.24, GORM (adapters only), Connect/gRPC, bcrypt, Postgres 16 + RLS, buf, Next.js, testcontainers-go.

**Design source:** MODULES.md M01 (area 11), the frozen-contract note in `iam.proto` ("Admin CRUD … added when the admin module needs it").

---

## Decisions taken (open items resolved for this phase)
- **Permission taxonomy** (formalize the `resource:action` set already in use; the admin manages roles from this catalog): `users:read`, `users:write`, `roles:read`, `roles:write` (new) + the existing `students:create/read/list/update/enroll`, `config:read/write`, `grades:read/write`. Documented in `FLAGS.md`/a short `docs/` note; the role editor offers this catalog.
- **New-user password:** `CreateUser` takes an initial **temporary password** (admin-provided) and sets `must_change_password=true`; the user rotates it at first login via the Phase-1 flow. (No email delivery — same posture as the owner seed.)
- **Teacher↔user link mechanism:** **gateway-orchestrated** — the gateway validates the iam user exists (iam `GetUser`) then calls a new `config.SetTeacherUser(teacherId, userId)` (config just stores it; no new config→iam seam). `userId=""` unlinks.
- **Grade-capture teacher-scoping is DEFERRED** — Phase 6 delivers the *capability* (users, roles, teacher↔user link). Enforcing "the caller must be the teacher who owns the assignment" waits until real teacher login exists (mobile/teacher app, Phase 9). Noted so it isn't silently assumed done.
- **Admin routes are flag-gated behind `module_iam`** (a user-facing admin surface); login/refresh/me/change-password/logout stay ungated (auth must always work). Owner needs `module_iam` ON to manage users.

---

## Delivery model
Integration branch `feature/phase6-roles-users` (off `dev`, created; the roadmap doc lives here too). Three sub-phases, each its own branch, merged `--no-ff`, checkpoint each:

| Sub-phase | Branch | Content |
|-----------|--------|---------|
| **6.1 Backend** | `feature/phase6-roles-users-be` | iam admin RPCs (user/role CRUD) — proto + domain + ports + usecase + adapter (RLS IT) + handler; **config** `SetTeacherUser` RPC + migration note (column exists). |
| **6.2 Gateway** | `feature/phase6-roles-users-gw` | `/api/v1/users`, `/api/v1/roles`, teacher-link route behind `module_iam` + `users:*`/`roles:*`; owner permission migration. |
| **6.3 Web** | `feature/phase6-roles-users-web` | Users & roles admin screens (create user, edit role permissions, assign roles, link teacher↔user); flag-gated; Cmd+K. |

**Per-sub-phase DoD:** gofmt/vet clean; unit + integration (RLS isolation) green; buf lint + breaking vs `dev` (additive); flag-ON/OFF; no secret literals (test fixtures neutral; new-user temp passwords in tests use neutral literals); tech-lead (+ security) gates. `gen/`/`go.work.sum` never staged. buf via Docker; base `dev`. Keep `go 1.24`.

---

# Sub-phase 6.1 — Backend

Branch: `git checkout -b feature/phase6-roles-users-be feature/phase6-roles-users`.

## Task 6.1.1: Proto (additive) — iam admin surface + config SetTeacherUser
- [ ] `iam.proto` `IamService`: add `CreateUser`, `GetUser`, `ListUsers`, `SetUserStatus` (suspend/reactivate), `CreateRole`, `GetRole`, `ListRoles`, `SetRolePermissions`. (Keep the existing `AssignRole`/`RevokeRole`.) Messages: `CreateUserRequest {email, display_name, temp_password, repeated string role_ids}` → returns `User`; `ListUsersResponse {repeated User}`; `SetUserStatusRequest {user_id, UserStatus status}`; `CreateRoleRequest {name, repeated string permissions}` → `Role`; `SetRolePermissionsRequest {role_id, repeated string permissions}`; `ListRolesResponse {repeated Role}`. Extend `User` with `repeated string role_ids` or a `ListUserRoles` — simplest: `GetUser`/`ListUsers` return the user + its role ids (add `repeated string role_ids = 9` to `User`).
- [ ] `config.proto` `ConfigService`: add `SetTeacherUser(SetTeacherUserRequest {teacher_id, user_id}) → Teacher` (empty `user_id` unlinks).
- [ ] `buf generate` + `lint` + `breaking --against '.git#branch=dev'` (additive → clean) + `cd gen/go && go build ./...`. Commit: `feat(iam,config): proto — user/role admin + SetTeacherUser`.

## Task 6.1.2: iam domain + ports + usecase (admin)
- [ ] Domain: reuse `User`/`Role`; add a `Role` constructor/validation if missing (name non-empty, permissions are well-formed `resource:action`); a permission-format validator (`^[a-z_]+:[a-z_]+$`).
- [ ] Ports (`UserRepository`/`RoleRepository`): add `CreateUser(ctx, user, passwordHash)`, `ListUsers(ctx)`, `SetStatus(ctx, userID, status)`, `SetUserRoles(ctx, userID, roleIDs)`; `CreateRole(ctx, role)`, `ListRoles(ctx)`, `SetPermissions(ctx, roleID, perms)`. Unique email/name → `KindConflict`.
- [ ] Usecase: an `AdminService` (or extend `AuthService`): `CreateUser(email, displayName, tempPassword, roleIDs)` — validate email, **hash the temp password (bcrypt), `must_change_password=true`**, persist + assign roles; `ListUsers`; `SuspendUser`/`ReactivateUser`; `CreateRole(name, perms)` (validate perms); `ListRoles`; `SetRolePermissions`. Conflict on dup email/role-name.
- [ ] TDD (fakes): create user hashes the temp password + sets must-change + assigns roles; dup email → Conflict; invalid permission string → Invalid; create/list/edit role; suspend blocks login (existing `CanAuthenticate` already enforces status). Neutral literals.
- [ ] Commit: `feat(iam): user/role admin use cases`.

## Task 6.1.3: iam adapter + RLS IT + handler
- [ ] Postgres: implement the new repo methods inside `withTenant`/authctx (`CreateUser` + `user_roles` inserts; `ListUsers` with roles resolved, no N+1; `SetStatus`; role CRUD). Unique violations → Conflict.
- [ ] Integration: create-user round-trip (+ roles), list, suspend, role CRUD; RLS isolation still holds on users/roles/user_roles (already covered — extend if needed). 
- [ ] gRPC handler: the 8 new RPCs (reuse `connectErr`); map `role_ids`. Handler tests.
- [ ] `cd services/iam && go test ./... && go test -count=1 -tags=integration ./internal/adapters/postgres/ && go build ./...`. Commit: `feat(iam): user/role admin adapter + gRPC handlers`.

## Task 6.1.4: config SetTeacherUser
- [ ] config: add `SetTeacherUser(teacherID, userID)` to the usecase + repo (`UPDATE teachers SET user_id=? WHERE id=?`; empty → NULL) + handler. Integration test: set + unset; RLS-scoped. (No validation of the user here — the gateway validates via iam.)
- [ ] `cd services/config && go test ./... && go test -count=1 -tags=integration ./internal/adapters/postgres/`. Commit: `feat(config): SetTeacherUser (link/unlink teacher ↔ iam user)`.

**Checkpoint 6.1** — merge `feature/phase6-roles-users-be` → `feature/phase6-roles-users`.

---

# Sub-phase 6.2 — Gateway

- [ ] **6.2.1 clients** — extend the gateway iam auth client with the admin methods; extend the config client with `SetTeacherUser`.
- [ ] **6.2.2 handlers** — REST: `POST/GET /api/v1/users`, `GET /api/v1/users/{id}`, `POST /api/v1/users/{id}/status`, `POST /api/v1/users/{id}/roles`; `POST/GET /api/v1/roles`, `POST /api/v1/roles/{id}/permissions`; `POST /api/v1/config/teachers/{id}/user` (link) — the handler validates the user exists via iam `GetUser`, then calls config `SetTeacherUser`.
- [ ] **6.2.3 routes** — a `iamAdminWrite`/`iamAdminRead` pair = `guard.Protect("iam", requireAuthChanged(RequirePermission("users:write"|"users:read"|"roles:write"|"roles:read")))`. Wire per route. The teacher-link route sits under `guard.Protect("config", …RequirePermission("config:write")…)` (it mutates a config teacher). Update `newMux` + `main_test.go` route-security tests.
- [ ] **6.2.4 permissions** — `migrations/iam/0006_admin_permissions.sql`: grant `users:read`,`users:write`,`roles:read`,`roles:write` to the owner (idempotent, `SET LOCAL app.current_tenant`). Add `iam` to the gateway `ENABLED_MODULES` if the admin surface should be reachable locally.
- [ ] Verify: gateway `go test ./...`; E2E — owner creates a role, creates a user with that role + a temp password, the new user logs in (forced change), then GETs `/me` and sees the role's permissions; owner links a teacher to a user; module `iam` OFF → admin routes 404. Commit.

**Checkpoint 6.2** — merge → `feature/phase6-roles-users`.

---

# Sub-phase 6.3 — Web
- [ ] **6.3.1 api** — `apps/web/app/admin/api.ts` (+ test): users + roles CRUD, assign roles, set status, link teacher↔user.
- [ ] **6.3.2 screens** — an **Admin** surface (flag `module_iam`): a Users list + "create user" (email, display name, temp password, roles); a Roles editor (name + permission checklist from the taxonomy catalog); assign-roles-to-user; a "link teacher to user" control on the teachers surface (or in admin). Fewest-clicks; a11y; `role="alert"`.
- [ ] **6.3.3 flag + palette + nav** — behind `useFlag("module_iam")`; Cmd+K "create user"/"create role". i18n keys. Registry ON/OFF test.
- [ ] Verify: `pnpm --filter @garcia-lorca/web test && tsc --noEmit && lint`. Commit.

**Checkpoint 6.3** — merge → `feature/phase6-roles-users`.

---

## Final gates (after 6.1–6.3 merged)
- Full suite (Go race + iam/config integration incl. RLS isolation; web) green.
- buf lint + breaking vs `dev` (additive).
- **Security gate (important — this is the authz control plane):** creating a user can't self-escalate (only an admin with `users:write` may create/assign roles; a user can't grant themselves permissions they lack — decide whether to enforce "can't assign a permission you don't hold" now or note it); temp passwords never logged; suspended users can't authenticate; RLS on users/roles/user_roles; permission strings validated; flag boundary + `users:*`/`roles:*`; no secret literals.
- Tech-lead gate. Update MODULES.md (M01 note: admin CRUD shipped) + FLAGS.md (permission catalog). Hand `feature/phase6-roles-users` to the user for review (do NOT merge to `dev`).

## Out of scope (later)
Teacher-scoped grade capture enforcement (needs teacher login — Phase 9/mobile); email/SSO delivery of invites; fine-grained per-record ownership (RLS row-owner policies via `app.current_user`); self-service password reset. The permission taxonomy is extensible per later module.

## Testing strategy
Unit (user/role admin, permission-format validation, temp-password hashing + must-change), integration (RLS isolation on users/roles/user_roles + config teachers.user_id set/unset, create-user round-trip), contract (proto additive), web (Vitest/RTL), flag-ON/OFF. Deterministic. Neutral literals; test temp passwords use neutral values (`.gitguardian.yaml` excludes test files).
