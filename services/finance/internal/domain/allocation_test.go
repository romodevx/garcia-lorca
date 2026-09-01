package domain_test

import (
	"testing"
	"time"

	"github.com/garcia-lorca/services/finance/internal/domain"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// The owner's own example. A family pays $6,700 on 10 October: September's
// tuition, thirty days of surcharge on it, and October's tuition due that day.
func TestADepositCoversOldestFirst(t *testing.T) {
	items := []domain.Payable{
		{ChargeID: "sep", DueOn: day(2026, time.September, 10), OwedCents: 320000},
		{ChargeID: "sep-fee", DueOn: day(2026, time.September, 10), OwedCents: 30000, Surcharge: true},
		{ChargeID: "oct", DueOn: day(2026, time.October, 10), OwedCents: 320000},
	}

	got, left := domain.Allocate(670000, items)

	if left != 0 {
		t.Errorf("left over = %d, want 0 — the deposit covers exactly the three", left)
	}
	if len(got) != 3 {
		t.Fatalf("allocated to %d items, want 3", len(got))
	}
	for i, want := range []int64{320000, 30000, 320000} {
		if got[i].AmountCents != want {
			t.Errorf("allocation %d = %d, want %d", i, got[i].AmountCents, want)
		}
	}
}

// Money that runs out mid-way part-pays the item it reaches and never touches
// what comes after. A parent who brings what they have must see it land on the
// oldest thing they owe, not spread thinly over everything.
func TestAShortDepositPartPaysAndStops(t *testing.T) {
	items := []domain.Payable{
		{ChargeID: "sep", DueOn: day(2026, time.September, 10), OwedCents: 320000},
		{ChargeID: "oct", DueOn: day(2026, time.October, 10), OwedCents: 320000},
	}

	got, left := domain.Allocate(400000, items)

	if left != 0 {
		t.Errorf("left over = %d, want 0", left)
	}
	if len(got) != 2 {
		t.Fatalf("allocated to %d items, want 2", len(got))
	}
	if got[0].AmountCents != 320000 || got[1].AmountCents != 80000 {
		t.Errorf("split = %d and %d, want 320000 and 80000", got[0].AmountCents, got[1].AmountCents)
	}
}

// More money than is owed is not an error and not a refusal: it is a credit the
// family keeps. Refusing it would send a parent away with cash in hand.
func TestMoneyBeyondWhatIsOwedStaysUnallocated(t *testing.T) {
	items := []domain.Payable{
		{ChargeID: "sep", DueOn: day(2026, time.September, 10), OwedCents: 320000},
	}

	got, left := domain.Allocate(500000, items)

	if len(got) != 1 || got[0].AmountCents != 320000 {
		t.Fatalf("allocations wrong: %+v", got)
	}
	if left != 180000 {
		t.Errorf("unallocated = %d, want 180000 — the family's credit", left)
	}
}

// Nothing owed at all: every peso is credit, and no allocation is invented.
func TestADepositWithNothingOwedIsAllCredit(t *testing.T) {
	got, left := domain.Allocate(500000, nil)

	if len(got) != 0 {
		t.Errorf("invented %d allocations against nothing owed", len(got))
	}
	if left != 500000 {
		t.Errorf("unallocated = %d, want the whole 500000", left)
	}
}

// An item already covered is skipped rather than allocated zero. A zero-peso
// payment row is a line in a family's statement that means nothing and has to be
// explained.
func TestFullyPaidItemsProduceNoAllocation(t *testing.T) {
	items := []domain.Payable{
		{ChargeID: "sep", DueOn: day(2026, time.September, 10), OwedCents: 0},
		{ChargeID: "oct", DueOn: day(2026, time.October, 10), OwedCents: 320000},
	}

	got, _ := domain.Allocate(320000, items)

	if len(got) != 1 || got[0].ChargeID != "oct" {
		t.Errorf("allocations wrong: %+v", got)
	}
}
