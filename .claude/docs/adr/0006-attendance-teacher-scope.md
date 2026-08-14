# ADR 0006 — teacher self-scoped attendance (the `/api/v1/teaching/attendance/*` surface)

- Status: Accepted
- Date: 2026-08-14
- Context: The web app serves school staff with iam roles. Taking the daily pass
  list is a maestro's job, but `attendance:read` is an `ownerOnlyPermissions`
  entry — a tenant-wide sensitive read (every group's attendance) that is
  deliberately non-grantable, so it cannot be handed to a teacher role. A maestro
  therefore has no route into the broad admin attendance endpoints. This mirrors
  the problem already solved for grade capture: teachers need a **self-scoped**
  surface, scoped by construction to the groups they teach, consistent with the
  shipped `/api/v1/teaching/*` (teaching:read/write) and family (ADR 0004)
  patterns. This ADR ratifies the human-approved decision; it does not reopen it.

## Decision

1. **New GRANTABLE permissions `attendance_own:read` / `attendance_own:write`** —
   dedicated, **not** a reuse of `teaching:read/write`. Rationale: granular
   delegation. A substitute can be granted the pass list without receiving
   grade-capture authority (or vice-versa). This cleanly mirrors the split between
   owner-only `attendance:read` (tenant-wide) and the self-scoped `attendance_own:*`.

2. **Self-scoped routes under the `teaching/*` family** (same edge as teacher grades):
   - `GET  /api/v1/teaching/groups` — the caller-teacher's groups.
   - `GET  /api/v1/teaching/attendance/roster?groupId&date` — pass list for a group·date.
   - `POST /api/v1/teaching/attendance` — save the pass list.

3. **Ownership rule:** `groupId ∈ DISTINCT(group_id of the caller-teacher's
   teaching_assignments)`. Attendance is per-group (any subject the teacher teaches
   in that group), so authorization is **not** limited to homeroom. The group set
   is derived from the authenticated principal, never from the request body.

4. **Any-date writes (no window).** Matches the attendance service model:
   exceptions stored per group·date with reconcile-on-save. There is no
   "today only" restriction; back-dating and corrections are supported.

5. **Student-name enrichment is gateway-side** via the trusted server-to-server
   `studentsClient`. The teacher holds no `students:read`, so the roster is
   hydrated at the edge with the resolved group's student ids; name enrichment is
   **best-effort** and degrades gracefully (ids without names) if students is slow
   or unavailable.

## Why dedicated `attendance_own:*` rather than reusing `teaching:read/write`

Folding attendance into `teaching:*` would couple two independently delegable
duties: anyone who could capture grades would automatically take attendance, and a
substitute who only needs the pass list would be over-granted grade authority.
Separate grantable permissions keep delegation granular and keep the mental model
symmetric — one owner-only tenant-wide permission (`attendance:read`) and one
self-scoped grantable pair (`attendance_own:*`), exactly as grades already do.

## Consequences

- **+1 iam migration:** the owner role gains `attendance_own:read/write` (via
  migration `0013`) so it can delegate them. iam itself has no enumerated
  permission catalog (validity is grammar-regex based); the web role-editor
  catalog (`apps/web/app/_screens/admin/catalog.ts`) gains the two entries in the
  frontend slice so an admin can tick them when building a Maestro role.
- **The attendance service is unchanged** — it is already group-keyed; the new
  surface is entirely at the gateway (ownership resolution + studentsClient
  enrichment), calling attendance server-to-server with the resolved group.
- **The ownership guard is the security-critical surface** and is reviewed like the
  teaching surface: IDOR (a teacher must not read/write a group they don't teach),
  no empty/spoofed `user_id`, and `groupId` re-checked against the caller's
  assignments **on write**, not only on read. Pinned by tests, same as `/teaching/*`.
- `module_attendance` stays **default OFF** until the security and tech-lead gates
  pass; disabled ⇒ these routes respond NotFound/PermissionDenied at the backend.
