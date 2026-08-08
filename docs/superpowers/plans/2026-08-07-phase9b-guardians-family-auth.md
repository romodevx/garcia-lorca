# Phase 9b — Guardians + family auth (Bearer) + self-scoped family reads Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]` checkboxes.

**Goal:** Give families an identity and a safe read path. A **guardian** is an iam user with a `guardian` role, linked to their children via `guardian_students` (in the students service). The mobile app authenticates with a **Bearer token** (body, no cookies). Guardians read their own children's data through **self-scoped `/api/v1/family/*` endpoints** that resolve caller→children→data server-side — scoped BY CONSTRUCTION, so we never grant guardians the tenant-wide `*:read` permissions the `IsOwnerOnly` guard blocks. Comms notification **push targets the student's guardians' devices** (refines 9a's tenant-wide fanout).

**Architecture:** students gains `guardian_students` (link table, RLS) + link/list RPCs + a **students→iam** seam to validate the guardian user. The gateway gains a **mobile auth** path (`/api/v1/auth/mobile/login` + `/refresh` returning tokens in the JSON body, NO cookies; web login unchanged — cookies only, tokens never in the body) and **family** endpoints that, for the authenticated guardian, resolve their children (students `ListStudentsForGuardian`) and aggregate reads server-to-server. comms push resolves guardians per student (comms→students). iam seeds a `guardian` role + a `family:read` permission (NOT owner-only). The gateway `middleware` already accepts `Authorization: Bearer` (`TokenFrom`), so the middleware needs no change.

**Decisions (confirmed):** guardian↔student in students (`guardian_students` + relationship); mobile Bearer via a **separate login** (body tokens, no cookies) so the web's HttpOnly posture is untouched.

**Tech Stack:** Go 1.24, GORM (adapters only), Connect/gRPC, Postgres 16 + RLS, buf, testcontainers-go. (The Expo app itself is 9c.)

---

## Model & rules
- `guardian_students`: `{id, tenant_id, guardian_user_id uuid (iam), student_id uuid (students, RPC-validated), relationship text CHECK (mother|father|guardian|other), created_at}`, `UNIQUE (tenant_id, guardian_user_id, student_id)`, RLS ENABLE+FORCE. A guardian may have many students; a student many guardians.
- A guardian is an **iam user** with the `guardian` role. Linking validates the student exists (local) and the guardian user exists + holds the guardian role (students→iam RPC `GetUser`).
- **Mobile auth:** `iam.Login` already returns tokens; the gateway's mobile endpoints return them in the body (access + refresh + expiry) and set NO cookies. Web `/auth/login` unchanged. Both paths validate the same JWT the same way (middleware `TokenFrom` already reads the Bearer header).
- **Family reads are self-scoped:** `/api/v1/family/*` derives the caller's user id from the principal, calls students `ListStudentsForGuardian(callerUserID)` → the child set, and aggregates per-child data via server-to-server calls (e.g. comms `ListNotifications`). A guardian can NEVER name another family's student — the child set is derived, never taken from the request. Backed by a `family:read` permission on the `guardian` role. This is why we do NOT relax the `IsOwnerOnly` guard: guardians use self-scoped endpoints, not the admin-side tenant-wide `*:read`.
- **Push targeting:** on consume, comms resolves the student's guardians (comms→students `ListGuardiansForStudent`) → their devices → push (replaces 9a's `DevicesForTenant`). Best-effort as before.

## Sub-phases
- **9b.1 students:** `guardian_students` migration + domain + link/list RPCs + students→iam validation. (`feature/phase9b-guardians-be`)
- **9b.2 iam + gateway auth:** `guardian` role + `family:read` (iam migration); gateway mobile login/refresh (body tokens); gateway guardian-linking routes (behind `guardians:write`/owner).
- **9b.3 gateway family + comms targeting:** `/api/v1/family/notifications` (self-scoped) + comms push targets guardians (comms→students).
- **9b.4 gates + docs.**

(Full task breakdown — migration/proto/domain/usecase/adapter/handler/tests per service, mirroring prior phases — is elaborated at execution time; this plan fixes the shape + the decisions. Each new table gets RLS + isolation tests; each cross-service ref is RPC-validated; every route flag+permission-gated; TDD throughout; deterministic; neutral literals.)

## Security posture (closes carry-forwards)
- Family reads self-scoped by construction → closes the "per-object authz" spirit of finance item 14 / attendance item 20 / comms item 26 **for the family path** (the admin-side `*:read` stays owner-only + guarded; guardians never get them).
- Mobile Bearer: tokens in the body ONLY for the non-cookie mobile path; web keeps HttpOnly (no token in JS). Refresh rotation preserved. Note (carry-forward): mobile token storage is the app's secure storage (9c); consider shorter access TTL for mobile.
- Guardian linking is an admin action (owner/`guardians:write`) until a self-service invite flow exists.

## Out of scope (9c/9d)
9c: the Expo family app wired to `/api/v1/auth/mobile/*` + `/api/v1/family/*`. 9d: personalized messages + agenda. Real FCM. A guardian self-service invite/claim flow. Extending `/family/*` to grades/attendance/account (add as the app needs them).
