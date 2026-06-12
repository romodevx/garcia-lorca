// Package postgres is the GORM-backed persistence adapter for billing. GORM lives ONLY here. It maps between domain entities and GORM models (double model + mapper) and binds every transaction to the caller via pkg/authctx for RLS.
package postgres
