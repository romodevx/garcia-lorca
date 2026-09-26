// Package repo is the GORM-backed persistence layer for attendance. GORM lives
// ONLY here; it binds transactions via pkg/authctx for RLS on sensitive tables.
//
// A double model (the GORM struct in models.go ↔ the service's AttendanceRecord
// via mapper.go) keeps the application layer framework-free: it depends on the
// service.Repo interface it declares itself, never on this package.
//
// Only exceptions are stored (default-present), so a list's save is a RECONCILE:
// ReconcileList loads the list's marks, applies the service's diff (upsert in
// place, delete what was cleared) and records the list's session inside ONE
// tenant-bound transaction — a partially applied list would misreport who was
// present.
package repo
