package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// What the family must hand over today to be current: the months already due,
// plus what being late has cost, less the early-payment reward if they are
// inside the window. The counter shows this as "Monto esperado" and never lets
// anyone edit it.
func TestExpectedTodayIsWhatIsDueSoFar(t *testing.T) {
	f := depositFixture(t)

	expected, err := f.svc.ExpectedToday(context.Background(), testStudent,
		time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ExpectedToday: %v", err)
	}

	// September ($3,200) + its thirty days of surcharge ($300) + October
	// ($3,200, due today). The other eight months are billed but not yet due.
	if expected.AmountCents != 670000 {
		t.Errorf("expected = %d, want 670000", expected.AmountCents)
	}
	if expected.SurchargeCents != 30000 {
		t.Errorf("surcharge = %d, want 30000", expected.SurchargeCents)
	}
}

// The months that have not arrived are billed but not expected today. Asking a
// family for the whole year at the counter is not what the annual figure means.
func TestExpectedTodayIgnoresMonthsThatHaveNotArrived(t *testing.T) {
	f := depositFixture(t)

	account, err := f.svc.GetAccount(context.Background(), testStudent)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	expected, err := f.svc.ExpectedToday(context.Background(), testStudent,
		time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ExpectedToday: %v", err)
	}
	if expected.AmountCents >= account.TotalBalanceCents {
		t.Errorf("expected %d is not less than the annual debt %d",
			expected.AmountCents, account.TotalBalanceCents)
	}
	if expected.AnnualBalanceCents != account.TotalBalanceCents {
		t.Errorf("annual balance = %d, want %d", expected.AnnualBalanceCents, account.TotalBalanceCents)
	}
}

// Inside the window the reward is already in the figure, so the cashier reads
// the discounted number aloud rather than working it out.
func TestExpectedTodayCarriesTheEarlyPaymentReward(t *testing.T) {
	f := prontoPagoFixture(t)

	expected, err := f.svc.ExpectedToday(context.Background(), testStudent,
		time.Date(2026, time.September, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ExpectedToday: %v", err)
	}
	if expected.DiscountCents != 10000 {
		t.Errorf("discount = %d, want 10000", expected.DiscountCents)
	}
	if expected.AmountCents != 310000 {
		t.Errorf("expected = %d, want 310000 — September less the reward", expected.AmountCents)
	}
}

// The father who always pays on the 5th and missed by a day. The school decides
// to honour it; the system records that somebody decided, rather than pretending
// the date was different.
func TestTheSchoolCanGrantTheEarlyPaymentByHand(t *testing.T) {
	f := prontoPagoFixture(t)
	f.clock = time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)

	got, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID:   testStudent,
		AmountCents: 310000,
		Method:      domain.MethodCash,
		PaidAt:      time.Date(2026, time.September, 20, 0, 0, 0, 0, time.UTC),
		// Outside the window, but the office says yes.
		GrantEarlyPayment: true,
	})
	if err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}
	if got.UnallocatedCents != 0 {
		t.Errorf("unallocated = %d, want 0 — $3,100 settles the discounted month", got.UnallocatedCents)
	}

	account, err := f.svc.GetAccount(context.Background(), testStudent)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	var granted int
	for _, c := range account.Charges {
		if c.Charge.DiscountReason() == domain.ReasonEarlyPaymentGranted {
			granted++
		}
	}
	if granted != 1 {
		t.Errorf("%d charges record the discount as GRANTED by the school, want 1", granted)
	}
}

// Without the school saying so, a late payment gets nothing. The button is a
// decision, not a default.
func TestWithoutTheGrantALatePaymentStillGetsNothing(t *testing.T) {
	f := prontoPagoFixture(t)
	f.clock = time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)

	got, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID:   testStudent,
		AmountCents: 310000,
		Method:      domain.MethodCash,
		PaidAt:      time.Date(2026, time.September, 20, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}
	// $3,100 against an undiscounted $3,200 month: it part-pays, nothing is left over.
	if got.UnallocatedCents != 0 {
		t.Errorf("unallocated = %d, want 0", got.UnallocatedCents)
	}
	account, err := f.svc.GetAccount(context.Background(), testStudent)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	for _, c := range account.Charges {
		if c.Charge.Discount().Int64() > 0 {
			t.Errorf("a discount appeared without anybody granting it: %+v", c.Charge)
		}
	}
}
