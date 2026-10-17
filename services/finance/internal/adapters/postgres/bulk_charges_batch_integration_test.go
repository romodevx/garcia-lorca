//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"testing"

	"gorm.io/gorm"

	"github.com/garcia-lorca/services/finance/internal/domain"
)

// bulkRunSize is more charges than one INSERT can carry: at 18 columns a charge,
// Postgres's 65535-parameter cap stops a single statement near 3640 rows. The
// first full-school billing run (567 imported pupils, months back to September)
// crossed it and answered 500 with nothing billed.
const bulkRunSize = 4000

// TestCreateChargesBulkSplitsPastTheParameterCap proves a run larger than one
// statement can hold is written whole, for both arbiters, and stays idempotent.
func TestCreateChargesBulkSplitsPastTheParameterCap(t *testing.T) {
	ctx := context.Background()
	const conceptID = "c0000000-0000-0000-0000-0000000000b9"
	repo := appRepo(t, tenantA)
	if _, err := repo.CreateConcept(ctx, newConcept(t, conceptID, tenantA, "bulk-batched", 8000)); err != nil {
		t.Fatalf("seed concept: %v", err)
	}

	build := func(idPrefix string) []domain.Charge {
		out := make([]domain.Charge, 0, 2*bulkRunSize)
		for i := 0; i < bulkRunSize; i++ {
			student := fmt.Sprintf("5b000000-0000-0000-0000-%012d", i)
			keyless := newCharge(t, fmt.Sprintf("%s1-0000-0000-0000-%012d", idPrefix, i),
				tenantA, student, cycleBulk, conceptID, 8000, 0, today)
			referenced, err := newCharge(t, fmt.Sprintf("%s2-0000-0000-0000-%012d", idPrefix, i),
				tenantA, student, cycleBulk, conceptID, 8000, 0, today).
				WithExternalRef("tuition:" + cycleBulk + ":" + student + ":2026-09")
			if err != nil {
				t.Fatalf("external ref: %v", err)
			}
			out = append(out, keyless, referenced)
		}
		return out
	}

	created, err := repo.CreateChargesBulk(ctx, build("db00000"))
	if err != nil {
		t.Fatalf("bulk past the parameter cap: %v", err)
	}
	if created != 2*bulkRunSize {
		t.Fatalf("created = %d, want %d", created, 2*bulkRunSize)
	}
	if n := countConceptCharges(t, tenantA, conceptID); n != 2*bulkRunSize {
		t.Fatalf("stored under tenant A = %d, want %d", n, 2*bulkRunSize)
	}

	again, err := repo.CreateChargesBulk(ctx, build("dc00000"))
	if err != nil {
		t.Fatalf("re-run: %v", err)
	}
	if again != 0 {
		t.Errorf("re-run created = %d, want 0", again)
	}
}

// countConceptCharges counts, under the tenant's own RLS context, the charges of
// one concept — what was stored, independently of the driver's RowsAffected.
func countConceptCharges(t *testing.T, tenant, conceptID string) int64 {
	t.Helper()
	db, err := open(appDSN)
	if err != nil {
		t.Fatalf("open app db: %v", err)
	}
	var n int64
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenant).Error; err != nil {
			return err
		}
		return tx.Table("charges").Where("concept_id = ?", conceptID).Count(&n).Error
	})
	if err != nil {
		t.Fatalf("count charges: %v", err)
	}
	return n
}
