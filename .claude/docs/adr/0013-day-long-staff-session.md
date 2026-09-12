# ADR 0013 — the panel's session lasts a day, and a failed refresh does not end it

- Status: Accepted
- Date: 2026-09-12
- Context: Two complaints from the owner, which turned out to be one symptom with
  two causes. (1) "Quiero mantener la sesión por 24 horas, y que si se cierra la
  pestaña o el navegador y se vuelve a abrir, la sesión siga." (2) "Siento que
  cuando dos perfiles se loguean, a uno lo desloguea."

## What was actually happening

The panel's session was a **fifteen-minute idle window**, enforced in three
places that had to agree: `staffRefreshTTL` in iam, the `gl_refresh` cookie's
`Max-Age` at the gateway, and a watchdog in the browser that called `logout()`.
It was the school's own rule, for the shared computer at the front desk.

The second complaint was **not** the cookie jar and not a shared session slot.
Sessions are stateless JWTs with no server-side store, so nothing one session
can do invalidates another; that was verified against the running stack with two
users, concurrent logins, cross-refreshes and 40 interleaved requests — every
one answered 200. The real cause was a pair of conflations:

- **iam** `RefreshToken` flattened every `FindByID` error into "invalid refresh
  token". The repository already distinguished them (`translateFindErr` returns
  `ErrNotFound` for a missing row, `KindInternal` for a query that failed), and
  the use case threw that away. A database that was merely busy answered 401.
- **The web** read any non-200 from `/auth/refresh` as "you are signed out"
  (`res.ok`, with `.catch(() => false)`), so a 500, a 429 or a dropped
  connection announced a lost session and cleared it.

Together: a transient failure during a rotation permanently ended a valid
session. It correlated with how many people were working — every active session
rotated on a timer, each rotation drew against `DB_MAX_OPEN_CONNS = 2` per
instance on a `db-f1-micro` — which is exactly why it read as *other people's
logins ending yours*.

## Decision

**One.** `staffRefreshTTL` becomes **24 h**, matched by the `gl_refresh` cookie's
`Max-Age` (86400). The window is **sliding**: every use of the panel rotates the
token, so somebody who works in it daily is never asked for a password again. A
positive `Max-Age` is also what makes the cookie persistent rather than one the
browser discards on exit — that is the half that answers "cerré la pestaña y me
sacó".

**Two.** The browser's idle watchdog is **removed**. `useSessionTimeout` becomes
`useSessionKeepAlive`: it still renews an active session every ten minutes so a
cashier entering payments never meets a login screen mid-entry, and it no longer
has a counterpart that ends the session. The server is the only authority on
when a session is over.

**Three.** A **failure is not a refusal**, expressed in both places that got it
wrong:

- iam's `RefreshToken` propagates anything that is not `KindNotFound`, so a
  broken database becomes a 500.
- the web's `authFetch` returns a three-valued `RefreshOutcome`
  (`rotated` | `refused` | `failed`); only `refused` (401/403) announces a lost
  session. `api.ts` rejects with an `HttpError` carrying the status so callers
  can tell them apart, and the boot-time session restore retries a failure three
  times with backoff instead of obeying it.

The `SessionKind` type stays even though both kinds now hold the same number: it
is the seam the two surfaces diverge along, and they already did once.

## Consequences

- **Accepted cost, chosen by the owner:** on the shared front-desk computer, a
  session left open stays open for whoever sits down next. "Cerrar sesión" is
  the only remaining defense there. The school weighed this against being thrown
  out mid-shift and chose the day.
- Sessions now rotate far less often (one keep-alive per ten active minutes,
  against a day-long window instead of a fifteen-minute one), which
  independently reduces pressure on the connection pool.
- **Not addressed here:** `DB_MAX_OPEN_CONNS = 2` for every service on a
  `db-f1-micro` is the underlying contention. Failed rotations are now retried
  instead of ending sessions, but the contention remains. Raising it is an
  infrastructure decision with its own arithmetic (~11 services × 2 against ~25
  total connections) and belongs in its own ADR.
