package usecase_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// depositFixture is the owner's own scenario: a pupil on the daily rule, with
// September's tuition unpaid since the 10th and October's due today.
//
// September $3,200 · surcharge $10 a day · October $3,200.
func depositFixture(t *testing.T) *fixture {
	t.Helper()
	f := dailyFixture(t)
	// Bill through 10 October so both months exist as charges, and put the clock
	// there too: a deposit dated after today is refused, and the whole scenario
	// happens on the 10th.
	runOn(t, f, "2026-10-10")
	// The inscription is settled on its own due date, so the scenario is the
	// owner's: two months of tuition, one of them thirty days late, and nothing
	// else outstanding.
	settleInscription(t, f)
	f.clock = time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	return f
}

// settleInscription pays the enrolment charge in full on the day it fell due.
func settleInscription(t *testing.T, f *fixture) {
	t.Helper()
	charges, err := f.svc.ListCharges(context.Background(), testStudent, "")
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	enrollment := f.repo.conceptIDOfKind(domain.KindEnrollment)
	for _, c := range charges {
		if c.Charge.ConceptID() != enrollment {
			continue
		}
		if _, _, err := f.svc.RegisterPaymentAt(context.Background(), c.Charge.ID(),
			c.Charge.Effective().Int64(), domain.MethodCash, "", "", c.Charge.DueDate()); err != nil {
			t.Fatalf("settling the inscription: %v", err)
		}
		return
	}
	t.Fatal("the fixture has no inscription charge to settle")
}

func deposit(t *testing.T, f *fixture, amount int64, on string) usecase.DepositResult {
	t.Helper()
	when, err := time.Parse("2006-01-02", on)
	if err != nil {
		t.Fatalf("bad test date %q: %v", on, err)
	}
	got, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID:      testStudent,
		AmountCents:    amount,
		Method:         domain.MethodCash,
		PaidAt:         when,
		IdempotencyKey: "dep-key-" + on,
	})
	if err != nil {
		t.Fatalf("RegisterDeposit(%d on %s): %v", amount, on, err)
	}
	return got
}

// The whole point, in the owner's numbers. On 10 October a family hands over
// $6,700: September's $3,200, the thirty days of surcharge it earned, and
// October's $3,200 due today.
func TestADepositCoversTheMonthAndItsSurcharge(t *testing.T) {
	f := depositFixture(t)

	got := deposit(t, f, 670000, "2026-10-10")

	if got.UnallocatedCents != 0 {
		t.Errorf("unallocated = %d, want 0 — the money covers exactly what is owed", got.UnallocatedCents)
	}
	if len(got.Surcharges) != 1 {
		t.Fatalf("materialised %d surcharges, want 1 (September, thirty days)", len(got.Surcharges))
	}
	if amount := got.Surcharges[0].Amount().Int64(); amount != 30000 {
		t.Errorf("surcharge = %d cents, want 30000 — thirty days at $10", amount)
	}
	if len(got.Allocations) != 3 {
		t.Errorf("split into %d payments, want 3 (September, its surcharge, October)", len(got.Allocations))
	}
}

// The surcharge exists only where money landed on it. A family that pays nothing
// must not accumulate a table of surcharge rows nobody asked for.
func TestNoMoneyMeansNoSurchargeRow(t *testing.T) {
	f := depositFixture(t)

	// Enough for September alone, not for its surcharge.
	got := deposit(t, f, 320000, "2026-10-10")

	if len(got.Allocations) != 1 {
		t.Fatalf("split into %d payments, want 1", len(got.Allocations))
	}
	if len(got.Surcharges) != 0 {
		t.Errorf("materialised %d surcharge rows for money that never reached them", len(got.Surcharges))
	}
}

// Money beyond the debt is the family's credit, not an error. Refusing it sends
// a parent away from the counter with cash in hand.
func TestMoneyBeyondTheDebtBecomesCredit(t *testing.T) {
	f := depositFixture(t)

	got := deposit(t, f, 1000000, "2026-10-10")

	if got.UnallocatedCents <= 0 {
		t.Errorf("unallocated = %d, want the leftover recorded as credit", got.UnallocatedCents)
	}
}

// The retry a lost response provokes must not cost the family twice.
func TestADepositReplayCollectsNothingTwice(t *testing.T) {
	f := depositFixture(t)

	first := deposit(t, f, 670000, "2026-10-10")
	second := deposit(t, f, 670000, "2026-10-10")

	if second.Deposit.ID() != first.Deposit.ID() {
		t.Errorf("replay stored a second deposit: %s then %s", first.Deposit.ID(), second.Deposit.ID())
	}
	if !second.Replayed {
		t.Error("the second call does not report itself as a replay")
	}
}

// The classic counter typo — 2027 for 2026 — silently lifts a payment out of
// every overdue calculation, and nobody notices until a parent is wrongly told
// they are current.
func TestADepositCannotBeDatedInTheFuture(t *testing.T) {
	f := depositFixture(t)

	_, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 100000, Method: domain.MethodCash,
		PaidAt: f.clock.AddDate(1, 0, 0),
	})
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("future-dated deposit: want KindInvalid, got %v", err)
	}
}

// A cashier who keys $67,000 instead of $6,700 has to be able to undo it, and
// what the money settled has to go back to being owed.
func TestVoidingADepositReleasesWhatItSettled(t *testing.T) {
	f := depositFixture(t)
	got := deposit(t, f, 670000, "2026-10-10")

	if err := f.svc.VoidDeposit(context.Background(), got.Deposit.ID(), "importe capturado mal"); err != nil {
		t.Fatalf("VoidDeposit: %v", err)
	}

	account, err := f.svc.GetAccount(context.Background(), testStudent)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if account.TotalBalanceCents <= 0 {
		t.Errorf("balance = %d after voiding the only deposit — the debt did not come back", account.TotalBalanceCents)
	}
}

// A family that overpays has money sitting with the school. If the statement
// does not say so, the office cannot tell an overpayment from a mistake and the
// parent's credit is invisible to everyone but them.
func TestTheStatementShowsTheFamilysCredit(t *testing.T) {
	f := depositFixture(t)
	deposit(t, f, 1000000, "2026-10-10")

	account, err := f.svc.GetAccount(context.Background(), testStudent)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if account.UnallocatedCents != 1000000-670000 {
		t.Errorf("credit = %d, want %d", account.UnallocatedCents, 1000000-670000)
	}
}

// prontoPagoFixture is the school on the early-payment rule: $100 off any month
// settled inside the first ten working days.
func prontoPagoFixture(t *testing.T) *fixture {
	t.Helper()
	f := billingFixture(t)
	if _, err := f.svc.UpsertTuitionPlan(context.Background(), usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel,
		EnrollmentCents: 50000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, Active: true,
		Policy: domain.CollectionPolicy{
			DailyMode: true, LateFeeDailyCents: 1000, SurchargeCountsAllDays: true,
			EarlyPaymentCents: 10000, EarlyPaymentDays: 10,
		},
	}); err != nil {
		t.Fatalf("seed pronto-pago plan: %v", err)
	}
	runOn(t, f, "2026-09-10")
	settleInscription(t, f)
	f.clock = time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC)
	return f
}

// A family that pays early pays less, and the system does it rather than waiting
// for someone at the counter to remember.
func TestPayingEarlyTakesTheDiscount(t *testing.T) {
	f := prontoPagoFixture(t)

	// 3 September is the third working day of the month. September's tuition is
	// $3,200, so $3,100 settles it.
	got := deposit(t, f, 310000, "2026-09-03")

	if got.UnallocatedCents != 0 {
		t.Errorf("unallocated = %d, want 0 — $3,100 settles a discounted month", got.UnallocatedCents)
	}
	account, err := f.svc.GetAccount(context.Background(), testStudent)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if account.TotalBalanceCents != 0 {
		t.Errorf("balance = %d, want 0 — September is settled", account.TotalBalanceCents)
	}
	var discounted int
	for _, c := range account.Charges {
		if c.Charge.Discount().Int64() == 10000 && c.Charge.DiscountReason() == domain.ReasonEarlyPayment {
			discounted++
		}
	}
	if discounted != 1 {
		t.Errorf("%d charges carry a recorded early-payment discount, want 1", discounted)
	}
}

// Past the window the discount is gone. It is a reward for paying early, not a
// price cut, and a system that gave it late would be quietly repricing the year.
func TestPayingLateTakesNoDiscount(t *testing.T) {
	f := prontoPagoFixture(t)
	f.clock = time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)

	got := deposit(t, f, 310000, "2026-09-20")

	account, err := f.svc.GetAccount(context.Background(), testStudent)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if account.TotalBalanceCents == 0 {
		t.Errorf("balance = 0 — a late payment got the early discount")
	}
	if got.UnallocatedCents != 0 {
		t.Errorf("unallocated = %d, want 0", got.UnallocatedCents)
	}
}

// Before taking the money, the cashier has to be able to say out loud what it
// will cover. The preview runs the same split as the real thing and writes
// nothing.
func TestPreviewShowsTheSplitWithoutTakingTheMoney(t *testing.T) {
	f := depositFixture(t)

	preview, err := f.svc.PreviewDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 670000, Method: domain.MethodCash,
		PaidAt: time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("PreviewDeposit: %v", err)
	}
	if len(preview.Allocations) != 3 {
		t.Errorf("preview split into %d, want 3 (September, its surcharge, October)", len(preview.Allocations))
	}
	if preview.UnallocatedCents != 0 {
		t.Errorf("preview credit = %d, want 0", preview.UnallocatedCents)
	}

	account, err := f.svc.GetAccount(context.Background(), testStudent)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if len(account.Payments) != 1 {
		// Only the inscription the fixture settled. A preview that collected
		// money would show up here.
		t.Errorf("the preview moved money: %d payments on the account", len(account.Payments))
	}
	if account.TotalBalanceCents != 640000 {
		t.Errorf("balance = %d after a preview, want the untouched 640000", account.TotalBalanceCents)
	}
}
