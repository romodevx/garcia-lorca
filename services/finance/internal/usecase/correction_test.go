package usecase_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// director is who corrects payments in these tests.
var director = domain.Receiver{UserID: "u-director", Name: "Directora Ruiz"}

func cashTender(amount int64) []usecase.TenderInput {
	return []usecase.TenderInput{{Method: domain.MethodCash, AmountCents: amount}}
}

func correct(t *testing.T, f *fixture, id string, amount int64, on, key string) usecase.CorrectionResult {
	t.Helper()
	in := usecase.CorrectionInput{
		DepositID: id, AmountCents: amount, Tenders: cashTender(amount),
		Reason: "Se capturó mal el monto", IdempotencyKey: key, By: director,
	}
	if on != "" {
		when, err := time.Parse("2006-01-02", on)
		if err != nil {
			t.Fatalf("bad test date %q: %v", on, err)
		}
		in.PaidAt = when
	}
	got, err := f.svc.CorrectDeposit(context.Background(), in)
	if err != nil {
		t.Fatalf("CorrectDeposit(%d on %s): %v", amount, on, err)
	}
	return got
}

// The owner's example: $500 was keyed when the family handed over $6,700. The
// correction pays what the money really paid, the original is kept and named,
// and the family is told the right amount.
func TestCorrectingTheAmountMovesTheMoneyAndKeepsTheOriginal(t *testing.T) {
	f := depositFixture(t)
	wrong := deposit(t, f, 50000, "2026-10-10")

	got := correct(t, f, wrong.Deposit.ID(), 670000, "", "corr-1")

	if !got.Original.Superseded() || got.Original.SupersededBy() != got.Corrected.Deposit.ID() {
		t.Errorf("original: superseded=%v by %q", got.Original.Superseded(), got.Original.SupersededBy())
	}
	if got.Corrected.Deposit.Corrects() != wrong.Deposit.ID() {
		t.Errorf("corrected names %q, want %q", got.Corrected.Deposit.Corrects(), wrong.Deposit.ID())
	}
	// The date is kept when none is given.
	if !got.Corrected.Deposit.PaidAt().Equal(wrong.Deposit.PaidAt()) {
		t.Errorf("corrected date %v, want the original's %v", got.Corrected.Deposit.PaidAt(), wrong.Deposit.PaidAt())
	}
	// September, its thirty days of surcharge, and October.
	if got.Corrected.UnallocatedCents != 0 || len(got.Corrected.Surcharges) != 1 || got.Corrected.Surcharges[0].Amount().Int64() != 30000 {
		t.Errorf("split: unallocated %d surcharges %+v", got.Corrected.UnallocatedCents, got.Corrected.Surcharges)
	}
	// Who took the money stays who took it; who corrected it is in the history.
	if got.Corrected.Deposit.ReceivedBy() != wrong.Deposit.ReceivedBy() {
		t.Error("the corrected deposit must keep who received the money")
	}
	c := got.Correction
	if c.Reason() != "Se capturó mal el monto" || c.By() != director || c.Before().AmountCents != 50000 || c.After().AmountCents != 670000 {
		t.Errorf("correction = %+v", c)
	}
	if !c.At().Equal(f.clock) {
		t.Errorf("corrected at %v, want the clock %v", c.At(), f.clock)
	}

	if len(f.publisher.corrected) != 1 {
		t.Fatalf("PaymentCorrected published %d times, want 1", len(f.publisher.corrected))
	}
	e := f.publisher.corrected[0]
	if e.AmountCents != 670000 || e.PreviousAmountCents != 50000 || e.PaymentID != got.Corrected.Deposit.ID() ||
		e.OriginalPaymentID != wrong.Deposit.ID() || e.StudentID != testStudent || e.TenantID != testTenant {
		t.Errorf("event = %+v", e)
	}
}

// The date decides the surcharge: a correction re-dates the money and the same
// rule runs again — the days of the wrong date are not kept.
func TestCorrectingTheDateReworksTheSurcharge(t *testing.T) {
	f := depositFixture(t)
	f.clock = time.Date(2026, time.October, 20, 12, 0, 0, 0, time.UTC)
	wrong := deposit(t, f, 320000, "2026-10-20")
	if len(wrong.Surcharges) != 1 {
		t.Fatalf("wrong-date deposit wrote %d surcharges, want 1", len(wrong.Surcharges))
	}

	got := correct(t, f, wrong.Deposit.ID(), 320000, "2026-10-10", "corr-date")

	if len(got.Corrected.Surcharges) != 1 || got.Corrected.Surcharges[0].Amount().Int64() != 30000 {
		t.Fatalf("corrected surcharges = %+v, want the $300 of 10 October", got.Corrected.Surcharges)
	}
	charges, err := f.svc.ListCharges(context.Background(), testStudent, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range charges {
		if c.Charge.ID() == wrong.Surcharges[0].ID() && c.Charge.Status() != domain.StatusCancelled {
			t.Errorf("the wrong date's surcharge is %s, want cancelled", c.Charge.Status())
		}
	}
}

// The early-payment reward is a function of the date: re-dated out of the
// window, the month loses it; the corrected money pays the full price.
func TestCorrectingTheDateOutOfTheWindowTakesBackTheReward(t *testing.T) {
	f := prontoPagoFixture(t)
	early := deposit(t, f, 310000, "2026-09-03")
	f.clock = time.Date(2026, time.September, 25, 12, 0, 0, 0, time.UTC)

	correct(t, f, early.Deposit.ID(), 310000, "2026-09-25", "corr-window")

	charges, err := f.svc.ListCharges(context.Background(), testStudent, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range charges {
		if c.Charge.DiscountReason() == domain.ReasonEarlyPayment {
			t.Errorf("charge %q kept the reward after the correction moved the date out of the window", c.Charge.Description())
		}
	}
}

func TestACorrectionNeedsAReasonAndChangesNothingWithout(t *testing.T) {
	f := depositFixture(t)
	wrong := deposit(t, f, 50000, "2026-10-10")

	_, err := f.svc.CorrectDeposit(context.Background(), usecase.CorrectionInput{
		DepositID: wrong.Deposit.ID(), AmountCents: 670000, Tenders: cashTender(670000), Reason: "  ", By: director,
	})
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("err = %v, want invalid", err)
	}
	stored, err := f.repo.GetDeposit(context.Background(), wrong.Deposit.ID())
	if err != nil || stored.Superseded() {
		t.Errorf("the original was touched without a reason: %v %v", stored.Superseded(), err)
	}
}

func TestACorrectionsTendersMustAddUp(t *testing.T) {
	f := depositFixture(t)
	wrong := deposit(t, f, 50000, "2026-10-10")

	_, err := f.svc.CorrectDeposit(context.Background(), usecase.CorrectionInput{
		DepositID: wrong.Deposit.ID(), AmountCents: 100000, Reason: "monto", By: director,
		Tenders: []usecase.TenderInput{
			{Method: domain.MethodCreditNote, AmountCents: 20000, Reference: "NC-1"},
			{Method: domain.MethodCash, AmountCents: 70000},
		},
	})
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("err = %v, want invalid (the methods do not add up)", err)
	}
	if _, err := f.svc.CorrectDeposit(context.Background(), usecase.CorrectionInput{
		DepositID: wrong.Deposit.ID(), AmountCents: 100000, Reason: "monto", By: director,
	}); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("no tenders: err = %v, want invalid", err)
	}
}

// A correction retried under its key is the same correction: the family is
// told once and nothing moves twice.
func TestACorrectionReplayChangesNothing(t *testing.T) {
	f := depositFixture(t)
	wrong := deposit(t, f, 50000, "2026-10-10")
	first := correct(t, f, wrong.Deposit.ID(), 670000, "", "corr-replay")

	again := correct(t, f, wrong.Deposit.ID(), 670000, "", "corr-replay")

	if !again.Replayed || again.Corrected.Deposit.ID() != first.Corrected.Deposit.ID() {
		t.Errorf("replay = %v %q, want the first correction %q", again.Replayed, again.Corrected.Deposit.ID(), first.Corrected.Deposit.ID())
	}
	if len(f.publisher.corrected) != 1 {
		t.Errorf("PaymentCorrected published %d times, want 1", len(f.publisher.corrected))
	}
}

// A receipt is corrected once — the next correction corrects its replacement —
// and a corrected receipt cannot be cancelled: its money already moved.
func TestACorrectedDepositIsNotCorrectedOrVoidedAgain(t *testing.T) {
	f := depositFixture(t)
	wrong := deposit(t, f, 50000, "2026-10-10")
	correct(t, f, wrong.Deposit.ID(), 670000, "", "corr-a")

	_, err := f.svc.CorrectDeposit(context.Background(), usecase.CorrectionInput{
		DepositID: wrong.Deposit.ID(), AmountCents: 60000, Tenders: cashTender(60000), Reason: "otra vez", By: director, IdempotencyKey: "corr-b",
	})
	if glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("second correction of the same receipt: %v, want conflict", err)
	}
	if err := f.svc.VoidDeposit(context.Background(), wrong.Deposit.ID(), "x"); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("voiding a corrected receipt: %v, want conflict", err)
	}
}

// The original day's cut no longer counts the superseded receipt, and lists it;
// the correction counts on the day it is dated.
func TestTheCashUpFollowsTheCorrection(t *testing.T) {
	f := depositFixture(t)
	wrong := deposit(t, f, 50000, "2026-10-10")
	correct(t, f, wrong.Deposit.ID(), 320000, "2026-10-09", "corr-cut")

	tenth, err := f.svc.DailyCashUp(context.Background(), time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if tenth.TotalCents != 0 || len(tenth.Superseded) != 1 || len(tenth.Deposits) != 0 {
		t.Errorf("10 Oct: total %d, superseded %d, deposits %d; want 0, 1, 0", tenth.TotalCents, len(tenth.Superseded), len(tenth.Deposits))
	}
	ninth, err := f.svc.DailyCashUp(context.Background(), time.Date(2026, time.October, 9, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if ninth.TotalCents != 320000 || ninth.ByMethod[domain.MethodCash] != 320000 {
		t.Errorf("9 Oct: total %d, want 320000", ninth.TotalCents)
	}
}

// The office reads the original struck through as superseded, with the folio
// that replaced it; the family reads only the corrected payment. The balance
// is the same for both.
func TestTheStatementShowsTheCorrection(t *testing.T) {
	f := depositFixture(t)
	wrong := deposit(t, f, 50000, "2026-10-10")
	got := correct(t, f, wrong.Deposit.ID(), 320000, "", "corr-statement")

	office, err := f.svc.GetStatement(context.Background(), testStudent)
	if err != nil {
		t.Fatal(err)
	}
	family, err := f.svc.GetFamilyStatement(context.Background(), testStudent)
	if err != nil {
		t.Fatal(err)
	}
	var superseded, corrected int
	for _, m := range office.Movements {
		if m.DepositID == wrong.Deposit.ID() {
			superseded++
			if !m.Superseded || m.SupersededByFolio != got.Corrected.Deposit.Folio() {
				t.Errorf("original line = %+v", m)
			}
		}
		if m.DepositID == got.Corrected.Deposit.ID() {
			corrected++
			if m.Superseded || m.CorrectsFolio != wrong.Deposit.Folio() {
				t.Errorf("corrected line = %+v", m)
			}
		}
	}
	if superseded != 1 || corrected != 1 {
		t.Errorf("office lines: %d superseded, %d corrected; want 1 and 1", superseded, corrected)
	}
	for _, m := range family.Movements {
		if m.DepositID == wrong.Deposit.ID() {
			t.Error("the family is shown the superseded receipt")
		}
	}
	if office.Totals.BalanceCents != family.Totals.BalanceCents {
		t.Errorf("balances differ: office %d family %d", office.Totals.BalanceCents, family.Totals.BalanceCents)
	}
	// The superseded $500 is not money received.
	if office.Totals.PaidCents != family.Totals.PaidCents {
		t.Errorf("paid differs: office %d family %d", office.Totals.PaidCents, family.Totals.PaidCents)
	}
}

// The history of a receipt is its whole chain, oldest first, each step with
// the concepts it paid named.
func TestTheCorrectionHistoryIsTheChain(t *testing.T) {
	f := depositFixture(t)
	wrong := deposit(t, f, 50000, "2026-10-10")
	first := correct(t, f, wrong.Deposit.ID(), 100000, "", "corr-h1")
	second := correct(t, f, first.Corrected.Deposit.ID(), 320000, "", "corr-h2")

	history, err := f.svc.DepositCorrections(context.Background(), second.Corrected.Deposit.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Corrections) != 2 || history.Corrections[0].OriginalID() != wrong.Deposit.ID() ||
		history.Corrections[1].CorrectedID() != second.Corrected.Deposit.ID() {
		t.Fatalf("history = %+v", history.Corrections)
	}
	line := history.Corrections[1].After().Lines[0]
	if history.Descriptions[line.ChargeID] == "" {
		t.Errorf("the concept %q is not named", line.ChargeID)
	}
	fromTheStart, err := f.svc.DepositCorrections(context.Background(), wrong.Deposit.ID())
	if err != nil || len(fromTheStart.Corrections) != 2 {
		t.Errorf("history from the original = %d (%v), want 2", len(fromTheStart.Corrections), err)
	}
}

func TestCorrectingAnUnknownDepositIsNotFound(t *testing.T) {
	f := depositFixture(t)
	_, err := f.svc.CorrectDeposit(context.Background(), usecase.CorrectionInput{
		DepositID: "not-a-uuid", AmountCents: 100, Tenders: cashTender(100), Reason: "x", By: director,
	})
	if glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("err = %v, want not found", err)
	}
}
