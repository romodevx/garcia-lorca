# Phase 1 Hardening — 7 Follow-ups Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the 7 tracked Phase-1 follow-ups with full test coverage and no security or performance regressions: server-side forced-rotation enforcement (#1), proto `user_id` removal (#7), cookie/token lifecycle (#3), edge hardening — login rate-limit + gateway password validation (#5), `pkg/authctx` consolidation (#4), owner-secret-manager stub + docs (#2), and the iam RLS doc fix (#6).

**Architecture:** iam stays the only token issuer; the access token now carries a `must_change_password` claim that the gateway enforces (403 everything except change-password until cleared). Auth tokens live only in HttpOnly cookies (never the JSON body). A per-instance token-bucket rate limiter fronts `/auth/login`. RLS binding is unified in `pkg/authctx` on `app.current_tenant`, used by every service's postgres adapter. Owner-secret sourcing for shared envs is stubbed in Terraform + documented (local path unchanged).

**Tech Stack:** Go 1.24/1.25 (Clean Arch), GORM (adapters only), Connect/gRPC, HMAC JWT, bcrypt/pgcrypto, Postgres 16 + RLS, buf, Next.js (web), pnpm, testcontainers-go, `testing.B` benchmarks, govulncheck.

---

## Delivery model

Five phases, **each shipped as its own PR** off `master` with a checkpoint. Order matters: A → B → C → D → E (E's doc fix describes the state D leaves). This plan file is committed on `feature/phase1-hardening`; each phase branches off `master` (rebasing on the prior phase once merged).

| Phase | Follow-ups | PR theme |
|-------|-----------|----------|
| A | #1, #7 | Server-side forced-rotation enforcement + drop `user_id` from the wire |
| B | #3 | Cookie/token lifecycle (tokens only in HttpOnly cookies; rotate on refresh; clear both on logout) |
| C | #5 | Edge hardening: login rate-limit + gateway password validation |
| D | #4 | `pkg/authctx` consolidation onto `app.current_tenant` |
| E | #2, #6 | Owner-secret Terraform stub + docs; iam RLS doc fix |

**Cross-phase Definition of Done (every phase):** `gofmt`/`go vet` clean; unit + integration (testcontainers) green; RLS isolation tests still pass where touched; `buf lint` + `buf breaking` (vs `master`) clean when proto changes; web `test`/`tsc`/`lint` green when web changes; **no secret literals** (`git grep` guard); a benchmark for any new hot path; and the security-agent gate + tech-lead gate pass before merge. `gen/` and `go.work.sum` are never staged. buf runs via Docker (`docker run --rm -v "$PWD":/work -w /work bufbuild/buf:latest <cmd>`); base branch is `master`.

---

## Current-state anchors (verified 2026-08-03)

- `proto/.../iam.proto` `message Claims { subject=1; tenant_id=2; email=3; permissions=4; expires_at=5; }` — **no** `must_change_password`.
- `domain.NewAccessClaims(user, now, ttl)` builds `domain.Claims` from `user`; `user.MustChangePassword()` exists.
- `jwt/hmac.go` `accessClaims` has `TenantID/Email/Permissions/TokenType` — no mcp flag.
- `grpc/iam_handler.go` `toProtoClaims` maps the 5 fields.
- iam `ChangePassword` RPC + usecase take `user_id` (from the request body).
- gateway `middleware.Principal{UserID,TenantID,Email,Permissions}`; `RequireAuth` sources token from `Authorization: Bearer` then `gl_access` cookie; `RequirePermission` exists.
- gateway `handler/auth.go`: `Login` sets `gl_access` cookie AND returns tokens in JSON; `Refresh` reads refresh token from the **body** and returns tokens in JSON, sets **no** cookie; `Logout` clears `gl_access`; `ChangePassword` sources `userID` from the principal.
- gateway `client/iamauth.go` `ValidateToken` maps proto claims → `Principal` (no mcp).
- gateway `cmd/main.go` `newMux` wires all routes; auth routes are not module-gated.
- `pkg/authctx` binds **`app.current_user`** via `WithUserContext` and is **unused**; iam and students postgres adapters each roll their own `withTenant` binding **`app.current_tenant`** (`SELECT set_config('app.current_tenant', ?, true)`).
- `deploy/terraform/secrets.tf` exists; `.env.example` has `OWNER_TEMP_PASSWORD=change-me-owner`; owner user seeded by `docker/seed-owner.sql` via pgcrypto.
- `services/iam/CLAUDE.md:31-32` claims RLS goes through `pkg/authctx.WithUserContext` / `app.current_user` (stale).

---

# Phase A — Forced-rotation enforcement (#1, #7)

**Goal:** The access token carries `must_change_password`; the gateway blocks every authenticated route except change-password while it is true. iam derives the change-password subject from the access token (not a body field), and `ChangePasswordRequest.user_id` is removed.

**Files**
- Modify: `proto/garcialorca/iam/v1/iam.proto` (Claims + ChangePasswordRequest)
- Modify: `services/iam/internal/domain/claims.go`
- Test: `services/iam/internal/domain/claims_test.go`
- Modify: `services/iam/internal/adapters/jwt/hmac.go`
- Test: `services/iam/internal/adapters/jwt/hmac_test.go`
- Modify: `services/iam/internal/adapters/grpc/iam_handler.go`
- Modify: `services/iam/internal/usecase/auth.go` (ChangePassword derives subject from token)
- Modify: `services/iam/internal/ports/ports.go` (if needed for validator use)
- Test: `services/iam/internal/usecase/auth_change_password_test.go`
- Modify: `services/gateway/internal/middleware/auth.go` (Principal + enforcement middleware)
- Test: `services/gateway/internal/middleware/auth_test.go`
- Modify: `services/gateway/internal/client/iamauth.go` (Principal mcp; ChangePassword passes token)
- Modify: `services/gateway/internal/handler/auth.go` (change-password passes the token)
- Modify: `services/gateway/cmd/main.go` (apply enforcement to protected routes)

### Task A1: Proto — add the claim, remove `user_id`

- [ ] **Step 1: Edit the proto**

In `message Claims`, after `expires_at = 5;` add:
```proto
  // must_change_password mirrors the account flag so the gateway can enforce the
  // forced first-login rotation without an extra round-trip to iam.
  bool must_change_password = 6;
```
Replace `message ChangePasswordRequest { ... }` with (drop `user_id`; the subject comes from the token):
```proto
message ChangePasswordRequest {
  // access_token identifies the principal (iam derives the subject from it);
  // the current password is re-verified before the change.
  string access_token = 1;
  string current_password = 2;
  string new_password = 3;
}
```

- [ ] **Step 2: Regenerate + validate**

Run:
```bash
docker run --rm -v "$PWD":/work -w /work bufbuild/buf:latest generate
docker run --rm -v "$PWD":/work -w /work bufbuild/buf:latest lint
docker run --rm -v "$PWD":/work -w /work bufbuild/buf:latest breaking --against '.git#branch=master'
```
Expected: `lint` clean. `breaking` **will report a change** for removing `ChangePasswordRequest.user_id` (field removal is wire-breaking). This is intentional and acceptable — the RPC is internal (gateway ↔ iam only) and has no external consumers. Record the breaking output and proceed. (Do not add `user_id` back.)

- [ ] **Step 3: Confirm gen + commit the proto**

```bash
cd gen/go && go build ./... ; cd /Users/andresromo/Repositories/Personal/garcia-lorca
git add proto/garcialorca/iam/v1/iam.proto
git commit -m "feat(iam): proto — Claims.must_change_password; ChangePassword by access_token (drop user_id)"
```

### Task A2: Domain claims carry the flag

- [ ] **Step 1: Failing test**

Add to `services/iam/internal/domain/claims_test.go`:
```go
func TestNewAccessClaimsCarriesMustChangePassword(t *testing.T) {
	email, _ := domain.NewEmail("owner@example.com")
	u, _ := domain.NewUser("u1", "t1", email, "Owner", domain.StatusActive, true, nil)
	c := domain.NewAccessClaims(u, time.Unix(0, 0), time.Minute)
	if !c.MustChangePassword {
		t.Error("Claims.MustChangePassword = false, want true")
	}
}
```

- [ ] **Step 2: Run → fail**

`cd services/iam && go test ./internal/domain/` → FAIL (`MustChangePassword` undefined on Claims).

- [ ] **Step 3: Implement**

In `services/iam/internal/domain/claims.go`, add the field to `Claims`:
```go
	Permissions []string
	MustChangePassword bool
	ExpiresAt   time.Time
```
In `NewAccessClaims`, set it (before the `return`):
```go
	return Claims{
		Subject:            user.ID(),
		TenantID:           user.TenantID(),
		Email:              user.Email().String(),
		Permissions:        permissions,
		MustChangePassword: user.MustChangePassword(),
		ExpiresAt:          now.Add(ttl),
	}
```

- [ ] **Step 4: Run → pass**; then commit with A3 (same layer).

### Task A3: JWT carries + returns the flag

- [ ] **Step 1: Failing test**

Add to `services/iam/internal/adapters/jwt/hmac_test.go` a round-trip asserting the flag survives issue→validate (mirror the existing access-token test; set a `domain.Claims{... MustChangePassword: true}` , issue, validate, assert `got.MustChangePassword == true`).

- [ ] **Step 2: Run → fail.**

- [ ] **Step 3: Implement**

In `services/iam/internal/adapters/jwt/hmac.go`, add to `accessClaims`:
```go
	Permissions []string `json:"perms"`
	MustChangePassword bool `json:"mcp"`
	TokenType   string   `json:"typ"`
```
In `IssueAccessToken`, set `MustChangePassword: claims.MustChangePassword` in the `accessClaims{...}` literal. In `ValidateAccessToken`, set `MustChangePassword: body.MustChangePassword` in the returned `domain.Claims{...}`.

- [ ] **Step 4: Run → pass.**

- [ ] **Step 5: grpc mapping** — in `services/iam/internal/adapters/grpc/iam_handler.go` `toProtoClaims`, add `MustChangePassword: c.MustChangePassword,` to the `&iamv1.Claims{...}` literal.

- [ ] **Step 6: Commit**
```bash
git add services/iam/internal/domain/claims.go services/iam/internal/domain/claims_test.go services/iam/internal/adapters/jwt/ services/iam/internal/adapters/grpc/iam_handler.go
git commit -m "feat(iam): propagate must_change_password into access-token claims"
```

### Task A4: iam ChangePassword derives subject from the access token

- [ ] **Step 1: Failing test**

Update `services/iam/internal/usecase/auth_change_password_test.go`: `ChangePassword` now takes `(ctx, accessToken, current, new)` and derives the user id via the token validator. Adjust the fakes: add a `TokenValidator` fake that returns a fixed subject for a known token, and assert: valid token + correct current → updates; valid token + wrong current → Unauthorized; invalid token → Unauthorized; short new → Invalid. Use neutral literals (`"test-owner-pw"`, `"test-new-pw-123"`), never a real-looking password, and `owner@example.com` for emails.

- [ ] **Step 2: Run → fail.**

- [ ] **Step 3: Implement**

In `services/iam/internal/usecase/auth.go`, change `ChangePassword` to resolve the subject from the token first:
```go
// ChangePassword sets a new password for the principal identified by accessToken
// (subject derived from the verified token — never a caller-supplied id), after
// verifying the current password. Clears must_change_password.
func (s *AuthService) ChangePassword(ctx context.Context, accessToken, currentPassword, newPassword string) error {
	if len(newPassword) < minPasswordLen {
		return glerrors.New(glerrors.KindInvalid, "new password is too short")
	}
	claims, err := s.validator.ValidateAccessToken(ctx, accessToken)
	if err != nil {
		return glerrors.New(glerrors.KindUnauthorized, "invalid access token")
	}
	_, hash, err := s.users.FindAuthByID(ctx, claims.Subject)
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
	return s.users.UpdatePassword(ctx, claims.Subject, newHash)
}
```
(`s.validator` already exists on `AuthService`.) Update the grpc handler `ChangePassword` to pass `req.Msg.GetAccessToken()` instead of `GetUserId()`:
```go
func (h *IAMHandler) ChangePassword(ctx context.Context, req *connect.Request[iamv1.ChangePasswordRequest]) (*connect.Response[iamv1.ChangePasswordResponse], error) {
	if err := h.auth.ChangePassword(ctx, req.Msg.GetAccessToken(), req.Msg.GetCurrentPassword(), req.Msg.GetNewPassword()); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&iamv1.ChangePasswordResponse{}), nil
}
```

- [ ] **Step 4: Run → pass** (`cd services/iam && go test ./...`).

- [ ] **Step 5: Commit**
```bash
git add services/iam/internal/usecase/auth.go services/iam/internal/usecase/auth_change_password_test.go services/iam/internal/adapters/grpc/iam_handler.go
git commit -m "feat(iam): ChangePassword derives subject from the access token"
```

### Task A5: Gateway carries the flag + enforces rotation

- [ ] **Step 1: Failing tests**

In `services/gateway/internal/middleware/auth_test.go` add:
- a test that `ValidateToken`'s fake returning `Principal{MustChangePassword:true}` causes `RequirePasswordChanged` to 403 (with a machine-readable body), and `false` passes through;
- confirm `RequireAuth` still injects the principal.

- [ ] **Step 2: Run → fail.**

- [ ] **Step 3: Implement**

In `services/gateway/internal/middleware/auth.go`, add `MustChangePassword bool` to `Principal`, and add the middleware:
```go
// RequirePasswordChanged blocks a principal that still must rotate its temporary
// password from doing anything except changing it. It MUST run after RequireAuth.
// Fail-closed: no principal ⇒ 401. Returns 403 with a stable "password_change_required"
// marker so clients can route to the change-password screen.
func RequirePasswordChanged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			unauthorized(w)
			return
		}
		if p.MustChangePassword {
			http.Error(w, "password_change_required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 4: Map the claim** — in `services/gateway/internal/client/iamauth.go` `ValidateToken`, add `MustChangePassword: claims.GetMustChangePassword(),` to the returned `Principal`.

- [ ] **Step 5: Wire enforcement** — in `services/gateway/cmd/main.go` `newMux`, apply `RequirePasswordChanged` to every authenticated route EXCEPT `/api/v1/auth/change-password` (and unauthenticated routes). Concretely wrap `/me` and `/students`:
```go
	mux.Handle("GET /api/v1/me", requireAuth(middleware.RequirePasswordChanged(http.HandlerFunc(authH.Me))))
	// change-password stays reachable while the flag is set:
	mux.Handle("POST /api/v1/auth/change-password", requireAuth(http.HandlerFunc(authH.ChangePassword)))
	...
	studentsCreate := requireAuth(middleware.RequirePasswordChanged(middleware.RequirePermission("students:create")(
		http.HandlerFunc(handler.NewStudentsHandler(studentsClient).Create))))
	mux.Handle("POST /api/v1/students", guard.Protect("students", studentsCreate))
```

- [ ] **Step 6: change-password passes the token to iam** — the gateway handler must forward the caller's access token (not a user id) to iam. In `services/gateway/internal/handler/auth.go` `ChangePassword`, extract the token the same way `RequireAuth` does and pass it. Add an exported helper in middleware for reuse:
  - In `middleware/auth.go` add:
    ```go
    // TokenFrom returns the caller's access token from the Authorization header or
    // the gl_access cookie ("" if neither). Shared with RequireAuth's sourcing.
    func TokenFrom(r *http.Request) string {
    	token := bearerToken(r)
    	if token == "" {
    		if c, err := r.Cookie("gl_access"); err == nil {
    			token = c.Value
    		}
    	}
    	return token
    }
    ```
    and refactor `RequireAuth` to call `TokenFrom(r)`.
  - Change `client.AuthClient.ChangePassword` signature to `ChangePassword(ctx, accessToken, currentPassword, newPassword)` and send `AccessToken` in the proto request. Update the `LoginService` interface and the handler to pass `middleware.TokenFrom(r)`:
    ```go
    if err := h.auth.ChangePassword(r.Context(), middleware.TokenFrom(r), req.CurrentPassword, req.NewPassword); err != nil {
    ```
  - Update the handler fake in `auth_test.go` accordingly.

- [ ] **Step 7: Run all gateway tests** — `cd services/gateway && go test ./...` → pass.

- [ ] **Step 8: Integration smoke (real stack)** — bring up the stack (`docker compose up -d gateway`, ensure `.env` has `OWNER_TEMP_PASSWORD`), then:
  - login → cookie set, `mustChangePassword:true`;
  - `GET /me` with that cookie → **403 `password_change_required`** (enforcement works);
  - `POST /auth/change-password` with cookie + current/new → 204;
  - login with new password → `mustChangePassword:false`; `GET /me` → 200.
  Record outputs. Tear down.

- [ ] **Step 9: Commit**
```bash
git add services/gateway
git commit -m "feat(gateway): enforce forced password rotation via claim; change-password by token"
```

**Checkpoint A:** forced rotation is enforced server-side; `user_id` is gone from the wire.

---

# Phase B — Cookie/token lifecycle (#3)

**Goal:** Access + refresh tokens are delivered ONLY as HttpOnly cookies (never in the JSON body). Refresh reads the refresh cookie and rotates BOTH cookies. Logout clears BOTH.

**Files**
- Modify: `services/gateway/internal/handler/auth.go`
- Test: `services/gateway/internal/handler/auth_test.go`
- Modify: `services/gateway/cmd/main.go` (refresh route unchanged; ensure cookie paths)
- Modify: `apps/web/app/auth/api.ts`, `apps/web/app/auth/api.test.ts`

Cookie conventions (constants at top of `handler/auth.go`):
```go
const (
	accessCookie  = "gl_access"
	refreshCookie = "gl_refresh"
	refreshPath   = "/api/v1/auth" // refresh cookie only sent to auth endpoints
)
```

### Task B1: Login sets both cookies, no tokens in body

- [ ] **Step 1: Update the failing test** — `TestLoginResponseIncludesMustChange` must now assert: body has `mustChangePassword` + `expiresIn` but **no** `accessToken`/`refreshToken`; response sets HttpOnly `gl_access` AND HttpOnly `gl_refresh` (path `/api/v1/auth`).

- [ ] **Step 2: Run → fail.**

- [ ] **Step 3: Implement** — change `loginResponse` to:
```go
type loginResponse struct {
	ExpiresIn          int64 `json:"expiresIn"`
	MustChangePassword bool  `json:"mustChangePassword"`
}
```
Add a `setAuthCookies(w, res)` helper that sets `gl_access` (Path `/`, MaxAge `ExpiresIn`) and `gl_refresh` (Path `refreshPath`, `HttpOnly/Secure/SameSite=Strict`, a longer MaxAge — use the refresh TTL surfaced by iam if available, else a fixed `refreshMaxAge = 24*3600`). `Login` calls it and writes the trimmed body.

- [ ] **Step 4: Run → pass.**

### Task B2: Refresh reads the cookie and rotates both

- [ ] **Step 1: Failing test** — refresh with a `gl_refresh` cookie → 200, sets new `gl_access` + `gl_refresh`, body has no tokens; missing cookie → 401.

- [ ] **Step 2: Run → fail.**

- [ ] **Step 3: Implement** — `Refresh` reads the refresh token from the `gl_refresh` cookie (not the body), calls `h.auth.Refresh`, calls `setAuthCookies`, and returns `{expiresIn}` only. Remove `refreshRequest`. Missing cookie → 401.

### Task B3: Logout clears both cookies

- [ ] **Step 1: Failing test** — logout sets deletion cookies (`MaxAge<0`) for BOTH `gl_access` and `gl_refresh` (matching paths).

- [ ] **Step 2–3: Implement** — `Logout` emits two deletion cookies (access on `/`, refresh on `refreshPath`).

### Task B4: Web client drops token handling

- [ ] **Step 1: Update `apps/web/app/auth/api.test.ts`** — `login` returns `{expiresIn, mustChangePassword}` (no tokens); `refresh` (if present) posts with `credentials:"include"` and no body-token dependence. Keep `changePassword`/`logout` as-is.

- [ ] **Step 2–3: Implement** — in `apps/web/app/auth/api.ts`, change `LoginResult` to `{ expiresIn: number; mustChangePassword: boolean }`; `login` returns that; the app relies on cookies (`credentials:"include"`) for subsequent calls. `AppShell` already reads only `mustChangePassword` — verify no compile break.

- [ ] **Step 4: Verify** — `cd services/gateway && go test ./...`; `pnpm --filter @garcia-lorca/web test && pnpm --filter @garcia-lorca/web exec tsc --noEmit && pnpm --filter @garcia-lorca/web lint`.

- [ ] **Step 5: E2E smoke** — login (no tokens in body; two Set-Cookie), refresh (rotates cookies), logout (clears both). Record outputs.

- [ ] **Step 6: Commit**
```bash
git add services/gateway apps/web/app/auth
git commit -m "feat(auth): tokens only in HttpOnly cookies; refresh rotates; logout clears both"
```

**Checkpoint B.**

---

# Phase C — Edge hardening: rate-limit + password validation (#5)

**Goal:** `POST /api/v1/auth/login` is rate-limited per client IP (in-process token bucket); the gateway validates `newPassword` at the edge before calling iam. No measurable latency regression on the hot path.

**Files**
- Create: `services/gateway/internal/middleware/ratelimit.go`
- Test: `services/gateway/internal/middleware/ratelimit_test.go`
- Bench: `services/gateway/internal/middleware/ratelimit_bench_test.go`
- Modify: `services/gateway/internal/handler/auth.go` (newPassword validation)
- Test: `services/gateway/internal/handler/auth_test.go`
- Modify: `services/gateway/cmd/main.go` (wire limiter) + `internal/config` (env knobs)

### Task C1: Token-bucket rate limiter

- [ ] **Step 1: Failing test** (`ratelimit_test.go`) — a limiter with capacity 3 / refill 1-per-second (injectable clock): first 3 requests from IP `1.2.3.4` pass, 4th → 429 with a `Retry-After` header; a different IP is independent; after advancing the clock one refill interval, one more request passes. Use an injected `now func() time.Time` for determinism (no real sleeps).

- [ ] **Step 2: Run → fail.**

- [ ] **Step 3: Implement** `RateLimit`:
```go
// RateLimit is a per-key (client IP) token-bucket limiter for a single instance.
// It is in-process (no shared state across instances) — sufficient for the MVP
// gateway (min-instances=1). O(1) per request under a sharded mutex; stale
// buckets are evicted lazily. Fail-open only on a missing key is NOT allowed:
// an unparseable client IP is treated as a single shared bucket (fail-closed-ish).
```
Design: `type RateLimiter struct { mu sync.Mutex; buckets map[string]*bucket; capacity float64; refillPerSec float64; now func() time.Time }`; `bucket{tokens float64; last time.Time}`. `Allow(key) (ok bool, retryAfter time.Duration)`: refill by elapsed*refillPerSec capped at capacity, take 1 if available. Provide `Middleware(next http.Handler) http.Handler` that keys on `clientIP(r)` (parse `r.RemoteAddr`; when `TRUST_PROXY` is set, key on the **right-most** `X-Forwarded-For` hop — the IP the single trusted proxy observed; the left-most is client-spoofable — and validate it parses as an IP, else fall back to `RemoteAddr`; default off ⇒ `RemoteAddr`), sets `Retry-After` and returns 429 on deny. Add a background-free lazy eviction: on each `Allow`, if `len(buckets)` exceeds a cap, drop buckets untouched for > window. Keep it simple and allocation-light.

- [ ] **Step 4: Run → pass.**

- [ ] **Step 5: Benchmark** (`ratelimit_bench_test.go`) — `BenchmarkRateLimiterAllow` hammering one key and many keys; assert (by inspection in the task report) it is allocation-free in steady state (`-benchmem` shows 0 allocs/op for the hot path) and sub-microsecond. Record `go test -bench=. -benchmem ./internal/middleware/` output.

### Task C2: Wire the limiter + edge password validation

- [ ] **Step 1: Config knobs** — in `services/gateway/internal/config`, add parse helpers for `LOGIN_RATELIMIT_BURST` (default 10) and `LOGIN_RATELIMIT_PER_MINUTE` (default 30), with tests.

- [ ] **Step 2: Wire** — in `newMux`, build the limiter from config and wrap the login route: `mux.Handle("POST /api/v1/auth/login", limiter.Middleware(http.HandlerFunc(authH.Login)))`. (Only login for now, per decision.)

- [ ] **Step 3: Edge password validation** — failing test: `ChangePassword` with an empty/short `newPassword` → 400 without calling iam (assert the fake iam is not invoked). Implement a guard in the handler:
```go
const minNewPasswordLen = 8
...
	if len(req.NewPassword) < minNewPasswordLen {
		http.Error(w, "new password is too short", http.StatusBadRequest)
		return
	}
```
(iam still enforces server-side; this is defense-in-depth + avoids a needless round-trip.)

- [ ] **Step 4: Verify** — gateway `go test ./...` (incl. bench compiles), `go vet`, `gofmt`.

- [ ] **Step 5: E2E smoke** — hammer `/auth/login` past the burst → observe 429 + `Retry-After`; a valid login within budget still works. Record output. Tear down.

- [ ] **Step 6: Commit**
```bash
git add services/gateway
git commit -m "feat(gateway): per-IP login rate limit + edge newPassword validation"
```

**Checkpoint C.**

---

# Phase D — `pkg/authctx` consolidation onto `app.current_tenant` (#4)

**Goal:** One security-critical RLS primitive (`pkg/authctx`) binds `app.current_tenant`; iam and students postgres adapters use it instead of their own `withTenant`. Behavior identical; RLS isolation tests still pass.

**Files**
- Modify: `pkg/authctx/authctx.go`, `pkg/authctx/authctx_test.go` (and `principal.go` if the rename touches it)
- Create: `services/iam/internal/adapters/postgres/txrunner.go` (GORM adapter for `authctx.TxRunner`/`Tx`)
- Modify: `services/iam/internal/adapters/postgres/repository.go` (use authctx)
- Modify: `services/students/internal/adapters/postgres/repository.go` (+ its own txrunner adapter)
- Integration tests for both must stay green.

### Task D1: Repoint authctx to the tenant var

- [ ] **Step 1: Update tests** — in `pkg/authctx/authctx_test.go`, change expectations to the new API: `WithTenantContext(ctx, runner, tenantID, fn)` issuing `set_config('app.current_tenant', <tenantID>, true)`; empty tenantID rejected (fail-closed); nil runner/callback rejected.

- [ ] **Step 2: Run → fail.**

- [ ] **Step 3: Implement** — in `pkg/authctx/authctx.go`: rename `sessionVar` value to `app.current_tenant`; rename `WithUserContext` → `WithTenantContext` with param `tenantID string` (keep the same `set_config(?, ?, true)` bound-parameter form and the empty-value fail-closed guard); update doc comments (RLS tenant isolation). Keep `SessionVar()` returning the new name. Update `principal.go` only if it referenced the old name.

- [ ] **Step 4: Run → pass** (`go test ./pkg/authctx/...`).

### Task D2: iam adapter uses authctx

- [ ] **Step 1: GORM TxRunner adapter** — create `services/iam/internal/adapters/postgres/txrunner.go`:
```go
package postgres

import (
	"context"

	"gorm.io/gorm"

	"github.com/garcia-lorca/pkg/authctx"
)

// gormTxRunner adapts *gorm.DB to authctx.TxRunner/Tx so the shared RLS helper
// can bind app.current_tenant without importing GORM.
type gormTxRunner struct{ db *gorm.DB }

func (r gormTxRunner) Transaction(ctx context.Context, fn func(authctx.Tx) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(gormTx{tx})
	})
}

type gormTx struct{ tx *gorm.DB }

func (t gormTx) Exec(ctx context.Context, sql string, args ...any) error {
	return t.tx.WithContext(ctx).Exec(sql, args...).Error
}

// txGorm recovers the *gorm.DB inside an authctx callback (the repo methods run
// their queries on it).
func txGorm(tx authctx.Tx) *gorm.DB { return tx.(gormTx).tx }
```

- [ ] **Step 2: Replace `withTenant`** — in `repository.go`, replace the private `withTenant` body so it delegates to the shared helper (keep the method name + signature so callers don't churn):
```go
func (b base) withTenant(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return authctx.WithTenantContext(ctx, gormTxRunner{b.db}, b.tenantID, func(tx authctx.Tx) error {
		return fn(txGorm(tx))
	})
}
```
Remove the old inline `set_config` + `glerrors.Wrap`. Keep imports tidy.

- [ ] **Step 3: Run** — `cd services/iam && go test ./... && go test -tags=integration ./internal/adapters/postgres/` → all pass, including `TestRLSIsolatesTenants` (proves the binding still isolates tenants through the shared helper).

### Task D3: students adapter uses authctx

- [ ] **Step 1** — add the same `txrunner.go` adapter to `services/students/internal/adapters/postgres/` and repoint its `withTenant` to `authctx.WithTenantContext` (mirror D2, matching the students repo's `base`/`Repository` shape — inspect it first).

- [ ] **Step 2: Run** — `cd services/students && go test ./... && go test -tags=integration ./internal/adapters/postgres/` (if it has integration tests) → pass; RLS isolation still green.

- [ ] **Step 3: Commit**
```bash
git add pkg/authctx services/iam/internal/adapters/postgres services/students/internal/adapters/postgres
git commit -m "refactor(rls): consolidate tenant binding in pkg/authctx (app.current_tenant)"
```

**Checkpoint D.**

---

# Phase E — Owner-secret stub + docs (#2, #6)

**Goal:** Document + stub how shared envs source `OWNER_TEMP_PASSWORD` from a secret manager (local path unchanged); fix the stale iam RLS doc.

**Files**
- Modify: `deploy/terraform/secrets.tf` (owner-temp-password secret stub, var-gated)
- Modify: `deploy/terraform/variables.tf` / `environments/*.tfvars.example` (toggle)
- Create/Modify: a deploy runbook note (e.g. `deploy/README.md` or `docs/`), and `.env.example` comment
- Modify: `services/iam/CLAUDE.md`

### Task E1: Terraform secret stub + docs (#2)

- [ ] **Step 1: Terraform stub** — in `deploy/terraform/secrets.tf`, add a `google_secret_manager_secret` + version resource for `owner-temp-password`, gated behind a variable (e.g. `count = var.seed_owner_secret ? 1 : 0`) so it is inert until a shared env opts in. Add the variable (default `false`) in `variables.tf`. Reference how Cloud Run would inject it as `OWNER_TEMP_PASSWORD` (the `migrate`/seed step) — in a comment or the services module, without wiring a live deploy. `terraform fmt` + `terraform validate` (if terraform is available; else note it was skipped and the HCL is syntactically reviewed).

- [ ] **Step 2: Docs** — document the sourcing in a short runbook section (deploy README or `docs/`): local uses `.env` `OWNER_TEMP_PASSWORD`; shared envs source it from Secret Manager → Cloud Run env → the seed step; the value is a one-time temp password (owner rotates at first login). Update the `.env.example` comment to point at the Terraform var for shared envs.

- [ ] **Step 3: Commit**
```bash
git add deploy docs .env.example
git commit -m "docs(deploy): stub + document owner-temp-password sourcing from Secret Manager"
```

### Task E2: Fix the iam RLS doc (#6)

- [ ] **Step 1: Edit `services/iam/CLAUDE.md`** — replace the stale RLS lines (currently: "All access goes through `pkg/authctx.WithUserContext`, which sets `app.current_user` via `SET LOCAL`...") with the accurate description after Phase D:
  > All access to RLS tables goes through `pkg/authctx.WithTenantContext`, which binds `app.current_tenant` via `set_config(..., true)` (SET LOCAL semantics) inside the transaction. Policies filter on `current_setting('app.current_tenant')`.

- [ ] **Step 2: Commit**
```bash
git add services/iam/CLAUDE.md
git commit -m "docs(iam): correct RLS description to app.current_tenant via authctx"
```

**Checkpoint E.**

---

## Final gates (run after all phases merge; required before any shared-env flag enablement)

- [ ] **Full suite:** `make test` (Go `-race` + web) and integration suites for iam + students (`-tags=integration`) — all green.
- [ ] **Contract:** `buf lint` + `buf breaking` vs `master` (expect the one intentional break: `ChangePasswordRequest.user_id` removal — documented).
- [ ] **Security gate (security agent):** RLS isolation audit (both services via the shared helper), OWASP pass on the new auth surface (rate-limit bypass, cookie flags, enforcement can't be skipped, no token in body/logs, change-password IDOR closed), `govulncheck ./...`, dependency audit, secret scan (`git grep` guard) clean.
- [ ] **Performance gate:** `go test -bench=. -benchmem ./services/gateway/internal/middleware/` — rate limiter O(1), allocation-free steady state; token-validate path shows no regression; confirm the limiter's map eviction bounds memory under many distinct IPs (a bench with 100k keys stays bounded).
- [ ] **Tech-lead gate:** canon review of the full diff.

## Testing strategy recap
Unit (domain/usecase/middleware/handlers), integration (testcontainers, RLS isolation per service), contract (endpoints vs OpenAPI/proto), web (Vitest/RTL), benchmarks (rate limiter), flag-aware where relevant, and every fixed bug leaves a regression test. Deterministic only — injected clocks for the limiter and token expiry, no real sleeps.
