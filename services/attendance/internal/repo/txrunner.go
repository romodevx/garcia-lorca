package repo

import (
	"context"

	"gorm.io/gorm"

	"github.com/garcia-lorca/pkg/authctx"
)

// gormTxRunner adapts *gorm.DB to authctx.TxRunner/Tx so the shared RLS helper
// can bind app.current_tenant without importing GORM.
type gormTxRunner struct{ db *gorm.DB }

func (r gormTxRunner) Transaction(ctx context.Context, fn func(authctx.Tx) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(gormTx{tx})
	})
}

type gormTx struct{ tx *gorm.DB }

func (t gormTx) Exec(ctx context.Context, sql string, args ...any) error {
	return t.tx.WithContext(ctx).Exec(sql, args...).Error
}

// txGorm recovers the *gorm.DB inside an authctx callback (the repository methods
// run their queries on it).
func txGorm(tx authctx.Tx) *gorm.DB { return tx.(gormTx).tx }
