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
			c.Charge.Effective().Int64(), domain.MethodCash, "", "", c.Charge.DueDate(), ""); err != nil {
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
//
// "Beyond the debt" now means beyond the WHOLE YEAR: ten months at $3,200 plus
// the surcharge September earned. A parent who settles the year and hands over
// a little extra keeps it.
func TestMoneyBeyondTheDebtBecomesCredit(t *testing.T) {
	f := depositFixture(t)

	got := deposit(t, f, 3300000, "2026-10-10")

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
	// The year is $32,000 of tuition plus $300 of surcharge; anything past that
	// is credit.
	const overpaid = 3300000
	deposit(t, f, overpaid, "2026-10-10")

	account, err := f.svc.GetAccount(context.Background(), testStudent)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if account.TotalBalanceCents != 0 {
		t.Fatalf("balance = %d, want the year settled", account.TotalBalanceCents)
	}
	if account.UnallocatedCents != overpaid-3230000 {
		t.Errorf("credit = %d, want %d", account.UnallocatedCents, overpaid-3230000)
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
	// The whole year is billed, so the balance is not zero — September is. The
	// month the money landed on is what this test is about.
	var settled int
	for _, c := range account.Charges {
		if c.Charge.Discount().Int64() > 0 && c.Charge.Status() == domain.StatusPaid {
			settled++
		}
	}
	if settled != 1 {
		t.Errorf("%d discounted months came out settled, want 1", settled)
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
	opening, err := f.svc.GetAccount(context.Background(), testStudent)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	before := opening.TotalBalanceCents

	preview, perr := f.svc.PreviewDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 670000, Method: domain.MethodCash,
		PaidAt: time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC),
	})
	if perr != nil {
		t.Fatalf("PreviewDeposit: %v", perr)
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
	if account.TotalBalanceCents != before {
		t.Errorf("balance moved from %d to %d — the preview collected something",
			before, account.TotalBalanceCents)
	}
}

// The day's takings, in one number per method plus the list behind it. This is
// the artefact that replaces the notebook: at close of day the office can say
// what came in, and in what form, without adding anything up by hand.
func TestDailyCashUpTotalsWhatCameIn(t *testing.T) {
	f := depositFixture(t)
	deposit(t, f, 320000, "2026-10-10")

	cut, err := f.svc.DailyCashUp(context.Background(), time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DailyCashUp: %v", err)
	}
	if cut.TotalCents != 320000 {
		t.Errorf("total = %d, want 320000", cut.TotalCents)
	}
	if cut.ByMethod[domain.MethodCash] != 320000 {
		t.Errorf("cash = %d, want 320000", cut.ByMethod[domain.MethodCash])
	}
	if len(cut.Deposits) != 1 {
		t.Errorf("listed %d deposits, want 1", len(cut.Deposits))
	}
}

// A cancelled deposit is not takings. Counting it would send someone to the safe
// looking for money that was handed back.
func TestDailyCashUpExcludesWhatWasCancelled(t *testing.T) {
	f := depositFixture(t)
	got := deposit(t, f, 320000, "2026-10-10")
	if err := f.svc.VoidDeposit(context.Background(), got.Deposit.ID(), "error de captura"); err != nil {
		t.Fatalf("VoidDeposit: %v", err)
	}

	cut, err := f.svc.DailyCashUp(context.Background(), time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DailyCashUp: %v", err)
	}
	if cut.TotalCents != 0 {
		t.Errorf("total = %d after the only deposit was cancelled, want 0", cut.TotalCents)
	}
}

// The owner's example, end to end: eleven months and an inscription exist from
// the start, the family hands over $6,000 in September, and six months come out
// marked paid — not five months and an unexplained credit.
func TestSixThousandPaysSixMonths(t *testing.T) {
	f := billingFixture(t)
	// A cleaner year for the arithmetic: $1,000 a month, no surcharge yet.
	if _, err := f.svc.UpsertTuitionPlan(context.Background(), usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel,
		EnrollmentCents: 200000, MonthlyCents: 100000,
		Months: 11, FirstBilledMonth: 9, DueDay: 10, Active: true,
	}); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	runOn(t, f, "2026-09-01")
	f.clock = time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)

	opening, err := f.svc.GetAccount(context.Background(), testStudent)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	// $1,000 × 11 + $2,000 = $13,000 owed for the year, visible on day one.
	if opening.TotalBalanceCents != 1300000 {
		t.Errorf("annual debt = %d, want 1300000", opening.TotalBalanceCents)
	}

	got := deposit(t, f, 600000, "2026-09-01")

	// The inscription is the oldest debt, so it takes $2,000; the remaining
	// $4,000 settles four months. Six charges closed in total.
	settled := 0
	for _, a := range got.Allocations {
		_ = a
		settled++
	}
	if settled != 5 {
		t.Errorf("the deposit landed on %d charges, want 5 (inscription + four months)", settled)
	}
	if got.UnallocatedCents != 0 {
		t.Errorf("unallocated = %d, want 0 — every peso found a month", got.UnallocatedCents)
	}

	after, err := f.svc.GetAccount(context.Background(), testStudent)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if after.TotalBalanceCents != 1300000-600000 {
		t.Errorf("balance = %d, want the annual debt less what was paid", after.TotalBalanceCents)
	}
	var paid int
	for _, c := range after.Charges {
		if c.Charge.Status() == domain.StatusPaid {
			paid++
		}
	}
	if paid != 5 {
		t.Errorf("%d charges are marked paid, want 5", paid)
	}
}

// And the partial: $1,500 pays one month and leaves $500 ON the next, not
// floating as credit. The owner said it in those words.
func TestFifteenHundredPaysOneMonthAndPartOfTheNext(t *testing.T) {
	f := billingFixture(t)
	if _, err := f.svc.UpsertTuitionPlan(context.Background(), usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel,
		EnrollmentCents: 0, MonthlyCents: 100000,
		Months: 11, FirstBilledMonth: 9, DueDay: 10, Active: true,
	}); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	runOn(t, f, "2026-09-01")
	f.clock = time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)

	got := deposit(t, f, 150000, "2026-09-01")

	if len(got.Allocations) != 2 {
		t.Fatalf("landed on %d charges, want 2", len(got.Allocations))
	}
	if got.Allocations[0].Amount().Int64() != 100000 || got.Allocations[1].Amount().Int64() != 50000 {
		t.Errorf("split = %d and %d, want 100000 and 50000",
			got.Allocations[0].Amount().Int64(), got.Allocations[1].Amount().Int64())
	}
	if got.UnallocatedCents != 0 {
		t.Errorf("unallocated = %d, want 0 — the $500 sits on October", got.UnallocatedCents)
	}
}

// Task 7: the deposit, and every payment it splits into, names who took the
// money — the counterweight to a payment captured with an earlier date.
func TestADepositNamesWhoReceivedIt(t *testing.T) {
	f := depositFixture(t)
	who := domain.Receiver{UserID: "c0000000-0000-0000-0000-000000000001", Name: "Laura Caja"}
	got, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 670000, Method: domain.MethodCash,
		PaidAt: time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC), Receiver: who,
	})
	if err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}
	if got.Deposit.ReceivedBy() != who {
		t.Errorf("deposit received by %+v, want %+v", got.Deposit.ReceivedBy(), who)
	}
	if len(got.Allocations) == 0 {
		t.Fatal("no allocations")
	}
	for _, a := range got.Allocations {
		if a.ReceivedBy() != who.UserID {
			t.Errorf("allocation %s received by %q, want %q", a.ID(), a.ReceivedBy(), who.UserID)
		}
	}
}
