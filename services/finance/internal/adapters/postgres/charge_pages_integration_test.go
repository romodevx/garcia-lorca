//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/finance/internal/adapters/postgres"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// Each test owns its cycle: the container is shared and never reset.
const (
	cyclePagedCharges = "70000000-0000-0000-0000-0000000014a1"
	cyclePagedOverdue = "70000000-0000-0000-0000-0000000014a2"
)

// pagedChargeCount is more than two pages of the 500-row bound: the first full
// school (567 pupils x 9 months = 5,103 charges) read only the first 500.
const pagedChargeCount = 1200

// seedPagedCycle stores pagedChargeCount charges in two blocks of equal due
// dates, with ids that do not follow insertion order, so the walk's order has to
// come from (due_date, id) and nothing else.
func seedPagedCycle(t *testing.T, repo *pgadapter.Repository) map[string]bool {
	t.Helper()
	const conceptID = "c0000000-0000-0000-0000-0000000014a1"
	ctx := context.Background()
	if _, err := repo.CreateConcept(ctx, newConcept(t, conceptID, tenantA, "paged-concept", 1000)); err != nil {
		t.Fatalf("seed concept: %v", err)
	}
	charges := make([]domain.Charge, 0, pagedChargeCount)
	ids := make(map[string]bool, pagedChargeCount)
	for i := 0; i < pagedChargeCount; i++ {
		scrambled := (i * 7919) % pagedChargeCount // 7919 is prime: a permutation
		id := fmt.Sprintf("d14a0000-0000-0000-0000-%012d", scrambled)
		student := fmt.Sprintf("5a140000-0000-0000-0000-%012d", i)
		due := today
		if i%2 == 1 {
			due = tomorrow()
		}
		charges = append(charges, newCharge(t, id, tenantA, student, cyclePagedCharges, conceptID, 1000, 0, due))
		ids[id] = true
	}
	if _, err := repo.CreateChargesBulk(ctx, charges); err != nil {
		t.Fatalf("seed charges: %v", err)
	}
	return ids
}

func walkCharges(t *testing.T, repo *pgadapter.Repository, cycle string, size int) (sizes []int, rows []ports.ChargeWithPaid) {
	t.Helper()
	var cursor ports.ChargeCursor
	for page := 0; ; page++ {
		if page > pagedChargeCount {
			t.Fatal("the walk never ended")
		}
		got, err := repo.ListChargesPage(context.Background(), "", cycle, cursor, size)
		if err != nil {
			t.Fatalf("ListChargesPage: %v", err)
		}
		sizes = append(sizes, len(got.Charges))
		rows = append(rows, got.Charges...)
		if got.Next.IsZero() {
			return sizes, rows
		}
		cursor = got.Next
	}
}

func chargeAfter(a, b domain.Charge) bool {
	if !a.DueDate().Equal(b.DueDate()) {
		return a.DueDate().After(b.DueDate())
	}
	return a.ID() > b.ID()
}

// TestListChargesPageWalksTheWholeCycle: 1,200 charges come back in three pages,
// each exactly once, in strictly increasing (due_date, id), and the last page
// says it is the last.
func TestListChargesPageWalksTheWholeCycle(t *testing.T) {
	repo := appRepo(t, tenantA)
	want := seedPagedCycle(t, repo)

	sizes, rows := walkCharges(t, repo, cyclePagedCharges, 500)

	if fmt.Sprint(sizes) != "[500 500 200]" {
		t.Errorf("page sizes = %v, want [500 500 200]", sizes)
	}
	seen := map[string]bool{}
	for i, r := range rows {
		id := r.Charge.ID()
		if seen[id] {
			t.Fatalf("charge %s came back twice", id)
		}
		seen[id] = true
		if !want[id] {
			t.Fatalf("charge %s is not this cycle's", id)
		}
		if i > 0 && !chargeAfter(r.Charge, rows[i-1].Charge) {
			t.Fatalf("row %d (%s) is not after row %d (%s) in (due_date, id)", i, id, i-1, rows[i-1].Charge.ID())
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("walked %d charges, want %d", len(seen), len(want))
	}

	// A page that ends exactly on the last row is the last page: the read looks
	// one row ahead instead of sending the caller for an empty page.
	exact, _ := walkCharges(t, repo, cyclePagedCharges, 400)
	if fmt.Sprint(exact) != "[400 400 400]" {
		t.Errorf("page sizes at 400 = %v, want [400 400 400] with no trailing empty page", exact)
	}
}

// A cursor whose id is not a charge id (a forged token that decoded) is the
// caller's mistake, not a database error surfacing as a 500.
func TestChargePagesRejectACursorThatIsNotAnID(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	forged := ports.ChargeCursor{DueDate: today, ID: "not-a-uuid"}

	_, err := repo.ListChargesPage(ctx, "", cyclePagedCharges, forged, 10)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("ListChargesPage: kind = %v (%v), want KindInvalid", glerrors.KindOf(err), err)
	}
	_, err = repo.ListOverduePage(ctx, cyclePagedOverdue, today, forged, 10)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("ListOverduePage: kind = %v (%v), want KindInvalid", glerrors.KindOf(err), err)
	}
}

// TestListOverduePageCursorPassesSettledRows: the cursor is the last row
// EXAMINED, so a page whose only candidate was paid comes back empty and still
// points past it.
func TestListOverduePageCursorPassesSettledRows(t *testing.T) {
	ctx := context.Background()
	const conceptID = "c0000000-0000-0000-0000-0000000014a2"
	repo := appRepo(t, tenantA)
	if _, err := repo.CreateConcept(ctx, newConcept(t, conceptID, tenantA, "paged-overdue", 1000)); err != nil {
		t.Fatalf("seed concept: %v", err)
	}
	ids := []string{
		"d14b0000-0000-0000-0000-000000000001",
		"d14b0000-0000-0000-0000-000000000002",
		"d14b0000-0000-0000-0000-000000000003",
	}
	for i, id := range ids {
		student := fmt.Sprintf("5b140000-0000-0000-0000-%012d", i)
		due := today.AddDate(0, 0, -3+i) // all before today, in id order
		if _, err := repo.CreateCharge(ctx, newCharge(t, id, tenantA, student, cyclePagedOverdue, conceptID, 1000, 0, due)); err != nil {
			t.Fatalf("seed charge %s: %v", id, err)
		}
	}
	if _, _, _, err := repo.RegisterPayment(ctx, one(newPayment(t, "f14b0000-0000-0000-0000-000000000002", tenantA, ids[1], 1000, paidAt)), derive, nil); err != nil {
		t.Fatalf("settle: %v", err)
	}

	var pages [][]string
	var cursor ports.ChargeCursor
	for i := 0; i < 5; i++ {
		got, err := repo.ListOverduePage(ctx, cyclePagedOverdue, today, cursor, 1)
		if err != nil {
			t.Fatalf("ListOverduePage: %v", err)
		}
		var page []string
		for _, c := range got.Charges {
			page = append(page, c.Charge.ID())
		}
		pages = append(pages, page)
		if got.Next.IsZero() {
			break
		}
		cursor = got.Next
	}

	want := fmt.Sprint([][]string{{ids[0]}, nil, {ids[2]}})
	if fmt.Sprint(pages) != want {
		t.Fatalf("pages = %v, want %v", pages, want)
	}
}
