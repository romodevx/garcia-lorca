package usecase_test

import (
	"context"
	"fmt"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// seedManyCharges issues n charges to the test pupil, all on the same due date,
// so the walk's order rests on the id tie-breaker alone.
func seedManyCharges(t *testing.T, f *fixture, n int) map[string]bool {
	t.Helper()
	concept := f.seedConcept(t, "Colegiatura", 250000)
	ids := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		c, err := f.svc.CreateCharge(context.Background(), seedChargeInput(concept.ID(), i, testDueDate))
		if err != nil {
			t.Fatalf("seed charge %d: %v", i, err)
		}
		ids[c.Charge.ID()] = true
	}
	return ids
}

func seedChargeInput(conceptID string, i int, dueDate string) usecase.CreateChargeInput {
	return usecase.CreateChargeInput{
		StudentID: testStudent, CycleID: testCycle, ConceptID: conceptID,
		DueDate: dueDate, AmountCentsOverride: 100000,
		DescriptionOverride: fmt.Sprintf("Cargo %04d", i),
		ExternalRef:         fmt.Sprintf("t14:%04d", i),
	}
}

// T14 regression: a cycle with 5,103 charges showed the first 500 and said
// nothing. The paged read hands back every charge exactly once, in a stable
// order, and says when it is done.
func TestListChargesPageWalksTheCycleWithoutGapsOrRepeats(t *testing.T) {
	f := newFixture(t)
	want := seedManyCharges(t, f, 1200)

	seen := map[string]bool{}
	var sizes []int
	var cursor ports.ChargeCursor
	for page := 0; ; page++ {
		if page > 10 {
			t.Fatal("the walk never ended")
		}
		got, err := f.svc.ListChargesPage(context.Background(), "", testCycle, cursor, 500)
		requireNoError(t, err)
		sizes = append(sizes, len(got.Charges))
		for _, c := range got.Charges {
			if seen[c.Charge.ID()] {
				t.Fatalf("charge %s came back twice", c.Charge.ID())
			}
			seen[c.Charge.ID()] = true
		}
		if got.Next.IsZero() {
			break
		}
		cursor = got.Next
	}

	if fmt.Sprint(sizes) != "[500 500 200]" {
		t.Errorf("page sizes = %v, want [500 500 200]", sizes)
	}
	if len(seen) != len(want) {
		t.Fatalf("walked %d charges, want %d", len(seen), len(want))
	}
}

// ListCharges is the whole set, never its first page.
func TestListChargesReturnsEveryPage(t *testing.T) {
	f := newFixture(t)
	want := seedManyCharges(t, f, 1200)

	got, err := f.svc.ListCharges(context.Background(), "", testCycle)

	requireNoError(t, err)
	if len(got) != len(want) {
		t.Fatalf("ListCharges = %d charges, want all %d", len(got), len(want))
	}
}

func TestListChargesPageSizeDefaultsAndClamps(t *testing.T) {
	tests := []struct {
		name      string
		size      int
		wantLimit int
	}{
		{"zero means the default", 0, 500},
		{"negative means the default", -3, 500},
		{"above the maximum is clamped", 9999, 500},
		{"a small page is honoured", 25, 25},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			_, err := f.svc.ListChargesPage(context.Background(), "", testCycle, ports.ChargeCursor{}, tc.size)
			requireNoError(t, err)
			if f.repo.lastPageLimit != tc.wantLimit {
				t.Errorf("repository limit = %d, want %d", f.repo.lastPageLimit, tc.wantLimit)
			}
		})
	}
}

func TestListChargesPageRequiresAScopingFilter(t *testing.T) {
	f := newFixture(t)

	_, err := f.svc.ListChargesPage(context.Background(), "", "", ports.ChargeCursor{}, 0)

	requireKind(t, err, glerrors.KindInvalid)
}

// A page of the overdue walk can come back EMPTY — every candidate it examined
// was paid — while later candidates still owe. The cursor must move past what
// was examined, not stop with what was returned.
func TestListOverduePageMovesPastAFullyPaidPage(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)
	var charges []domain.Charge
	for i := 0; i < 3; i++ {
		// Distinct days, so (due_date, id) order is creation order.
		c, err := f.svc.CreateCharge(context.Background(), seedChargeInput(concept.ID(), i, fmt.Sprintf("2026-09-0%d", i+1)))
		requireNoError(t, err)
		charges = append(charges, c.Charge)
	}
	// The middle one in (due_date, id) order is settled.
	if _, _, err := f.svc.RegisterPayment(context.Background(), charges[1].ID(), 100000, domain.MethodCash, "", ""); err != nil {
		t.Fatalf("settle: %v", err)
	}

	var pages [][]string
	var cursor ports.ChargeCursor
	for i := 0; i < 5; i++ {
		got, err := f.svc.ListOverduePage(context.Background(), testCycle, cursor, 1)
		requireNoError(t, err)
		var ids []string
		for _, c := range got.Charges {
			ids = append(ids, c.Charge.ID())
		}
		pages = append(pages, ids)
		if got.Next.IsZero() {
			break
		}
		cursor = got.Next
	}

	want := fmt.Sprint([][]string{{charges[0].ID()}, nil, {charges[2].ID()}})
	if fmt.Sprint(pages) != want {
		t.Fatalf("pages = %v, want %v — the walk must step over a page that came back empty", pages, want)
	}
}

// The whole overdue list (and the billing run behind it) keeps walking past 500
// examined candidates that were all paid.
func TestListOverdueWalksPastAPaidFirstPage(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 250000)
	var last domain.Charge
	for i := 0; i < 501; i++ {
		due := testDueDate
		if i == 500 {
			due = "2026-09-10" // still overdue, and last in (due_date, id) order
		}
		c, err := f.svc.CreateCharge(context.Background(), seedChargeInput(concept.ID(), i, due))
		requireNoError(t, err)
		last = c.Charge
		if i < 500 {
			if _, _, err := f.svc.RegisterPayment(context.Background(), c.Charge.ID(), 100000, domain.MethodCash, "", ""); err != nil {
				t.Fatalf("settle %d: %v", i, err)
			}
		}
	}

	got, err := f.svc.ListOverdue(context.Background(), testCycle)

	requireNoError(t, err)
	if len(got) != 1 || got[0].Charge.ID() != last.ID() {
		t.Fatalf("overdue = %d rows, want just the 501st charge %s", len(got), last.ID())
	}
}
