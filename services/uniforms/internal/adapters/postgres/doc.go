// Package postgres is the uniform shop's persistence adapter. GORM lives here
// and nowhere else: the domain never sees a tag, a column or a transaction. Every
// access is tenant-scoped for RLS, binding app.current_tenant per transaction
// through pkg/authctx, the same posture as students and finance (ADR 0001).
package postgres
