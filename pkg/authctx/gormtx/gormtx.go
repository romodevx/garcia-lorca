// Package gormtx adapts *gorm.DB to authctx.TxRunner so every service's
// repository can bind app.current_tenant through the shared RLS helper without
// re-implementing the adapter. It is the only GORM-coupled corner of pkg;
// authctx itself stays framework-free so the RLS primitive can be unit-tested
// with a fake.
package gormtx

import (
	"context"

	"gorm.io/gorm"

	"github.com/garcia-lorca/pkg/authctx"
)

// NewRunner adapts db to authctx.TxRunner, ready to hand to
// authctx.WithTenantContext.
func NewRunner(db *gorm.DB) authctx.TxRunner { return runner{db} }

// runner opens the GORM transaction the RLS helper binds the tenant inside of.
type runner struct{ db *gorm.DB }

func (r runner) Transaction(ctx context.Context, fn func(authctx.Tx) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(gormTx{tx})
	})
}

// gormTx is the transaction handle passed back to the caller. It stays
// unexported so the only way to reach the *gorm.DB is Gorm, keeping the
// tenant-bound transaction the single path to the database.
type gormTx struct{ tx *gorm.DB }

func (t gormTx) Exec(ctx context.Context, sql string, args ...any) error {
	return t.tx.WithContext(ctx).Exec(sql, args...).Error
}

// Gorm recovers the *gorm.DB inside an authctx.Tx callback; repositories run
// their queries on it so every statement sees the bound RLS tenant. It panics
// if tx did not come from this package's runner, which is a programming error.
func Gorm(tx authctx.Tx) *gorm.DB { return tx.(gormTx).tx }
