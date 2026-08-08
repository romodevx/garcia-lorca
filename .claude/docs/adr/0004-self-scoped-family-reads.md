# ADR 0004 — self-scoped family reads (the `/api/v1/family/*` pattern)

- Status: Accepted
- Date: 2026-08-07
- Context: Phase 9b. Guardians (family) must read their own children's data
  (notifications now; grades/attendance/account later) from the mobile app. The
  admin-side reads (`comms:read`, `finance:read`, `attendance:read`) are
  tenant-wide and `IsOwnerOnly`-guarded — granting them to a `guardian` role
  would expose every family's data. We need a read path that is scoped to the
  caller's own children without inventing per-object filtering in each service.

## Decision

Family reads go through **self-scoped gateway endpoints** under `/api/v1/family/*`,
gated by a dedicated **`family:read`** permission (held by the `guardian` role;
NOT one of the tenant-wide `*:read` perms). The endpoint derives the child set
from the **authenticated principal**, never from the request:

```
principal.UserID  ──> students.ListStudentsForGuardian(userID)  ──> child set
                  ──> for each child: <module>.ReadForStudent(childID)  ──> aggregate
```

The caller cannot name a student; any `?studentId=`/`?guardianUserId=` in the
request is ignored (pinned by a test). So the endpoint is **scoped by
construction**, not by a filter that could be forgotten. The underlying services
keep their owner-only admin permissions unchanged — the gateway calls them
server-to-server with the resolved child ids.

## Why not grant guardians the tenant-wide reads + per-object filtering

That would require every read endpoint (comms/finance/attendance/grades) to learn
"who are the caller's children", threading identity + an ownership model into each
service — and one missed filter leaks a family's data. The self-scoped edge keeps
the ownership resolution in ONE place (the gateway, which already holds the
principal) and keeps `IsOwnerOnly` closed. `guardians:read/write` are also
`IsOwnerOnly` because they compose into this read (self-link → /family/*).

## Consequences

- **Every future family read copies this shape:** `family:read`, principal →
  `ListStudentsForGuardian` → per-child fan-out → aggregate. Do NOT add a
  `studentId` request parameter to a `/family/*` route.
- Per-child fan-out is sequential and bounded by family size (acceptable). Batch
  only if a future family read fans out per child *per module*.
- The admin path (`/api/v1/finance|attendance|comms/...` with the tenant-wide
  `*:read`) stays owner-only until real per-object scoping lands there
  independently — the family path does not unblock it.
- Push targeting mirrors the same ownership resolution (comms →
  `ListGuardiansForStudent` → the student's guardians' devices).
- Guardian linking (`guardians:write`) is the artifact this authorizes on, so it
  is owner-only + guarded; a guardian self-service invite/claim flow is deferred.
