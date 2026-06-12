// Package repo is the GORM-backed persistence layer for attendance. GORM lives ONLY here; it binds transactions via pkg/authctx for RLS on sensitive tables.
package repo
