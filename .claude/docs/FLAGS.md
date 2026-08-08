# FLAGS.md — Feature-Flag Registry · Sistema de Gestión Escolar García Lorca

Every module is born behind a feature flag, **default OFF**. The flag is the
**presentation boundary** (show only the modules I choose) and the **security
boundary** (server-side: a disabled module's routes return 404; frontend checks
are UX-only via a single `useFlag` hook / `FlagsProvider`).

## Rules
- Flag keys are `snake_case`, form `module_<name>`; registered in a migration.
- **Default OFF**, everywhere. Enabling a flag in staging/production requires the
  module's **tech-lead gate + security gate** to have passed.
- Server-side enforcement is authoritative; disabled route → 404.
- Permanently-GA modules: schedule a tech-debt task to remove the dead flag.
- `M00` (Cimientos) and `S01/S02` (spikes) have **no flag** — infra/research, not shippable modules.

## Flag registry

| Flag key | Module | Microservice | Default | Description |
|----------|--------|--------------|---------|-------------|
| `module_iam` | M01 IAM | iam | **OFF** | Auth, JWT, roles & permissions. Spine — gate carefully; everything depends on it. |
| `module_gateway` | M02 Gateway/BFF | gateway | **OFF** | Connect edge + the flag-enforcement middleware itself + dashboard read-model plumbing. |
| `module_students` | M03 Students/Admisión | students | **OFF** | Student control & admission. First vertical slice. |
| `module_config` | M04 Cycles/Config | config | **OFF** | School-cycle lifecycle & per-cycle configuration. **Backed by a real `config` service since Phase 2 (academic catalog: cycles/levels/grade_levels/subjects/terms/groups).** Gateway routes require `config:read`/`config:write`. |
| `module_finance` | M05 Finance/Adeudos | finance | **OFF** | Debts & payment ledger. **Backed by a real `finance` service since Phase 7 (finance core: dynamic charge concepts, charges single + bulk-per-cycle/group, window payments cash/card/transfer + reference, discounts, account statement, overdue). Money is integer cents; RLS on concepts/charges/payments; atomic payment knot. Emits `PaymentRegistered`.** Gateway routes require `finance:read`/`finance:write`. Scholarships (M08) + CONTPAQi export (S02) are Phase 7b (the `ExportContpaqi` RPC is a stub). |
| `module_attendance` | M06 Attendance | attendance | **OFF** | Daily attendance. **Backed by a real thin `attendance` service since Phase 8 (per-group daily pass list, default-present — stores only exceptions; reconcile-on-save; emits `AttendanceMarked`/`AbsenceRecorded`). RLS on `attendance`; attendance→students+config by RPC.** Gateway requires `attendance:read`/`attendance:write`. QR check-in deferred to Phase 8b. |
| `module_grades` | M07 Grades/Boletas | grades | **OFF** | Grade capture & boletas (Strategy per SEP level). |
| `module_scholarships` | M08 Scholarships | scholarships | **OFF** | Scholarship application/cancellation. |
| `module_docs` | M09 Docs/Trámites | docs | **OFF** | Document & procedure requests. |
| `module_comms` | M10 Comms/Agenda/Push | comms | **OFF** | Communications & push. **Backed by a real thin `comms` service since Phase 9a (event backbone): the first Pub/Sub CONSUMER — subscribes to `AbsenceRecorded`/`PaymentRegistered`/student events, persists a Notification idempotently (`processed_events` ledger), registers FCM device tokens, pushes via a log stub (real FCM behind env). Producers now publish for real (emulator locally).** Gateway requires `comms:read`/`comms:write`. **Phase 9b: guardian targeting** (push goes to the student's guardians' devices) + a self-scoped **`family:read`** surface (`/api/v1/family/notifications`, gated by `module_comms`+`module_students`) + a seeded **`guardian` role** (holds only `family:read`). Agenda + personalized messages = 9d; Expo family app = 9c. |
| `module_billing` | M11 Billing CFDI 4.0 + IEDU | billing | **OFF** | **Out of MVP (post-MVP, future-ready).** Flag stays **registered, default OFF** as a reserved boundary; **no implementation, no PAC integration this cycle.** Proto contract kept as a frozen stub. |
| `module_uniforms` | M12 Uniforms store | uniforms | **OFF** | Uniform reservation store (State pattern). |
| `module_reports` | M13 Reports/Dashboard | gateway + read models | **OFF** | Advanced cross-module dashboard read-models. |

## Presentation presets

A **presentation preset** is a named set of flags switched on together so a demo
shows exactly the chosen modules and nothing else. Every preset must have an
**E2E smoke test**. Presets only turn ON modules whose tech-lead + security gates
have passed.

| Preset | Flags ON | Demo intent |
|--------|----------|-------------|
| `preset_spine` | `module_iam`, `module_gateway`, `module_students` | The end-to-end vertical slice: enroll a student through the gateway. |
| `preset_core_ops` | spine + `module_config`, `module_finance`, `module_attendance`, `module_grades` | Day-to-day school operation: cycles, debts, QR attendance, grades/boletas. |
| `preset_full_mvp` | core_ops + `module_scholarships`, `module_docs`, `module_comms` | Full Must+Should MVP surface. |
| `preset_finance_demo` | spine + `module_finance`, `module_scholarships` | Finance story: debts, payments, overdue, scholarships + CONTPAQi export (S02). Billing/CFDI is out of MVP. |
| `preset_family_app` | spine + `module_students`, `module_attendance`, `module_grades`, `module_comms` | Parent-facing experience: attendance, grades, push notifications. **Must keep `module_students` + `module_comms` ON together** — the `/family/*` route reads guardian links (students) to aggregate notifications (comms); the route is guarded on both. |
| `preset_academic_spine` | `module_iam`, `module_gateway`, `module_students`, `module_config`, `module_grades` | The academic spine: owner login (forced first-login password change) through grade capture. |

> Note: no preset is "all flags on by default." Demos opt in explicitly; the
> default state of every flag in every environment remains **OFF**.
