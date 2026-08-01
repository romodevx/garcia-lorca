# Phase 1 — Real Login + Owner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The director/owner logs into García Lorca with real credentials seeded by a migration, is forced to change the temporary password on first login, and lands authenticated in the web app — replacing today's fake role-picker.

**Architecture:** iam issues JWTs (unchanged). A migration seeds the owner user (bcrypt temp password + `must_change_password=true`) and an owner role. `Login` reports `must_change_password`; a new `ChangePassword` RPC clears it. The gateway proxies both, sets an HttpOnly auth cookie, and the web app gains a real login + change-password flow. A one-shot compose `migrate` service applies `migrations/**/*.sql` at local dev startup (a gap today).

**Tech Stack:** Go 1.24 (Clean Arch: domain/usecase/ports/adapters), GORM (adapters only), Connect/gRPC, HMAC JWT, bcrypt, Postgres 16 + RLS, buf, Next.js (App Router SPA), pnpm/Turborepo, testcontainers-go.

---

## Prerequisites & conventions

- Branch: `feature/academic-spine` (already created; the design spec is committed there).
- Run Go unit tests from a service module dir, e.g. `cd services/iam && go test ./...`.
- Integration tests need Docker: `go test -tags=integration ./internal/adapters/postgres/`.
- **Never edit `gen/`.** After any `.proto` change: `make gen` (or `buf generate`), then `buf lint` and `buf breaking` against `main`.
- Default tenant (single-tenant M01): `00000000-0000-0000-0000-000000000001`.
- Chosen temp password for the owner (local/dev): `GarciaLorca2026!` — the owner MUST change it at first login.
- Owner email (local/dev): `director@garcialorca.edu.mx`.

---

## File structure (created/modified in this phase)

**Infra / migrations**
- Create: `migrations/iam/0002_owner_and_password_policy.sql` — `must_change_password` column + owner role + owner user seed.
- Modify: `docker-compose.yml` — add one-shot `migrate` service; services depend on it.
- Create: `docker/apply-migrations.sh` — ordered psql applier run by the `migrate` service.
- Modify: `Makefile` — add `migrate` target.

**proto**
- Modify: `proto/garcialorca/iam/v1/iam.proto` — add `User.must_change_password`, `LoginResponse.must_change_password`, and the `ChangePassword` RPC + messages.

**iam service**
- Modify: `services/iam/internal/domain/user.go` — carry `mustChangePassword`.
- Modify: `services/iam/internal/ports/ports.go` — `UpdatePassword`, and `FindAuthByEmail` returns the flag.
- Modify: `services/iam/internal/usecase/auth.go` — `AuthResult.MustChangePassword`; new `ChangePassword`.
- Create: `services/iam/internal/usecase/auth_change_password_test.go` — unit tests for `ChangePassword`.
- Modify: `services/iam/internal/adapters/postgres/models.go` — `MustChangePassword` column.
- Modify: `services/iam/internal/adapters/postgres/mapper.go` — map the flag.
- Modify: `services/iam/internal/adapters/postgres/repository.go` — return flag; implement `UpdatePassword`.
- Modify: `services/iam/internal/adapters/postgres/repository_integration_test.go` — apply both migrations; assert seed + password update.
- Modify: `services/iam/internal/adapters/grpc/iam_handler.go` — map flag into `LoginResponse`; implement `ChangePassword`.
- Modify: `services/iam/internal/adapters/grpc/*_test.go` (handler tests) — cover `ChangePassword` mapping.

**gateway service**
- Modify: `services/gateway/internal/client/iamauth.go` — `LoginResult.MustChangePassword`; `ChangePassword` client method.
- Modify: `services/gateway/internal/handler/auth.go` — expose `mustChangePassword`; set HttpOnly cookie on login; `ChangePassword` handler.
- Modify: `services/gateway/internal/handler/auth_test.go` — cover the new field + cookie.
- Modify: `services/gateway/internal/middleware/auth.go` — accept the auth cookie as a token source (in addition to `Authorization: Bearer`).
- Modify: `services/gateway/cmd/main.go` — route `POST /api/v1/auth/change-password`.

**web app**
- Create: `apps/web/app/auth/api.ts` — `login()` / `changePassword()` fetch helpers.
- Create: `apps/web/app/auth/api.test.ts` — unit tests (mocked fetch).
- Modify: `apps/web/app/_screens/Login.tsx` — real credential form (email + password), calls `login()`.
- Create: `apps/web/app/_screens/ChangePassword.tsx` — forced change-password screen.
- Modify: `apps/web/app/AppShell.tsx` — drive `login → change-password (if required) → dashboard`.

---

## Task 0: Local migration runner (enabler)

**Why:** `make run` only creates the `app` role (`docker/postgres-init.sh`); nothing applies `migrations/**/*.sql`, so the tables the services need never exist at dev runtime. This task closes that gap so the owner seed (Task 1) can run.

**Files:**
- Create: `docker/apply-migrations.sh`
- Modify: `docker-compose.yml`
- Modify: `Makefile:105` (near the `psql` target)

- [ ] **Step 1: Write the migration applier script**

Create `docker/apply-migrations.sh`:

```sh
#!/bin/sh
# Applies every migrations/**/*.sql in lexical order (iam/0001, iam/0002,
# students/0001, …) as the Postgres superuser, so tables are owned by the
# migrator (not the app role). Idempotent SQL is the migration author's job;
# this runner just orders and applies. Used by the compose `migrate` one-shot
# and by `make migrate`.
set -e

: "${PGHOST:=postgres}"
: "${PGUSER:=postgres}"
: "${PGDATABASE:=garcia_lorca}"
export PGPASSWORD="${POSTGRES_PASSWORD:?POSTGRES_PASSWORD must be set}"

echo "[migrate] applying migrations to ${PGDATABASE}@${PGHOST} as ${PGUSER}"
for f in $(find /migrations -name '*.sql' | sort); do
  echo "[migrate] -> $f"
  psql -v ON_ERROR_STOP=1 -h "$PGHOST" -U "$PGUSER" -d "$PGDATABASE" -f "$f"
done
echo "[migrate] done"
```

Make it executable:

```bash
chmod +x docker/apply-migrations.sh
```

- [ ] **Step 2: Add the one-shot `migrate` service to docker-compose**

In `docker-compose.yml`, under `services:` (right after the `pubsub` service, before the Go services), add:

```yaml
  # ---- Migrations (one-shot) ----------------------------------------------
  # Applies migrations/**/*.sql as the superuser once Postgres is healthy, then
  # exits 0. Go services wait for it to complete so tables exist before boot.
  migrate:
    image: postgres:16-bookworm
    entrypoint: ["/bin/sh", "/apply-migrations.sh"]
    environment:
      PGHOST: postgres
      PGUSER: postgres
      PGDATABASE: garcia_lorca
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
    volumes:
      - ./migrations:/migrations:ro
      - ./docker/apply-migrations.sh:/apply-migrations.sh:ro
    depends_on:
      postgres:
        condition: service_healthy
    networks: [gl]
    restart: "no"
```

- [ ] **Step 3: Make Go services wait for migrations to finish**

In `docker-compose.yml`, change the shared `x-go-service` anchor's `depends_on` so every Go service waits for `migrate` to complete:

```yaml
x-go-service: &go-service
  build:
    context: .
    dockerfile: docker/go-dev.Dockerfile
  image: garcia-lorca-go-dev:local
  volumes:
    - .:/workspace
    - go-mod-cache:/go/pkg/mod
    - go-build-cache:/root/.cache/go-build
  depends_on:
    postgres:
      condition: service_healthy
    migrate:
      condition: service_completed_successfully
  networks: [gl]
  restart: unless-stopped
```

Also update the two services that override `depends_on` (they must keep the migrate dependency). For `gateway`:

```yaml
    depends_on:
      postgres:
        condition: service_healthy
      migrate:
        condition: service_completed_successfully
      iam:
        condition: service_started
```

(`iam`, `students`, etc. that use the anchor's default `depends_on` need no change.)

- [ ] **Step 4: Add a `make migrate` target**

In `Makefile`, right after the `psql` target (line ~107), add:

```makefile
.PHONY: migrate
migrate: ## Aplica migrations/**/*.sql en la base local (orden léxico)
	$(COMPOSE) run --rm migrate
```

- [ ] **Step 5: Verify migrations apply against the running stack**

Run:

```bash
make run
docker compose run --rm migrate
docker compose exec -T postgres psql -U app -d garcia_lorca -c "\dt"
```

Expected: the `migrate` run prints `-> /migrations/iam/0001_init.sql` and `-> /migrations/students/0001_init.sql` and `[migrate] done`; `\dt` lists `users`, `roles`, `user_roles`, `students`. (Task 1 adds `0002`.)

- [ ] **Step 6: Commit**

```bash
git add docker/apply-migrations.sh docker-compose.yml Makefile
git commit -m "feat(dev): apply migrations at local startup via one-shot migrate service"
```

---

## Task 1: Owner seed migration + password-policy column

**Files:**
- Create: `migrations/iam/0002_owner_and_password_policy.sql`

- [ ] **Step 1: Generate a bcrypt hash for the temp password**

bcrypt salts randomly, so the hash cannot be hardcoded blindly — generate one now and paste it into the migration in Step 2. Run:

```bash
cd services/iam && go run - <<'EOF'
package main
import ("fmt"; "golang.org/x/crypto/bcrypt")
func main() {
	h, _ := bcrypt.GenerateFromPassword([]byte("GarciaLorca2026!"), bcrypt.DefaultCost)
	fmt.Println(string(h))
}
EOF
```

Expected: a line like `$2a$10$....` (60 chars). Copy it; it replaces `<BCRYPT_HASH>` below.

- [ ] **Step 2: Write the migration**

Create `migrations/iam/0002_owner_and_password_policy.sql`:

```sql
-- iam (M01) — password policy + owner/director seed.
--
-- Adds must_change_password (forces a rotation at first login) and seeds the
-- single owner account for the default tenant. Applied by the migrator
-- (superuser), so RLS does not block these INSERTs. Idempotent: safe to re-run.

BEGIN;

-- --- password policy ------------------------------------------------------

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS must_change_password boolean NOT NULL DEFAULT false;

-- --- owner seed (default tenant, single-tenant M01) -----------------------
-- Fixed UUIDs so re-running is a no-op and later migrations can reference them.

INSERT INTO roles (id, tenant_id, name, permissions)
VALUES (
    '00000000-0000-0000-0000-0000000000a1',
    '00000000-0000-0000-0000-000000000001',
    'owner',
    -- Spine permissions the owner needs today; later phases append their own.
    ARRAY['students:create', 'students:read', 'students:list', 'students:update']
)
ON CONFLICT (tenant_id, name) DO NOTHING;

INSERT INTO users (id, tenant_id, email, display_name, password_hash, status, must_change_password)
VALUES (
    '00000000-0000-0000-0000-0000000000d1',
    '00000000-0000-0000-0000-000000000001',
    'director@garcialorca.edu.mx',
    'Director',
    '<BCRYPT_HASH>',   -- bcrypt of GarciaLorca2026! (from Task 1 Step 1)
    'active',
    true
)
ON CONFLICT (tenant_id, email) DO NOTHING;

INSERT INTO user_roles (user_id, role_id, tenant_id)
VALUES (
    '00000000-0000-0000-0000-0000000000d1',
    '00000000-0000-0000-0000-0000000000a1',
    '00000000-0000-0000-0000-000000000001'
)
ON CONFLICT (user_id, role_id) DO NOTHING;

COMMIT;
```

- [ ] **Step 3: Apply and verify the seed**

Run:

```bash
docker compose run --rm migrate
docker compose exec -T postgres psql -U postgres -d garcia_lorca -c \
  "SELECT email, status, must_change_password FROM users WHERE email='director@garcialorca.edu.mx';"
```

Expected: one row, `status=active`, `must_change_password=t`.

- [ ] **Step 4: Commit**

```bash
git add migrations/iam/0002_owner_and_password_policy.sql
git commit -m "feat(iam): owner seed + must_change_password column"
```

---

## Task 2: Proto — must_change_password + ChangePassword RPC

**Files:**
- Modify: `proto/garcialorca/iam/v1/iam.proto`

- [ ] **Step 1: Add the field to User**

In `proto/garcialorca/iam/v1/iam.proto`, in `message User`, after `updated_at = 7;` add:

```proto
  // must_change_password is true while the account still holds a temporary
  // password; the only allowed action until it clears is ChangePassword.
  bool must_change_password = 8;
```

- [ ] **Step 2: Add the field to LoginResponse**

In `message LoginResponse`, after `User user = 4;` add:

```proto
  // Mirrors User.must_change_password for convenience: the client routes to the
  // change-password flow when true.
  bool must_change_password = 5;
```

- [ ] **Step 3: Add the RPC and its messages**

In `service IamService`, after the `RefreshToken` rpc, add:

```proto
  // ChangePassword sets a new password for the principal, verifying the current
  // one first. Clears must_change_password. Used for the forced first-login
  // rotation and ordinary password changes.
  rpc ChangePassword(ChangePasswordRequest) returns (ChangePasswordResponse);
```

After the `RefreshTokenResponse` message, add:

```proto
message ChangePasswordRequest {
  string user_id = 1;
  string current_password = 2;
  string new_password = 3;
}

message ChangePasswordResponse {}
```

- [ ] **Step 4: Regenerate, lint, and check backward compatibility**

Run:

```bash
make gen
buf lint
buf breaking --against '.git#branch=master'
```

Expected: `buf lint` clean; `buf breaking` reports NO breaking changes (all additions are new field numbers / a new RPC).

- [ ] **Step 5: Commit**

```bash
git add proto/garcialorca/iam/v1/iam.proto
git commit -m "feat(iam): proto — must_change_password + ChangePassword RPC"
```

---

## Task 3: Domain — User carries mustChangePassword

**Files:**
- Modify: `services/iam/internal/domain/user.go`
- Test: `services/iam/internal/domain/user_test.go`

- [ ] **Step 1: Write the failing test**

Add to `services/iam/internal/domain/user_test.go` (create the file if absent; use the existing package `domain_test` if the file exists — match the sibling tests' package):

```go
func TestUserMustChangePassword(t *testing.T) {
	email, err := domain.NewEmail("director@garcialorca.edu.mx")
	if err != nil {
		t.Fatalf("NewEmail: %v", err)
	}
	u, err := domain.NewUser("u1", "t1", email, "Director", domain.StatusActive, true, nil)
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	if !u.MustChangePassword() {
		t.Error("MustChangePassword() = false, want true")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd services/iam && go test ./internal/domain/`
Expected: FAIL — `NewUser` takes the old arity / `MustChangePassword` undefined.

- [ ] **Step 3: Add the field, constructor param, and accessor**

In `services/iam/internal/domain/user.go`, add the field to the struct (after `status Status`):

```go
	status             Status
	mustChangePassword bool
	roles              []Role
```

Change the `NewUser` signature and body to thread the flag (insert `mustChangePassword bool` before `roles []Role`):

```go
func NewUser(id, tenantID string, email Email, displayName string, status Status, mustChangePassword bool, roles []Role) (User, error) {
	if strings.TrimSpace(id) == "" {
		return User{}, glerrors.New(glerrors.KindInvalid, "user id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return User{}, glerrors.New(glerrors.KindInvalid, "user tenant id is required")
	}
	assigned := make([]Role, len(roles))
	copy(assigned, roles)
	return User{
		id:                 id,
		tenantID:           tenantID,
		email:              email,
		displayName:        displayName,
		status:             status,
		mustChangePassword: mustChangePassword,
		roles:              assigned,
	}, nil
}
```

Add the accessor near `CanAuthenticate`:

```go
// MustChangePassword reports whether the account still holds a temporary
// password and must rotate it before doing anything else.
func (u User) MustChangePassword() bool { return u.mustChangePassword }
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd services/iam && go test ./internal/domain/`
Expected: PASS (the new test). Other packages won't compile yet — that's expected; Tasks 4–6 update the callers.

- [ ] **Step 5: Commit**

```bash
git add services/iam/internal/domain/user.go services/iam/internal/domain/user_test.go
git commit -m "feat(iam): domain User carries mustChangePassword"
```

---

## Task 4: Ports + usecase — expose flag, add ChangePassword

**Files:**
- Modify: `services/iam/internal/ports/ports.go`
- Modify: `services/iam/internal/usecase/auth.go`
- Test: `services/iam/internal/usecase/auth_change_password_test.go`

- [ ] **Step 1: Update the ports**

In `services/iam/internal/ports/ports.go`, change `FindAuthByEmail` to also return the flag, and add `UpdatePassword`:

```go
	// FindAuthByEmail returns the user, their stored password hash, and whether
	// they must change their password, for login. A missing user is KindNotFound.
	FindAuthByEmail(ctx context.Context, tenantID string, email domain.Email) (user domain.User, passwordHash string, mustChange bool, err error)

	// FindByID returns a user (with roles resolved) or KindNotFound.
	FindByID(ctx context.Context, userID string) (domain.User, error)

	// FindAuthByID returns the user and their stored password hash by id
	// (ChangePassword verifies the current password).
	FindAuthByID(ctx context.Context, userID string) (user domain.User, passwordHash string, err error)

	// UpdatePassword sets a new hash and clears must_change_password.
	UpdatePassword(ctx context.Context, userID, newHash string) error

	// AddRole / RemoveRole change a user's role assignments.
	AddRole(ctx context.Context, userID, roleID string) error
	RemoveRole(ctx context.Context, userID, roleID string) error
```

- [ ] **Step 2: Write the failing ChangePassword test**

Create `services/iam/internal/usecase/auth_change_password_test.go`. It uses lightweight fakes for the ports; match the existing test package (`usecase_test` if the sibling tests use it, else `usecase`). Check `services/iam/internal/usecase/auth_test.go` for the existing fakes and reuse them; if none are exported, define minimal ones here:

```go
package usecase_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/usecase"
)

type cpUsers struct {
	user      domain.User
	hash      string
	newHash   string
	updated   bool
	findErr   error
}

func (f *cpUsers) FindAuthByEmail(context.Context, string, domain.Email) (domain.User, string, bool, error) {
	return domain.User{}, "", false, glerrors.ErrNotFound
}
func (f *cpUsers) FindByID(context.Context, string) (domain.User, error) { return f.user, f.findErr }
func (f *cpUsers) FindAuthByID(context.Context, string) (domain.User, string, error) {
	return f.user, f.hash, f.findErr
}
func (f *cpUsers) UpdatePassword(_ context.Context, _ , newHash string) error {
	f.updated, f.newHash = true, newHash
	return nil
}
func (f *cpUsers) AddRole(context.Context, string, string) error    { return nil }
func (f *cpUsers) RemoveRole(context.Context, string, string) error { return nil }

// hasher fake: "H:"+plain is the "hash"; Verify matches that shape.
type cpHasher struct{}

func (cpHasher) Hash(plain string) (string, error) { return "H:" + plain, nil }
func (cpHasher) Verify(hash, plain string) error {
	if hash == "H:"+plain {
		return nil
	}
	return glerrors.New(glerrors.KindUnauthorized, "invalid credentials")
}

func newCPService(users *cpUsers) *usecase.AuthService {
	// roles/issuer/validator are unused by ChangePassword; pass nil where safe.
	return usecase.NewAuthService(users, nil, cpHasher{}, nil, nil,
		"00000000-0000-0000-0000-000000000001", 15*time.Minute, time.Now)
}

func TestChangePasswordSuccessClearsFlag(t *testing.T) {
	email, _ := domain.NewEmail("director@garcialorca.edu.mx")
	u, _ := domain.NewUser("d1", "00000000-0000-0000-0000-000000000001", email, "Director", domain.StatusActive, true, nil)
	users := &cpUsers{user: u, hash: "H:GarciaLorca2026!"}
	svc := newCPService(users)

	err := svc.ChangePassword(context.Background(), "d1", "GarciaLorca2026!", "NuevaClave#2026")
	if err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if !users.updated || users.newHash != "H:NuevaClave#2026" {
		t.Errorf("updated=%v newHash=%q", users.updated, users.newHash)
	}
}

func TestChangePasswordWrongCurrentIsUnauthorized(t *testing.T) {
	email, _ := domain.NewEmail("director@garcialorca.edu.mx")
	u, _ := domain.NewUser("d1", "00000000-0000-0000-0000-000000000001", email, "Director", domain.StatusActive, true, nil)
	users := &cpUsers{user: u, hash: "H:GarciaLorca2026!"}
	svc := newCPService(users)

	err := svc.ChangePassword(context.Background(), "d1", "wrong", "NuevaClave#2026")
	if glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Errorf("KindOf = %v, want Unauthorized", glerrors.KindOf(err))
	}
	if users.updated {
		t.Error("password updated despite wrong current password")
	}
}

func TestChangePasswordRejectsShortNew(t *testing.T) {
	email, _ := domain.NewEmail("director@garcialorca.edu.mx")
	u, _ := domain.NewUser("d1", "00000000-0000-0000-0000-000000000001", email, "Director", domain.StatusActive, true, nil)
	users := &cpUsers{user: u, hash: "H:GarciaLorca2026!"}
	svc := newCPService(users)

	err := svc.ChangePassword(context.Background(), "d1", "GarciaLorca2026!", "short")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("KindOf = %v, want Invalid", glerrors.KindOf(err))
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd services/iam && go test ./internal/usecase/`
Expected: FAIL — `ChangePassword` undefined; `AuthResult` has no `MustChangePassword`.

- [ ] **Step 4: Update AuthResult, Authenticate, and add ChangePassword**

In `services/iam/internal/usecase/auth.go`:

Add the field to `AuthResult`:

```go
type AuthResult struct {
	AccessToken        string
	RefreshToken       string
	ExpiresIn          int64 // access token TTL in seconds
	User               domain.User
	MustChangePassword bool
}
```

Update `Authenticate` to consume the new `FindAuthByEmail` return and set the flag on the result. Replace the `user, hash, err := ...` block through the `return s.issueFor(...)` line:

```go
	user, hash, mustChange, err := s.users.FindAuthByEmail(ctx, s.tenantID, email)
	if err != nil {
		return AuthResult{}, glerrors.New(glerrors.KindUnauthorized, "invalid credentials")
	}
	if err := s.hasher.Verify(hash, password); err != nil {
		return AuthResult{}, glerrors.New(glerrors.KindUnauthorized, "invalid credentials")
	}
	if !user.CanAuthenticate() {
		return AuthResult{}, glerrors.New(glerrors.KindForbidden, "account is not active")
	}

	res, err := s.issueFor(ctx, user)
	if err != nil {
		return AuthResult{}, err
	}
	res.MustChangePassword = mustChange
	return res, nil
```

Add the minimum-length constant near the top of the file (after the imports):

```go
// minPasswordLen is the smallest acceptable new password (defense-in-depth; the
// gateway/UI also validate).
const minPasswordLen = 8
```

Add the use case (after `RefreshToken`):

```go
// ChangePassword sets a new password for userID after verifying the current one.
// It clears must_change_password (the forced-rotation flag). A wrong current
// password is Unauthorized; a too-short new password is Invalid.
func (s *AuthService) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	if len(newPassword) < minPasswordLen {
		return glerrors.New(glerrors.KindInvalid, "new password is too short")
	}
	_, hash, err := s.users.FindAuthByID(ctx, userID)
	if err != nil {
		return glerrors.New(glerrors.KindUnauthorized, "invalid credentials")
	}
	if err := s.hasher.Verify(hash, currentPassword); err != nil {
		return glerrors.New(glerrors.KindUnauthorized, "invalid credentials")
	}
	newHash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return err
	}
	return s.users.UpdatePassword(ctx, userID, newHash)
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd services/iam && go test ./internal/usecase/`
Expected: PASS (adapters/tests updated in Task 5 still won't compile — next task).

- [ ] **Step 6: Commit**

```bash
git add services/iam/internal/ports/ports.go services/iam/internal/usecase/auth.go services/iam/internal/usecase/auth_change_password_test.go
git commit -m "feat(iam): usecase ChangePassword + expose mustChangePassword"
```

---

## Task 5: Postgres adapter — flag mapping + UpdatePassword

**Files:**
- Modify: `services/iam/internal/adapters/postgres/models.go`
- Modify: `services/iam/internal/adapters/postgres/mapper.go`
- Modify: `services/iam/internal/adapters/postgres/repository.go`
- Test: `services/iam/internal/adapters/postgres/repository_integration_test.go`

- [ ] **Step 1: Add the column to the GORM model**

In `services/iam/internal/adapters/postgres/models.go`, add to `userModel` (after `Status`):

```go
	Status              string `gorm:"column:status"`
	MustChangePassword  bool   `gorm:"column:must_change_password"`
```

- [ ] **Step 2: Thread the flag through the mapper**

In `services/iam/internal/adapters/postgres/mapper.go`, change `toDomainUser` to also return the flag and pass it to `NewUser`. Update the signature and the two return sites:

```go
func toDomainUser(m userModel, roleRows []roleModel) (domain.User, string, bool, error) {
	email, err := domain.NewEmail(m.Email)
	if err != nil {
		return domain.User{}, "", false, err
	}
	roles := make([]domain.Role, 0, len(roleRows))
	for _, rr := range roleRows {
		role, err := toDomainRole(rr)
		if err != nil {
			return domain.User{}, "", false, err
		}
		roles = append(roles, role)
	}
	status, ok := statusToDomain[m.Status]
	if !ok {
		return domain.User{}, "", false, glerrors.New(glerrors.KindInternal, "unknown user status: "+m.Status)
	}
	user, err := domain.NewUser(m.ID, m.TenantID, email, m.DisplayName, status, m.MustChangePassword, roles)
	if err != nil {
		return domain.User{}, "", false, err
	}
	return user, m.PasswordHash, m.MustChangePassword, nil
}
```

- [ ] **Step 3: Update the repository to the new signatures + UpdatePassword**

In `services/iam/internal/adapters/postgres/repository.go`:

Change `FindAuthByEmail` to return the flag:

```go
func (r *UserRepo) FindAuthByEmail(ctx context.Context, _ string, email domain.Email) (domain.User, string, bool, error) {
	var (
		user       domain.User
		hash       string
		mustChange bool
	)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var um userModel
		if err := tx.Where("email = ?", email.String()).First(&um).Error; err != nil {
			return translateFindErr(err)
		}
		roleRows, err := loadRoles(tx, um.ID)
		if err != nil {
			return err
		}
		user, hash, mustChange, err = toDomainUser(um, roleRows)
		return err
	})
	if err != nil {
		return domain.User{}, "", false, err
	}
	return user, hash, mustChange, nil
}
```

Update `FindByID` to the new `toDomainUser` arity (ignore hash + flag):

```go
		user, _, _, err = toDomainUser(um, roleRows)
		return err
```

Add `FindAuthByID` and `UpdatePassword` (after `FindByID`):

```go
// FindAuthByID returns a user and their password hash by id (for ChangePassword).
func (r *UserRepo) FindAuthByID(ctx context.Context, userID string) (domain.User, string, error) {
	var (
		user domain.User
		hash string
	)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var um userModel
		if err := tx.Where("id = ?", userID).First(&um).Error; err != nil {
			return translateFindErr(err)
		}
		roleRows, err := loadRoles(tx, um.ID)
		if err != nil {
			return err
		}
		user, hash, _, err = toDomainUser(um, roleRows)
		return err
	})
	if err != nil {
		return domain.User{}, "", err
	}
	return user, hash, nil
}

// UpdatePassword sets a new hash and clears must_change_password in one write.
func (r *UserRepo) UpdatePassword(ctx context.Context, userID, newHash string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&userModel{}).
			Where("id = ?", userID).
			Updates(map[string]any{"password_hash": newHash, "must_change_password": false})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "updating password", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
	})
}
```

- [ ] **Step 4: Write the failing integration test + apply both migrations**

In `services/iam/internal/adapters/postgres/repository_integration_test.go`:

Change the single-file `migrationFile` constant to a directory and update `migrate()` to apply all iam migrations in order. Replace the `migrationFile` const with:

```go
	migrationsDir = "../../../../../migrations/iam"
```

Find the `migrate(...)` helper and make it apply every `*.sql` in `migrationsDir` sorted (use `os.ReadDir` + `sort`), executing each file's contents. Then add this test (uses `superDB`, which bypasses RLS, matching the seed's tenant):

```go
func TestOwnerSeedAndPasswordUpdate(t *testing.T) {
	const ownerTenant = "00000000-0000-0000-0000-000000000001"
	repo := pgadapter.NewUserRepo(appDB(t, ownerTenant), ownerTenant)

	email, err := domain.NewEmail("director@garcialorca.edu.mx")
	if err != nil {
		t.Fatalf("NewEmail: %v", err)
	}
	user, hash, mustChange, err := repo.FindAuthByEmail(context.Background(), ownerTenant, email)
	if err != nil {
		t.Fatalf("FindAuthByEmail: %v", err)
	}
	if !mustChange {
		t.Error("owner must_change_password = false, want true")
	}
	if err := bcrypt.NewHasher(0).Verify(hash, "GarciaLorca2026!"); err != nil {
		t.Errorf("seeded temp password does not verify: %v", err)
	}

	if err := repo.UpdatePassword(context.Background(), user.ID(), "H-new"); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
	_, _, mustChangeAfter, err := repo.FindAuthByEmail(context.Background(), ownerTenant, email)
	if err != nil {
		t.Fatalf("FindAuthByEmail after update: %v", err)
	}
	if mustChangeAfter {
		t.Error("must_change_password still true after UpdatePassword")
	}
}
```

If the test file has no `appDB(t, tenant)` helper that opens the app-role pool for a given tenant, add a small one near the other helpers (reuse `appDSN`):

```go
func appDB(t *testing.T, _ string) *gorm.DB {
	t.Helper()
	db, err := open(appDSN)
	if err != nil {
		t.Fatalf("open app db: %v", err)
	}
	return db
}
```

- [ ] **Step 5: Run the integration test**

Run: `cd services/iam && go test -tags=integration ./internal/adapters/postgres/`
Expected: PASS, including `TestOwnerSeedAndPasswordUpdate` and the existing RLS isolation tests (which must still pass with `0002` applied). Requires Docker.

- [ ] **Step 6: Commit**

```bash
git add services/iam/internal/adapters/postgres/
git commit -m "feat(iam): postgres — mustChangePassword mapping, UpdatePassword, seed IT"
```

---

## Task 6: gRPC handler — map flag + ChangePassword

**Files:**
- Modify: `services/iam/internal/adapters/grpc/iam_handler.go`
- Test: `services/iam/internal/adapters/grpc/iam_handler_test.go` (match the actual handler test filename in that dir)

- [ ] **Step 1: Write the failing handler test**

In the grpc adapter test file, add a test that `Login` surfaces the flag and `ChangePassword` maps errors. Reuse the existing fakes for `AuthService` collaborators if present; otherwise this asserts via a constructed `AuthService` with fakes (mirror Task 4's fakes). Minimal mapping test for the response field:

```go
func TestLoginResponseCarriesMustChangePassword(t *testing.T) {
	// Arrange an AuthService whose Authenticate returns MustChangePassword=true.
	// Reuse the usecase-level fakes: a users repo returning a seeded owner with
	// must_change=true and a hasher that verifies the temp password.
	// (See services/iam/internal/usecase/auth_change_password_test.go for fakes.)
	// Then assert handler.Login(...).Msg.GetMustChangePassword() == true.
	t.Skip("wire with existing handler test fakes; assert GetMustChangePassword() == true")
}
```

> Note: replace the `t.Skip` with the concrete arrangement using whatever fakes the existing `iam_handler_test.go` already defines. If that file constructs a real `usecase.AuthService` with in-memory fakes, copy that setup and set the seeded user's `must_change=true`. Keep the assertion: `res.Msg.GetMustChangePassword()` is `true`.

- [ ] **Step 2: Run to verify it fails/*skips*, then implement**

Run: `cd services/iam && go test ./internal/adapters/grpc/`
Expected: compile error (`GetMustChangePassword` undefined until gen is regenerated in Task 2 — it is) → then the skip. Proceed to implementation.

- [ ] **Step 3: Map the flag in Login**

In `services/iam/internal/adapters/grpc/iam_handler.go`, in `Login`, add the field to the response:

```go
	return connect.NewResponse(&iamv1.LoginResponse{
		AccessToken:        res.AccessToken,
		RefreshToken:       res.RefreshToken,
		ExpiresIn:          res.ExpiresIn,
		User:               toProtoUser(res.User),
		MustChangePassword: res.MustChangePassword,
	}), nil
```

Also set it on the mapped user in `toProtoUser`:

```go
func toProtoUser(u domain.User) *iamv1.User {
	return &iamv1.User{
		Id:                 u.ID(),
		TenantId:           u.TenantID(),
		Email:              u.Email().String(),
		DisplayName:        u.DisplayName(),
		Status:             toProtoStatus(u.Status()),
		MustChangePassword: u.MustChangePassword(),
	}
}
```

- [ ] **Step 4: Implement the ChangePassword handler**

Add to `iam_handler.go` (after `RefreshToken`):

```go
func (h *IAMHandler) ChangePassword(ctx context.Context, req *connect.Request[iamv1.ChangePasswordRequest]) (*connect.Response[iamv1.ChangePasswordResponse], error) {
	if err := h.auth.ChangePassword(ctx, req.Msg.GetUserId(), req.Msg.GetCurrentPassword(), req.Msg.GetNewPassword()); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&iamv1.ChangePasswordResponse{}), nil
}
```

(The compile-time `var _ iamv1connect.IamServiceHandler = (*IAMHandler)(nil)` now requires this method — good.)

- [ ] **Step 5: Finish the test and run**

Replace the `t.Skip` from Step 1 with the concrete arrangement and assertion. Run: `cd services/iam && go test ./...`
Expected: PASS across the iam module.

- [ ] **Step 6: Commit**

```bash
git add services/iam/internal/adapters/grpc/
git commit -m "feat(iam): handler maps mustChangePassword + ChangePassword RPC"
```

---

## Task 7: Gateway — cookie, flag passthrough, change-password route

**Files:**
- Modify: `services/gateway/internal/client/iamauth.go`
- Modify: `services/gateway/internal/handler/auth.go`
- Test: `services/gateway/internal/handler/auth_test.go`
- Modify: `services/gateway/internal/middleware/auth.go`
- Modify: `services/gateway/cmd/main.go`

- [ ] **Step 1: Extend the auth client**

In `services/gateway/internal/client/iamauth.go`, add `MustChangePassword` to `LoginResult` and a `ChangePassword` method. Update `LoginResult`:

```go
type LoginResult struct {
	AccessToken        string
	RefreshToken       string
	ExpiresIn          int64
	MustChangePassword bool
}
```

In `Login`, set the flag from the response:

```go
	return LoginResult{
		AccessToken:        res.Msg.GetAccessToken(),
		RefreshToken:       res.Msg.GetRefreshToken(),
		ExpiresIn:          res.Msg.GetExpiresIn(),
		MustChangePassword: res.Msg.GetMustChangePassword(),
	}, nil
```

Add the method (after `Refresh`):

```go
// ChangePassword rotates the password for userID via iam.
func (c *AuthClient) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	_, err := c.rpc.ChangePassword(ctx, connect.NewRequest(&iamv1.ChangePasswordRequest{
		UserId:          userID,
		CurrentPassword: currentPassword,
		NewPassword:     newPassword,
	}))
	return err
}
```

- [ ] **Step 2: Write the failing handler tests**

In `services/gateway/internal/handler/auth_test.go`, extend `fakeLogin` with the new capability and add tests. Update the fake and add cases:

```go
func (f fakeLogin) ChangePassword(context.Context, string, string, string) error { return f.cpErr }
```

Add `cpErr error` to the `fakeLogin` struct, then:

```go
func TestLoginResponseIncludesMustChange(t *testing.T) {
	h := handler.NewAuthHandler(fakeLogin{res: client.LoginResult{AccessToken: "a", RefreshToken: "r", ExpiresIn: 900, MustChangePassword: true}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"d@x","password":"p"}`))
	h.Login(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["mustChangePassword"] != true {
		t.Errorf("mustChangePassword=%v, want true", body["mustChangePassword"])
	}
	if c := rec.Result().Cookies(); len(c) == 0 || c[0].Name != "gl_access" || !c[0].HttpOnly {
		t.Errorf("expected HttpOnly gl_access cookie, got %v", rec.Result().Cookies())
	}
}
```

- [ ] **Step 3: Run to verify failure**

Run: `cd services/gateway && go test ./internal/handler/`
Expected: FAIL — `mustChangePassword` absent; no cookie; `LoginService` lacks `ChangePassword`.

- [ ] **Step 4: Update the handler**

In `services/gateway/internal/handler/auth.go`:

Extend the `LoginService` interface:

```go
type LoginService interface {
	Login(ctx context.Context, email, password string) (client.LoginResult, error)
	Refresh(ctx context.Context, refreshToken string) (client.LoginResult, error)
	ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error
}
```

Add the field to `loginResponse`:

```go
type loginResponse struct {
	AccessToken        string `json:"accessToken"`
	RefreshToken       string `json:"refreshToken"`
	ExpiresIn          int64  `json:"expiresIn"`
	MustChangePassword bool   `json:"mustChangePassword"`
}
```

In `Login`, set the HttpOnly cookie and the flag before writing JSON:

```go
	http.SetCookie(w, &http.Cookie{
		Name:     "gl_access",
		Value:    res.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(res.ExpiresIn),
	})
	writeJSON(w, http.StatusOK, loginResponse{
		AccessToken:        res.AccessToken,
		RefreshToken:       res.RefreshToken,
		ExpiresIn:          res.ExpiresIn,
		MustChangePassword: res.MustChangePassword,
	})
```

Add the change-password handler (after `Me`). It runs behind `RequireAuth`, so the principal is in context:

```go
type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// ChangePassword handles POST /api/v1/auth/change-password for the authenticated
// principal. RequireAuth injects the principal; the user id is never taken from
// the request body.
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req changePasswordRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := h.auth.ChangePassword(r.Context(), p.UserID, req.CurrentPassword, req.NewPassword); err != nil {
		http.Error(w, http.StatusText(httpStatusFor(err)), httpStatusFor(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 5: Accept the cookie as a token source in RequireAuth**

In `services/gateway/internal/middleware/auth.go`, find where `RequireAuth` reads `Authorization: Bearer`. Add a cookie fallback: if the header is missing/empty, read the `gl_access` cookie. Locate the bearer-extraction block and adjust so the token comes from the header OR the cookie (keep the existing 401 when neither is present). Concretely, where the token string is obtained, use:

```go
		token := bearerToken(r) // existing header parse → "" if absent
		if token == "" {
			if c, err := r.Cookie("gl_access"); err == nil {
				token = c.Value
			}
		}
		if token == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
```

(If the existing code inlines the header parsing rather than a `bearerToken` helper, extract that logic into a small `bearerToken(r *http.Request) string` helper in the same file first, then apply the block above.)

- [ ] **Step 6: Route change-password in the mux**

In `services/gateway/cmd/main.go`, in `newMux`, after the `GET /api/v1/me` line add:

```go
	mux.Handle("POST /api/v1/auth/change-password", requireAuth(http.HandlerFunc(authH.ChangePassword)))
```

- [ ] **Step 7: Run gateway tests**

Run: `cd services/gateway && go test ./...`
Expected: PASS (handler + middleware + existing).

- [ ] **Step 8: Commit**

```bash
git add services/gateway/
git commit -m "feat(gateway): auth cookie, mustChangePassword passthrough, change-password route"
```

---

## Task 8: Web — real login + forced change-password

**Files:**
- Create: `apps/web/app/auth/api.ts`
- Test: `apps/web/app/auth/api.test.ts`
- Modify: `apps/web/app/_screens/Login.tsx`
- Create: `apps/web/app/_screens/ChangePassword.tsx`
- Modify: `apps/web/app/AppShell.tsx`

- [ ] **Step 1: Write the failing api-helper test**

Create `apps/web/app/auth/api.test.ts` (Vitest; mirror `packages/api-client/src/transport.test.ts` style):

```ts
import { afterEach, describe, expect, it, vi } from "vitest";
import { changePassword, login } from "./api";

afterEach(() => vi.restoreAllMocks());

describe("login", () => {
  it("posts credentials and returns mustChangePassword", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ accessToken: "a", refreshToken: "r", expiresIn: 900, mustChangePassword: true }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    const res = await login("http://gw", "d@x", "p");
    expect(res.mustChangePassword).toBe(true);
    expect(fetchMock).toHaveBeenCalledWith("http://gw/api/v1/auth/login", expect.objectContaining({ method: "POST", credentials: "include" }));
  });

  it("throws on 401", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 401 })));
    await expect(login("http://gw", "d@x", "bad")).rejects.toThrow();
  });
});

describe("changePassword", () => {
  it("posts current+new and resolves on 204", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("", { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);
    await expect(changePassword("http://gw", "old", "newlongpass")).resolves.toBeUndefined();
    expect(fetchMock).toHaveBeenCalledWith("http://gw/api/v1/auth/change-password", expect.objectContaining({ method: "POST", credentials: "include" }));
  });
});
```

- [ ] **Step 2: Run to verify it fails**

Run: `pnpm --filter @garcia-lorca/web test -- auth/api.test.ts`
Expected: FAIL — `./api` does not exist.

- [ ] **Step 3: Implement the api helpers**

Create `apps/web/app/auth/api.ts`:

```ts
/**
 * Thin fetch helpers for the gateway auth endpoints. Credentials ("include")
 * carry the HttpOnly gl_access cookie the gateway sets on login; the returned
 * access token is also available for callers that prefer a bearer header.
 */
export interface LoginResult {
  accessToken: string;
  refreshToken: string;
  expiresIn: number;
  mustChangePassword: boolean;
}

function url(baseURL: string, path: string): string {
  return `${baseURL.replace(/\/+$/, "")}${path}`;
}

export async function login(baseURL: string, email: string, password: string): Promise<LoginResult> {
  const res = await fetch(url(baseURL, "/api/v1/auth/login"), {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
  if (!res.ok) {
    throw new Error(`login: HTTP ${res.status}`);
  }
  return (await res.json()) as LoginResult;
}

export async function changePassword(baseURL: string, currentPassword: string, newPassword: string): Promise<void> {
  const res = await fetch(url(baseURL, "/api/v1/auth/change-password"), {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ currentPassword, newPassword }),
  });
  if (!res.ok) {
    throw new Error(`changePassword: HTTP ${res.status}`);
  }
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `pnpm --filter @garcia-lorca/web test -- auth/api.test.ts`
Expected: PASS.

- [ ] **Step 5: Replace the fake role-picker with a real login form**

Rewrite `apps/web/app/_screens/Login.tsx` so it collects **email + password** and calls a callback with credentials (keep the existing brand panel/markup; change only the form panel + props/state). Replace the `LoginProps` interface and the component's state/handlers:

```tsx
export interface LoginProps {
  theme: Theme;
  onToggle: () => void;
  onSubmit: (email: string, password: string) => Promise<void>;
  error?: string;
  pending?: boolean;
}
```

Remove the `Role`/`ROLES`/`PLACEHOLDER_USER` role-picker block and its segmented buttons. Replace the component's local state:

```tsx
export function Login({ theme, onToggle, onSubmit, error, pending }: LoginProps): ReactElement {
  const [email, setEmail] = useState("");
  const [pwd, setPwd] = useState("");
  const [show, setShow] = useState(false);
```

Change the "Usuario" field to an email input bound to `email`/`setEmail` (label "Correo", `type="email"`, `autoComplete="username"`), keep the password field bound to `pwd`/`setPwd`, and change the submit button to:

```tsx
          <Btn full icon="logout" onClick={() => void onSubmit(email, pwd)} disabled={pending} style={{ height: 54, fontSize: 17 }}>
            {pending ? "Entrando…" : "Entrar al sistema"}
          </Btn>
          {error ? <p role="alert" style={{ color: "var(--orange)", fontSize: 14, marginTop: 12, textAlign: "center" }}>{error}</p> : null}
```

(Delete the now-unused `Role`, `RoleOption`, `ROLES`, `PLACEHOLDER_USER`, and the `Icon`-based role segmented group. Keep `Rosette`, brand panel, theme toggle.)

- [ ] **Step 6: Create the forced change-password screen**

Create `apps/web/app/_screens/ChangePassword.tsx`:

```tsx
"use client";

import { useState, type CSSProperties, type ReactElement } from "react";
import { Btn, ThemeToggle } from "../_design/Controls";
import type { Theme } from "../theme/theme-script";

export interface ChangePasswordProps {
  theme: Theme;
  onToggle: () => void;
  onSubmit: (currentPassword: string, newPassword: string) => Promise<void>;
  error?: string;
  pending?: boolean;
}

const inp: CSSProperties = {
  width: "100%",
  height: 52,
  padding: "0 16px",
  borderRadius: "var(--r-md)",
  border: "1.5px solid var(--border)",
  background: "var(--surface)",
  color: "var(--text)",
  fontSize: 16,
  outline: "none",
};

const MIN_LEN = 8;

export function ChangePassword({ theme, onToggle, onSubmit, error, pending }: ChangePasswordProps): ReactElement {
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");

  const tooShort = next.length > 0 && next.length < MIN_LEN;
  const mismatch = confirm.length > 0 && next !== confirm;
  const canSubmit = !pending && current.length > 0 && next.length >= MIN_LEN && next === confirm;

  return (
    <div className="app-root" style={{ minHeight: "100vh", display: "grid", placeItems: "center", padding: 40 }}>
      <div style={{ position: "absolute", top: 24, right: 28 }}>
        <ThemeToggle theme={theme} onToggle={onToggle} />
      </div>
      <div className="fade-up" style={{ width: "100%", maxWidth: 412 }}>
        <h2 style={{ fontSize: 28, fontWeight: 700, color: "var(--text)" }}>Crea tu contraseña</h2>
        <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 8 }}>
          Por seguridad, cambia la contraseña temporal antes de continuar.
        </p>
        <div style={{ marginTop: 22, display: "flex", flexDirection: "column", gap: 14 }}>
          <input aria-label="Contraseña actual" type="password" autoComplete="current-password" placeholder="Contraseña temporal" value={current} onChange={(e) => setCurrent(e.target.value)} style={inp} />
          <input aria-label="Nueva contraseña" type="password" autoComplete="new-password" placeholder="Nueva contraseña" value={next} onChange={(e) => setNext(e.target.value)} style={inp} />
          <input aria-label="Confirmar contraseña" type="password" autoComplete="new-password" placeholder="Confirmar contraseña" value={confirm} onChange={(e) => setConfirm(e.target.value)} style={inp} />
        </div>
        {tooShort ? <p style={{ color: "var(--text-muted)", fontSize: 13, marginTop: 10 }}>Mínimo {MIN_LEN} caracteres.</p> : null}
        {mismatch ? <p style={{ color: "var(--orange)", fontSize: 13, marginTop: 10 }}>Las contraseñas no coinciden.</p> : null}
        <Btn full icon="shield" onClick={() => void onSubmit(current, next)} disabled={!canSubmit} style={{ height: 54, fontSize: 17, marginTop: 20 }}>
          {pending ? "Guardando…" : "Guardar y entrar"}
        </Btn>
        {error ? <p role="alert" style={{ color: "var(--orange)", fontSize: 14, marginTop: 12, textAlign: "center" }}>{error}</p> : null}
      </div>
    </div>
  );
}
```

- [ ] **Step 7: Wire the flow in AppShell**

In `apps/web/app/AppShell.tsx`, replace the fake `login`/role state with the real flow. Add imports:

```tsx
import { login as apiLogin, changePassword as apiChangePassword } from "./auth/api";
import { ChangePassword } from "./_screens/ChangePassword";
```

Add the gateway base URL (public env, matches compose `NEXT_PUBLIC_API_URL`):

```tsx
const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
```

Extend the `View` union with a change-password screen:

```tsx
type View =
  | { screen: "login" }
  | { screen: "changePassword" }
  | { screen: "dashboard" }
  | { screen: "finance" }
  | { screen: "module"; moduleId: string };
```

Replace the `login`/`logout` handlers and `role` state usage with credential-driven handlers (keep `USERS`/`role` only for the existing dashboard chrome — default it to `"admin"`):

```tsx
  const [authError, setAuthError] = useState<string | undefined>(undefined);
  const [pending, setPending] = useState(false);

  const handleLogin = async (email: string, password: string): Promise<void> => {
    setPending(true);
    setAuthError(undefined);
    try {
      const res = await apiLogin(API_URL, email, password);
      setView(res.mustChangePassword ? { screen: "changePassword" } : { screen: "dashboard" });
    } catch {
      setAuthError("Correo o contraseña incorrectos.");
    } finally {
      setPending(false);
    }
  };

  const handleChangePassword = async (current: string, next: string): Promise<void> => {
    setPending(true);
    setAuthError(undefined);
    try {
      await apiChangePassword(API_URL, current, next);
      setView({ screen: "dashboard" });
    } catch {
      setAuthError("No se pudo cambiar la contraseña. Verifica la actual.");
    } finally {
      setPending(false);
    }
  };

  const logout = (): void => setView({ screen: "login" });
```

Replace the login render branch and add the change-password branch:

```tsx
  if (view.screen === "login") {
    return <Login theme={theme} onToggle={toggle} onSubmit={handleLogin} error={authError} pending={pending} />;
  }
  if (view.screen === "changePassword") {
    return <ChangePassword theme={theme} onToggle={toggle} onSubmit={handleChangePassword} error={authError} pending={pending} />;
  }
```

- [ ] **Step 8: Run web tests, typecheck, and lint**

Run:

```bash
pnpm --filter @garcia-lorca/web test
pnpm --filter @garcia-lorca/web exec tsc --noEmit
pnpm --filter @garcia-lorca/web lint
```

Expected: tests PASS; no type errors; lint clean (remove any now-unused imports the linter flags in `Login.tsx`/`AppShell.tsx`).

- [ ] **Step 9: Commit**

```bash
git add apps/web/app/auth/ apps/web/app/_screens/Login.tsx apps/web/app/_screens/ChangePassword.tsx apps/web/app/AppShell.tsx
git commit -m "feat(web): real login + forced change-password, replacing the role-picker"
```

---

## Task 9: End-to-end verification + flags

- [ ] **Step 1: Bring up the full stack fresh**

Run:

```bash
make run-new
```

Expected: `postgres` healthy → `migrate` completes (applies iam/0001, iam/0002, students/0001) → services start → web on :3000, gateway on :8080.

- [ ] **Step 2: (Optional) enable spine flags for later module routes**

The auth routes (`/api/v1/auth/login`, `/auth/change-password`, `/me`) are **not** behind `ModuleGuard` (see `newMux`), so the Phase-1 login flow works regardless of flags. Flags only gate module routes (e.g. `POST /api/v1/students`), which land in later phases. To prep, set `ENABLED_MODULES` for the `gateway` service in `docker-compose.yml` (add `ENABLED_MODULES: "iam,gateway,students,config,grades"` if absent), then `make restart_gateway` and verify:

```bash
curl -s localhost:8080/api/v1/features
```

Expected JSON includes `"module_iam":true`, `"module_gateway":true`, `"module_students":true`.

- [ ] **Step 3: Exercise the login → forced change → me flow via curl**

```bash
# 1) First login: expect mustChangePassword=true and a Set-Cookie: gl_access
curl -s -i -X POST localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"director@garcialorca.edu.mx","password":"GarciaLorca2026!"}'

# Save the cookie jar for the authenticated calls:
curl -s -c /tmp/gl.cookies -X POST localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"director@garcialorca.edu.mx","password":"GarciaLorca2026!"}' >/dev/null

# 2) Change the password (authenticated by cookie):
curl -s -i -b /tmp/gl.cookies -X POST localhost:8080/api/v1/auth/change-password \
  -H 'Content-Type: application/json' \
  -d '{"currentPassword":"GarciaLorca2026!","newPassword":"Direccion#2026"}'

# 3) Re-login with the NEW password: expect mustChangePassword=false
curl -s -X POST localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"director@garcialorca.edu.mx","password":"Direccion#2026"}'

# 4) /me works with the fresh cookie:
curl -s -c /tmp/gl2.cookies -X POST localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"director@garcialorca.edu.mx","password":"Direccion#2026"}' >/dev/null
curl -s -b /tmp/gl2.cookies localhost:8080/api/v1/me
```

Expected: (1) `"mustChangePassword":true` + `Set-Cookie: gl_access=...; HttpOnly`; (2) `204 No Content`; (3) `"mustChangePassword":false`; (4) `/me` returns the director principal with `permissions` including `students:create`.

> Note: after Step 2 the seeded temp password is gone. To re-run from scratch: `make run-new` (drops volumes and re-seeds).

- [ ] **Step 4: Browser smoke**

Open `http://localhost:3000`. Log in with `director@garcialorca.edu.mx` / (temp or changed password). On a fresh DB you are routed to the change-password screen; after saving you land on the dashboard. A wrong password shows the inline error.

- [ ] **Step 5: Full test + lint gate**

Run:

```bash
make test
make lint
```

Expected: Go tests (`go test ./... -race`) and `pnpm turbo run test` PASS; `buf lint`, `go vet`, and frontend lint clean. (Integration tests run separately: `cd services/iam && go test -tags=integration ./internal/adapters/postgres/`.)

- [ ] **Step 6: Update docs and commit**

Update `.claude/docs/FLAGS.md` presentation presets to add `preset_academic_spine` (`module_iam`, `module_gateway`, `module_students`, `module_config`, `module_grades`) and note the owner-login flow in the module status. Then:

```bash
git add .claude/docs/FLAGS.md
git commit -m "docs(flags): academic-spine preset + owner login note"
```

---

## Security notes for the gate (before enabling flags in shared envs)

- **Temp password in a migration** is acceptable for local/dev seeding only. For staging/production, the owner seed must come from the secret manager (not a committed hash), and `must_change_password=true` must be enforced. Flag this to the security agent.
- **Forced-rotation enforcement is UX-side in Phase 1:** login returns full-permission tokens plus `must_change_password`, and the web routes to the change-password screen. A determined caller could ignore the flag. Hardening (carry `must_change` in the access-token claims and have the gateway 403 everything except change-password until it clears) is a follow-up required before the security gate passes for a shared env. Documented, not built here.
- **Cookie:** `gl_access` is `HttpOnly + Secure + SameSite=Strict`. `Secure` requires HTTPS; for plain-HTTP localhost the browser still stores it on `http://localhost` (allowed exception), and curl works over the cookie jar. Confirm behavior in the target browser during Step 4.

## Deferred from the spec (not blocking Phase 1)

- **RLS session-var consolidation (spec §3.1 defect #1).** The migrations and every
  service already key RLS on `app.current_tenant`; the only inconsistency is the unused
  `pkg/authctx` helper (which binds `app.current_user`). Login works correctly on the
  existing per-repo `withTenant`, so consolidating `pkg/authctx` is deferred to the
  `config`-service phase (Phase 2), where a shared helper pays off across new adapters.
- **`students.grade_group` reconciliation (spec §3.1 defect #2).** Superseded by
  `enrollments → groups`; handled in the enrollment phase (Phase 4), not here.

## Testing strategy recap (per repo canon)

- Unit: domain (`MustChangePassword`), usecase (`ChangePassword` happy/wrong-current/short-new), gateway handler (flag + cookie), web api helpers (mocked fetch).
- Integration (testcontainers-go): owner seed present + `must_change_password` toggles via `UpdatePassword`; existing RLS isolation tests still green with `0002` applied.
- Flag-aware: gateway routes are behind `ModuleGuard`; OFF → 404 is already covered by existing middleware tests (extend if a login-specific OFF case is wanted).
- Every fixed bug during implementation leaves a regression test.
