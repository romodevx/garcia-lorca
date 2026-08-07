package gormtx

import (
	"testing"

	"gorm.io/gorm"

	"github.com/garcia-lorca/pkg/authctx"
)

// TestNewRunnerSatisfiesTxRunner pins the contract the services depend on: the
// adapter is usable as the runner argument of authctx.WithTenantContext.
func TestNewRunnerSatisfiesTxRunner(t *testing.T) {
	var runner authctx.TxRunner = NewRunner(&gorm.DB{})
	if runner == nil {
		t.Fatal("NewRunner returned a nil authctx.TxRunner")
	}
}

// TestGormRecoversTheHandle proves the round trip repositories rely on: whatever
// *gorm.DB the transaction callback is given comes back out of Gorm unchanged,
// so queries run on the transaction that has the RLS tenant bound.
func TestGormRecoversTheHandle(t *testing.T) {
	db := &gorm.DB{}

	var tx authctx.Tx = gormTx{db}

	if got := Gorm(tx); got != db {
		t.Fatalf("Gorm returned %p, want the transaction handle %p", got, db)
	}
}
