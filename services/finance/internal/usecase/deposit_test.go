package usecase_test

import (
	"context"
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
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

// Money short of the month leaves the month open, and an open month keeps
// earning: its surcharge is derived again tomorrow, so no row is written yet.
func TestAPartPaymentWritesNoSurchargeRow(t *testing.T) {
	f := depositFixture(t)

	got := deposit(t, f, 100000, "2026-10-10")

	if len(got.Surcharges) != 0 {
		t.Errorf("materialised %d surcharge rows while the month is still open", len(got.Surcharges))
	}
}

// Task 9: settling the month without its surcharge does not forgive it. The
// days it earned are written down as a debt of their own — or a cashier could
// take $3,500, key $3,200 and leave no trace of the $300. Forgiving it is the
// owner's discount or cancellation.
func TestSettlingTheMonthLeavesItsSurchargeOwed(t *testing.T) {
	f := depositFixture(t)

	// September alone: $3,200, not its thirty days of surcharge.
	got := deposit(t, f, 320000, "2026-10-10")

	if len(got.Allocations) != 1 {
		t.Fatalf("split into %d payments, want 1 (September)", len(got.Allocations))
	}
	if len(got.Surcharges) != 1 || got.Surcharges[0].Amount().Int64() != 30000 {
		t.Fatalf("surcharges = %+v, want September's $300 written down", got.Surcharges)
	}

	expected, err := f.svc.ExpectedToday(context.Background(), testStudent, f.clock)
	if err != nil {
		t.Fatalf("ExpectedToday: %v", err)
	}
	if expected.SurchargeCents != 30000 || expected.AmountCents != 350000 {
		t.Errorf("expected after = %d (surcharge %d), want 350000 with 30000 surcharge — October plus September's fee",
			expected.AmountCents, expected.SurchargeCents)
	}

	// The next deposit collects it, and never charges those days twice.
	next, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 350000, Method: domain.MethodCash,
		PaidAt: time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC), IdempotencyKey: "dep-key-second",
	})
	if err != nil {
		t.Fatalf("second deposit: %v", err)
	}
	if next.UnallocatedCents != 0 || len(next.Surcharges) != 0 {
		t.Errorf("second deposit: unallocated %d, new surcharges %d; want 0 and 0", next.UnallocatedCents, len(next.Surcharges))
	}
}

// Money that reaches part of the surcharge writes it once: the settled month
// and the allocation must not each add their own row.
func TestAPartlyPaidSurchargeIsWrittenOnce(t *testing.T) {
	f := depositFixture(t)

	got := deposit(t, f, 330000, "2026-10-10")

	if len(got.Surcharges) != 1 || got.Surcharges[0].Amount().Int64() != 30000 {
		t.Fatalf("surcharges = %d rows, want one row of 30000", len(got.Surcharges))
	}
	if len(got.Allocations) != 2 || got.Allocations[1].Amount().Int64() != 10000 {
		t.Errorf("allocations = %d, want September and $100 on its surcharge", len(got.Allocations))
	}
}

// Task 9: a deposit keyed on the wrong date and voided takes its surcharge
// with it. Re-keyed on the right, earlier date, the family owes that date's
// days — not the wrong date's, which a row left standing would keep.
func TestVoidAndReKeyEarlierOwesTheEarlierDays(t *testing.T) {
	f := depositFixture(t)
	f.clock = time.Date(2026, time.October, 20, 12, 0, 0, 0, time.UTC)

	wrong := deposit(t, f, 320000, "2026-10-20")
	if len(wrong.Surcharges) != 1 {
		t.Fatalf("wrong-date deposit wrote %d surcharges, want 1", len(wrong.Surcharges))
	}
	if err := f.svc.VoidDeposit(context.Background(), wrong.Deposit.ID(), "fecha equivocada"); err != nil {
		t.Fatalf("VoidDeposit: %v", err)
	}

	right := deposit(t, f, 320000, "2026-10-10")
	if len(right.Surcharges) != 1 || right.Surcharges[0].Amount().Int64() != 30000 {
		t.Fatalf("re-keyed surcharges = %+v, want the $300 of 10 October", right.Surcharges)
	}
}

// Task 9: a month the reward alone closes — a part payment left less than the
// reward — still keeps the surcharge it earned. The reward is capped at what the
// month owes, and a month closed without money is settled all the same.
func TestAMonthClosedByTheRewardKeepsItsSurcharge(t *testing.T) {
	f := prontoPagoFixture(t)
	f.clock = time.Date(2026, time.October, 20, 12, 0, 0, 0, time.UTC)
	runOn(t, f, "2026-10-20")
	charges, err := f.svc.ListCharges(context.Background(), testStudent, "")
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	var october domain.Charge
	for _, c := range charges {
		if c.Charge.ExternalRef() == "tuition:"+testCycle+":"+testStudent+":2026-10" {
			october = c.Charge
		}
	}
	// $50 of October left, less than the $100 reward.
	if _, _, err := f.svc.RegisterPaymentAt(context.Background(), october.ID(), 315000, domain.MethodCash, "", "",
		time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC), ""); err != nil {
		t.Fatalf("part payment: %v", err)
	}

	// September granted ($3,100) and its forty days ($400): nothing reaches
	// October (due Monday the 12th), which the reward closes, nor its eight days.
	got, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 350000, Method: domain.MethodCash,
		PaidAt: time.Date(2026, time.October, 20, 0, 0, 0, 0, time.UTC), IdempotencyKey: "dep-granted",
		GrantEarlyPayment: true,
	})
	if err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}
	var octoberFee int64
	for _, fee := range got.Surcharges {
		if strings.Contains(fee.ExternalRef(), october.ID()) {
			octoberFee = fee.Amount().Int64()
		}
	}
	if octoberFee != 8000 {
		t.Errorf("October's surcharge written = %d, want 8000 — eight days, kept though the reward closed the month", octoberFee)
	}
}

// Task 10: a uniform sold to a pupil with no group yet is charged with no
// cycle. If its payment never landed, it is an open debt like any other: a
// deposit must still go through, and the surcharge it earned rides it with no
// cycle either — as a uniform charge with a cycle already earns one.
func TestADepositPaysAnOverdueSaleWithNoCycle(t *testing.T) {
	f := depositFixture(t)
	concept := f.seedConcept(t, "Uniformes", 0)
	sale, err := f.svc.CreateCharge(context.Background(), usecase.CreateChargeInput{
		StudentID: testStudent, ConceptID: concept.ID(), DueDate: "2026-09-10",
		AmountCentsOverride: 32000, DescriptionOverride: "Uniformes · U-2026-000009",
		ExternalRef: "uniforms:ful-unpaid",
	})
	if err != nil {
		t.Fatalf("CreateCharge (sale, no cycle): %v", err)
	}

	// September ($3,200 + $300), the sale ($320 + $300), October ($3,200).
	got := deposit(t, f, 730000, "2026-10-10")

	var saleFee *domain.Charge
	for i, fee := range got.Surcharges {
		if strings.Contains(fee.ExternalRef(), sale.Charge.ID()) {
			saleFee = &got.Surcharges[i]
		}
	}
	if saleFee == nil {
		t.Fatalf("no surcharge written for the overdue sale; surcharges = %d", len(got.Surcharges))
	}
	if saleFee.CycleID() != "" || saleFee.Amount().Int64() != 30000 {
		t.Errorf("sale surcharge = cycle %q, %d; want no cycle and 30000", saleFee.CycleID(), saleFee.Amount().Int64())
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
	// T5 (ISSUE-006): the balance nets the credit — the year settled and
	// the rest below zero, as the statement reads it.
	if account.TotalBalanceCents != -(overpaid - 3230000) {
		t.Fatalf("balance = %d, want the year settled and %d in the family's favour",
			account.TotalBalanceCents, overpaid-3230000)
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

// Review of D8: a cut asked without a day is the school's today, not the
// server's: at 19:00 in Morelia on the 10th (01:00 UTC on the 11th) the office
// still closes the 10th.
func TestDailyCashUpWithoutADayIsTheSchoolsToday(t *testing.T) {
	f := depositFixture(t)
	deposit(t, f, 320000, "2026-10-10")
	f.clock = time.Date(2026, time.October, 11, 1, 0, 0, 0, time.UTC)

	cut, err := f.svc.DailyCashUp(context.Background(), time.Time{})

	if err != nil {
		t.Fatalf("DailyCashUp: %v", err)
	}
	if got := cut.Day.Format("2006-01-02"); got != "2026-10-10" {
		t.Errorf("day = %s, want the school's today 2026-10-10", got)
	}
	if cut.TotalCents != 320000 {
		t.Errorf("total = %d, want the 10th's 320000", cut.TotalCents)
	}
}

// Money taken outside a deposit — a counter sale's, paid straight against its
// charge — is in the drawer too. The cut that counted deposits alone said
// $6,500 while the drawer held $1,350 more (QA 2026-09-25, ISSUE-010).
func TestDailyCashUpCountsPaymentsTakenOutsideADeposit(t *testing.T) {
	f := depositFixture(t)
	charges, err := f.svc.ListCharges(context.Background(), testStudent, "")
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	var open ports.ChargeWithPaid
	for _, c := range charges {
		if c.Charge.Amount().Int64()-c.Charge.Discount().Int64()-c.PaidCents > 0 {
			open = c
			break
		}
	}
	if open.Charge.ID() == "" {
		t.Fatal("the fixture left no charge open")
	}
	if _, _, err := f.svc.RegisterPayment(context.Background(), open.Charge.ID(), 5000, domain.MethodCard, "", "counter-sale-1"); err != nil {
		t.Fatalf("RegisterPayment: %v", err)
	}
	deposit(t, f, 320000, "2026-10-10")

	cut, err := f.svc.DailyCashUp(context.Background(), time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DailyCashUp: %v", err)
	}
	if cut.TotalCents != 325000 {
		t.Errorf("total = %d, want 325000 (the deposit and the loose payment)", cut.TotalCents)
	}
	if cut.ByMethod[domain.MethodCard] != 5000 {
		t.Errorf("card = %d, want 5000", cut.ByMethod[domain.MethodCard])
	}
	if len(cut.Payments) != 1 {
		t.Fatalf("listed %d loose payments, want 1", len(cut.Payments))
	}
	if got := cut.Payments[0].Description; got != open.Charge.Description() {
		t.Errorf("the loose payment says %q, want what it paid for: %q", got, open.Charge.Description())
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

// Task 8: the cashier chooses what the money pays. Only the chosen charges are
// paid, oldest first; a month's surcharge still goes with it; what is left is
// the family's credit, not spread onto what was not chosen.
func TestADepositPaysOnlyTheChosenCharges(t *testing.T) {
	f := depositFixture(t)
	expected, err := f.svc.ExpectedToday(context.Background(), testStudent, time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ExpectedToday: %v", err)
	}
	october := expected.Lines[1].ChargeID

	got, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 400000, Method: domain.MethodCash,
		PaidAt: time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC), ChargeIDs: []string{october},
	})
	if err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}
	if len(got.Allocations) != 1 || got.Allocations[0].ChargeID() != october || got.Allocations[0].Amount().Int64() != 320000 {
		t.Errorf("allocations = %+v, want October alone, 3,200", got.Allocations)
	}
	if got.UnallocatedCents != 80000 {
		t.Errorf("credit = %d, want the 800 left over", got.UnallocatedCents)
	}
	if len(got.Surcharges) != 0 {
		t.Errorf("materialised %d surcharges; September was not chosen", len(got.Surcharges))
	}
}

// A chosen late month carries its surcharge: the fee is never left behind.
func TestAChosenLateMonthCarriesItsSurcharge(t *testing.T) {
	f := depositFixture(t)
	expected, _ := f.svc.ExpectedToday(context.Background(), testStudent, time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC))
	september := expected.Lines[0].ChargeID

	got, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 350000, Method: domain.MethodCash,
		PaidAt: time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC), ChargeIDs: []string{september},
	})
	if err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}
	if len(got.Surcharges) != 1 || got.UnallocatedCents != 0 {
		t.Errorf("surcharges %d, credit %d; want September's surcharge paid and nothing left", len(got.Surcharges), got.UnallocatedCents)
	}
}

func TestAChargeOfAnotherStudentCannotBeChosen(t *testing.T) {
	f := depositFixture(t)
	_, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 1000, Method: domain.MethodCash,
		PaidAt: time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC), ChargeIDs: []string{"not-theirs"},
	})
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v", err)
	}
}

// Under the monthly rule the billing run writes each surcharge as its own
// charge ("latefee:<month>:…"). It rides on its month — on the counter's list
// and in the money — and is never a line of its own the cashier could choose
// apart from it (a choice holding it used to be refused outright).
func TestAMonthlySurchargeRidesOnItsMonth(t *testing.T) {
	f := billingFixture(t)
	runOn(t, f, "2026-11-12")
	f.clock = time.Date(2026, time.November, 12, 12, 0, 0, 0, time.UTC)
	lateFee := f.repo.conceptIDOfKind(domain.KindLateFee)

	expected, err := f.svc.ExpectedToday(context.Background(), testStudent, f.clock)
	if err != nil {
		t.Fatalf("ExpectedToday: %v", err)
	}
	var september usecase.ExpectedLine
	for _, l := range expected.Lines {
		if c := f.repo.charges[l.ChargeID]; c.ConceptID() == lateFee {
			t.Fatalf("a surcharge is a line of its own: %+v", l)
		}
		if strings.HasPrefix(l.Description, "Colegiatura septiembre") {
			september = l
		}
	}
	if september.SurchargeCents <= 0 {
		t.Fatalf("september carries no surcharge: %+v", september)
	}

	got, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: september.TotalCents, Method: domain.MethodCash,
		PaidAt: f.clock, ChargeIDs: []string{september.ChargeID},
	})
	if err != nil {
		t.Fatalf("RegisterDeposit(september): %v", err)
	}
	if got.UnallocatedCents != 0 {
		t.Errorf("left %d unallocated; the month and its surcharge should take it all", got.UnallocatedCents)
	}
}

// A month already paid whose surcharge is still open is still owed: it has a
// line (its surcharge) and can be chosen, and the due lines still add up.
func TestAPaidMonthWithAnOpenSurchargeIsStillOnTheList(t *testing.T) {
	f := depositFixture(t)
	day := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	// September in full and $100 of its $300 surcharge.
	deposit(t, f, 330000, "2026-10-10")

	expected, err := f.svc.ExpectedToday(context.Background(), testStudent, day)
	if err != nil {
		t.Fatalf("ExpectedToday: %v", err)
	}
	sep := expected.Lines[0]
	if sep.OwedCents != 0 || sep.SurchargeCents != 20000 || sep.TotalCents != 20000 || !sep.Due {
		t.Fatalf("september = %+v, want 0 owed + 200 surcharge still open", sep)
	}
	var due int64
	for _, l := range expected.Lines {
		if l.Due {
			due += l.TotalCents
		}
	}
	if due != expected.AmountCents {
		t.Errorf("due lines add up to %d, expected amount is %d", due, expected.AmountCents)
	}

	got, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 20000, Method: domain.MethodCash,
		PaidAt: day, ChargeIDs: []string{sep.ChargeID}, IdempotencyKey: "surcharge-only",
	})
	if err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}
	if got.UnallocatedCents != 0 || len(got.Allocations) != 1 {
		t.Errorf("allocations %+v, credit %d; want the open surcharge paid", got.Allocations, got.UnallocatedCents)
	}
}

// Paying a month ahead earns its early-payment reward at the counter; the list
// shows it, so the amount it fills in is what the deposit takes — no reward
// left over as credit.
func TestAMonthPaidAheadIsListedWithItsReward(t *testing.T) {
	f := prontoPagoFixture(t)
	day := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	expected, err := f.svc.ExpectedToday(context.Background(), testStudent, day)
	if err != nil {
		t.Fatalf("ExpectedToday: %v", err)
	}
	var october usecase.ExpectedLine
	for _, l := range expected.Lines {
		if strings.HasPrefix(l.Description, "Colegiatura octubre") {
			october = l
		}
	}
	if october.Due || october.DiscountCents != 10000 || october.TotalCents != 310000 {
		t.Fatalf("october = %+v, want not due, $100 reward, $3,100", october)
	}

	got, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: october.TotalCents, Method: domain.MethodCash,
		PaidAt: day, ChargeIDs: []string{october.ChargeID},
	})
	if err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}
	if got.UnallocatedCents != 0 {
		t.Errorf("left %d as credit; the listed total should settle October", got.UnallocatedCents)
	}
}

// A surcharge the office typed by hand (the "Recargo" concept, no month it
// rides on) is a debt of its own: it has a line, it can be chosen, and a
// deposit pays it — before, no deposit ever did, so a family paying the full
// quote was left with it open and the money as credit.
func TestAHandTypedSurchargeIsADebtOfItsOwn(t *testing.T) {
	f := depositFixture(t)
	day := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	lateFee := f.repo.conceptIDOfKind(domain.KindLateFee)
	manual, err := f.svc.CreateCharge(context.Background(), usecase.CreateChargeInput{
		StudentID: testStudent, CycleID: testCycle, ConceptID: lateFee,
		DueDate: "2026-10-01", AmountCentsOverride: 5000, DescriptionOverride: "Recargo por cheque devuelto",
	})
	if err != nil {
		t.Fatalf("CreateCharge: %v", err)
	}

	expected, err := f.svc.ExpectedToday(context.Background(), testStudent, day)
	if err != nil {
		t.Fatalf("ExpectedToday: %v", err)
	}
	var line usecase.ExpectedLine
	var due int64
	for _, l := range expected.Lines {
		if l.ChargeID == manual.Charge.ID() {
			line = l
		}
		if l.Due {
			due += l.TotalCents
		}
	}
	if line.TotalCents != 5000 || !line.Due {
		t.Fatalf("the hand-typed surcharge's line = %+v, want 50 due", line)
	}
	if due != expected.AmountCents {
		t.Errorf("due lines add up to %d, expected amount is %d", due, expected.AmountCents)
	}

	got, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 5000, Method: domain.MethodCash,
		PaidAt: day, ChargeIDs: []string{manual.Charge.ID()},
	})
	if err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}
	if got.UnallocatedCents != 0 || len(got.Allocations) != 1 {
		t.Errorf("allocations %+v, credit %d; want the surcharge paid", got.Allocations, got.UnallocatedCents)
	}
}

// A surcharge never earns a surcharge of its own — not even one typed by hand
// that is paid late: that is how a fixed fee turns into compound interest.
func TestAHandTypedSurchargeEarnsNoSurcharge(t *testing.T) {
	f := depositFixture(t)
	lateFee := f.repo.conceptIDOfKind(domain.KindLateFee)
	manual, err := f.svc.CreateCharge(context.Background(), usecase.CreateChargeInput{
		StudentID: testStudent, CycleID: testCycle, ConceptID: lateFee,
		DueDate: "2026-09-01", AmountCentsOverride: 5000, DescriptionOverride: "Recargo por cheque devuelto",
	})
	if err != nil {
		t.Fatalf("CreateCharge: %v", err)
	}
	got, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 100000, Method: domain.MethodCash,
		PaidAt: time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC), ChargeIDs: []string{manual.Charge.ID()},
	})
	if err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}
	if len(got.Surcharges) != 0 {
		t.Errorf("materialised %d surcharges on a surcharge", len(got.Surcharges))
	}
	if got.UnallocatedCents != 95000 {
		t.Errorf("credit = %d, want 950 left after the 50 surcharge", got.UnallocatedCents)
	}
}

// Review of task 12: an id that cannot be a deposit's is "no such receipt", not
// a database error.
func TestADepositReceiptForAMalformedIdIsNotFound(t *testing.T) {
	f := depositFixture(t)
	for _, id := range []string{"", "dep-1", "../x"} {
		if _, err := f.svc.DepositReceipt(context.Background(), id); glerrors.KindOf(err) != glerrors.KindNotFound {
			t.Errorf("DepositReceipt(%q) = %v, want KindNotFound", id, err)
		}
	}
}
