# `(admin)` route group

Administrative surface (school staff / direction). Routes here are gated per
module via `FeatureGate` / `useFlag` and lazy-loaded so disabled module chunks
are never downloaded.

FASE 0: empty stub. Module routes (students, finance, grades, etc.) are added by
their respective module cycles. A disabled module must not register its route.
