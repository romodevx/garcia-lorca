# ADR 0005 — refresh-token revocation on password change

- Status: Accepted
- Date: 2026-08-12
- Context: Security review carry-forward. iam issues stateless JWT refresh tokens
  with a 24h TTL. Because they are stateless, a refresh token stolen from a user
  stayed valid for its full 24h **even after the user changed their password** —
  changing the password did nothing to the attacker's session. We need a
  password change to revoke previously issued refresh tokens, without adopting a
  full server-side session store (stateful sessions) and without logging the
  legitimate user out at the very moment they change their password.

## Decision

Add a per-user **`password_changed_at`** timestamp (migration
`iam/0011`). It is the revocation floor:

- **iam `RefreshToken`** rejects any refresh token whose `iat` (issued-at, already
  embedded in the JWT) predates the user's `password_changed_at`. So every
  refresh token minted before the last password change is dead — the stolen
  token stops working the instant the password is rotated. `ParseRefreshToken`
  now returns the token's issued-at alongside the subject.
- **iam `UpdatePassword`** stamps `password_changed_at = now()` atomically with
  the new hash and the cleared `must_change_password` flag.
- **No user logout on change.** The change also revokes the *legitimate* current
  refresh token, so the **gateway** `ChangePassword` handler, immediately after a
  successful change, re-issues a fresh session by logging in with the verified
  principal email + the new password, and sets fresh `gl_access` / `gl_refresh`
  cookies **in the same response**. The web client no longer fires a follow-up
  refresh (which would now fail against the revoked token) — it lands straight on
  the dashboard.

The `password_changed_at` value only flows one way into the domain (hydrated on
load via `User.WithPasswordChangedAt`); `NewUser`'s creation signature is
unchanged. A user constructed in memory carries the zero instant, which imposes
no floor (any `iat` is after the zero time), so only DB-backed users with a real
rotation timestamp enforce the cutoff.

## Why not a stateful refresh-token store

A server-side store (persist every issued refresh token, check/revoke on use) is
the textbook answer and enables per-token revocation and reuse detection. It is
also a new stateful subsystem: a table, writes on every login/refresh, cleanup
of expired rows, and a read on every refresh. For the one requirement we have
now — *a password change kills old refresh tokens* — a single `password_changed_at`
column gives fail-closed revocation with zero per-refresh writes and one column
read we already do (the user is loaded in `RefreshToken` anyway). Reuse
detection / per-device revocation, if ever needed, is a future ADR that can add
the store without undoing this.

## Why re-login at the gateway (not change the ChangePassword contract)

The alternative is to make iam's `ChangePassword` RPC return a fresh token pair.
That changes the proto contract (and `gen/`), the grpc handler, and every caller.
Re-issuing at the gateway keeps the iam contract untouched: the gateway already
holds the verified principal (email) and the new password from the request body,
and it already owns cookie management. A rare re-login failure right after a
successful change degrades gracefully to `204` (password changed, no fresh
cookies) and the client falls back to the login screen.

## Consequences

- A stolen refresh token is neutralized the moment the victim changes their
  password (previously: up to 24h of continued access).
- Access tokens (15 min) are intentionally NOT version-checked — their short TTL
  bounds the exposure and a per-request DB check would tax every authorized call.
  The revocation acts at the refresh boundary, which is where longevity lives.
- Suspending a user already blocks refresh (`CanAuthenticate`); this adds the
  password-change axis. Neither yet covers "revoke a specific device now" — that
  needs the stateful store above.

## Amendment 2026-10-17 — credential version (CC-302)

The whole-second floor left a hole: every refresh token minted in the change's
own second survived it, so a stolen token rotated more than once a second always
held one and outlived every password change. `users.credential_version`
(iam/0032) is bumped by every write that stamps `password_changed_at`, the
refresh JWT carries it as `cv`, and `RefreshToken` honors a token only under the
CURRENT version (`domain.RefreshClaims.RevokedFor`). A counter, not a finer
clock: `password_changed_at` is the database's `now()` and `iat` the app's clock,
and a millisecond comparison between the two would log the re-issued session out
on any skew. The second floor stays for tokens minted before `cv` existed (they
read as version 0, the default of every row), so the first rotation after the
deploy revokes those exactly too.
