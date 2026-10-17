//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"testing"

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

	again, err := repo.CreateChargesBulk(ctx, build("dc00000"))
	if err != nil {
		t.Fatalf("re-run: %v", err)
	}
	if again != 0 {
		t.Errorf("re-run created = %d, want 0", again)
	}
}
