package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// T7 · A4: one ledger, worked out here, read by the office, the parents' app
// and the PDF. These tests pin it to the numbers the counter already quotes.

func statementOf(t *testing.T, f *fixture, student string) usecase.Statement {
	t.Helper()
	got, err := f.svc.GetStatement(context.Background(), student)
	if err != nil {
		t.Fatalf("GetStatement: %v", err)
	}
	return got
}

func familyStatementOf(t *testing.T, f *fixture, student string) usecase.Statement {
	t.Helper()
	got, err := f.svc.GetFamilyStatement(context.Background(), student)
	if err != nil {
		t.Fatalf("GetFamilyStatement: %v", err)
	}
	return got
}

func movementsOf(s usecase.Statement, kind domain.MovementKind) []domain.Movement {
	var out []domain.Movement
	for _, m := range s.Movements {
		if m.Kind == kind {
			out = append(out, m)
		}
	}
	return out
}

// The owner's scenario on 10 October: September thirty days late, October due
// today, the inscription paid. The statement carries the thirty days the daily
// rule has earned, and its "due today" is the counter's figure to the cent.
func TestTheStatementQuotesWhatTheCounterAsksFor(t *testing.T) {
	f := depositFixture(t)

	got := statementOf(t, f, testStudent)

	counter, err := f.svc.ExpectedToday(context.Background(), testStudent, f.clock)
	requireNoError(t, err)
	if got.DueTodayCents != counter.AmountCents {
		t.Errorf("due today = %d, the counter asks for %d", got.DueTodayCents, counter.AmountCents)
	}
	accrued := movementsOf(got, domain.MovementSurcharge)
	if len(accrued) != 1 || !accrued[0].Accrued || accrued[0].AmountCents != 30000 {
		t.Fatalf("accrued surcharges = %+v, want one of 30000 (thirty days at $10)", accrued)
	}
	if got := accrued[0].Date.Format("2006-01-02"); got != "2026-10-10" {
		t.Errorf("accrued surcharge dated %s, want the statement's day", got)
	}

	account, err := f.svc.GetAccount(context.Background(), testStudent)
	requireNoError(t, err)
	// The year's ten months plus the thirty days lateness has earned; the
	// inscription is paid and there is no credit. The account reads the same
	// number (T5 · ISSUE-006).
	const want = 10*320000 + 30000
	if got.Totals.BalanceCents != want || account.TotalBalanceCents != want {
		t.Errorf("balance = %d, account %d, want %d", got.Totals.BalanceCents, account.TotalBalanceCents, want)
	}
	if last := got.Movements[len(got.Movements)-1]; last.BalanceCents != got.Totals.BalanceCents {
		t.Errorf("running balance ends at %d, totals say %d", last.BalanceCents, got.Totals.BalanceCents)
	}
	if got.Totals.PaidCents != inscriptionCents {
		t.Errorf("paid = %d, want the inscription's %d", got.Totals.PaidCents, inscriptionCents)
	}
}

// A deposit is one line of money as it arrived — not one per charge it paid —
// and the surcharge it wrote down becomes a surcharge line of its own.
func TestADepositIsOneMovementWithItsFolio(t *testing.T) {
	f := depositFixture(t)
	got := deposit(t, f, 670000, "2026-10-10")

	st := statementOf(t, f, testStudent)

	var depositLines []domain.Movement
	for _, m := range movementsOf(st, domain.MovementPayment) {
		if m.DepositID != "" {
			depositLines = append(depositLines, m)
		}
	}
	if len(depositLines) != 1 {
		t.Fatalf("deposit lines = %d, want 1", len(depositLines))
	}
	if d := depositLines[0]; d.DepositID != got.Deposit.ID() || d.AmountCents != 670000 || d.Method != domain.MethodCash || d.Folio != got.Deposit.Folio() {
		t.Errorf("deposit line = %+v", d)
	}
	for _, s := range movementsOf(st, domain.MovementSurcharge) {
		if s.Accrued {
			t.Errorf("an accrued surcharge survived the deposit that wrote it down: %+v", s)
		}
	}
	account, err := f.svc.GetAccount(context.Background(), testStudent)
	requireNoError(t, err)
	if st.Totals.BalanceCents != account.TotalBalanceCents {
		t.Errorf("balance = %d, want the account's %d (T5: one number)", st.Totals.BalanceCents, account.TotalBalanceCents)
	}
}

// A voided deposit: the office sees it struck through, the family does not see
// it at all, and neither balance moves.
func TestAVoidedDepositMovesNothing(t *testing.T) {
	f := depositFixture(t)
	before := statementOf(t, f, testStudent)
	got := deposit(t, f, 100000, "2026-10-10")
	requireNoError(t, f.svc.VoidDeposit(context.Background(), got.Deposit.ID(), "importe capturado mal"))

	staff := statementOf(t, f, testStudent)
	family := familyStatementOf(t, f, testStudent)

	var voided *domain.Movement
	for i, m := range staff.Movements {
		if m.DepositID == got.Deposit.ID() {
			voided = &staff.Movements[i]
		}
	}
	if voided == nil || !voided.Voided || voided.VoidReason != "importe capturado mal" {
		t.Fatalf("the office does not see the voided deposit struck through: %+v", voided)
	}
	for _, m := range family.Movements {
		if m.DepositID == got.Deposit.ID() {
			t.Error("the family is shown a voided deposit")
		}
	}
	if staff.Totals.BalanceCents != before.Totals.BalanceCents || family.Totals.BalanceCents != before.Totals.BalanceCents {
		t.Errorf("balances moved: before %d, staff %d, family %d",
			before.Totals.BalanceCents, staff.Totals.BalanceCents, family.Totals.BalanceCents)
	}
}

// Money over the year is the family's credit: the balance goes below zero.
func TestOverpaymentIsANegativeBalance(t *testing.T) {
	f := depositFixture(t)
	deposit(t, f, 3300000, "2026-10-10")

	st := statementOf(t, f, testStudent)

	account, err := f.svc.GetAccount(context.Background(), testStudent)
	requireNoError(t, err)
	if st.Totals.BalanceCents != -account.UnallocatedCents || st.UnallocatedCents != account.UnallocatedCents ||
		account.TotalBalanceCents != st.Totals.BalanceCents {
		t.Errorf("balance = %d (credit %d), want -%d", st.Totals.BalanceCents, st.UnallocatedCents, account.UnallocatedCents)
	}
}

// T5: a charge that has not opened. The family does not see it; the office sees
// it marked, outside the balance — so both read the same balance.
func TestAChargeNotYetOpenIsMarkedForTheOfficeAndHiddenFromTheFamily(t *testing.T) {
	f := windowFixture(t)
	runOn(t, f, "2026-05-15")
	inscription := inscriptionOf(t, f, testStudent).Charge.ID()
	f.clock = ymdUTC(2026, time.May, 20)

	staff := statementOf(t, f, testStudent)
	family := familyStatementOf(t, f, testStudent)

	var marked bool
	for _, m := range staff.Movements {
		if m.ChargeID == inscription && m.Kind == domain.MovementCharge {
			marked = m.NotYetDue
		}
	}
	if !marked {
		t.Error("the office does not see the inscription marked as not yet due")
	}
	for _, m := range family.Movements {
		if m.ChargeID == inscription {
			t.Error("the family sees a charge before it opens")
		}
	}
	if staff.Totals.NotYetDueCents != inscriptionCents {
		t.Errorf("not yet due = %d, want %d", staff.Totals.NotYetDueCents, inscriptionCents)
	}
	if staff.Totals.BalanceCents != family.Totals.BalanceCents {
		t.Errorf("staff balance %d, family balance %d — the office and the family must read one number",
			staff.Totals.BalanceCents, family.Totals.BalanceCents)
	}
}

// A discount is its own line, under its charge, saying why.
func TestADiscountIsALineThatSaysWhy(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 100000)
	charge := f.seedCharge(t, concept.ID(), 100000)
	if _, err := f.svc.ApplyDiscount(context.Background(), charge.ID(), 20000, domain.ReasonLevelDiscount); err != nil {
		t.Fatalf("ApplyDiscount: %v", err)
	}

	st := statementOf(t, f, testStudent)

	discounts := movementsOf(st, domain.MovementDiscount)
	if len(discounts) != 1 || discounts[0].AmountCents != 20000 || discounts[0].ChargeID != charge.ID() {
		t.Fatalf("discounts = %+v", discounts)
	}
	if want := "Descuento de nivel · " + charge.Description(); discounts[0].Description != want {
		t.Errorf("description = %q, want %q", discounts[0].Description, want)
	}
	if st.Totals.ChargedCents != 100000 || st.Totals.DiscountCents != 20000 || st.Totals.BalanceCents != 80000 {
		t.Errorf("totals = %+v", st.Totals)
	}
}

// A cancelled charge owes nothing and is not on the statement.
func TestACancelledChargeIsNotAMovement(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Excursión", 50000)
	charge := f.seedCharge(t, concept.ID(), 50000)
	if _, err := f.svc.CancelCharge(context.Background(), charge.ID()); err != nil {
		t.Fatalf("CancelCharge: %v", err)
	}

	st := statementOf(t, f, testStudent)

	if len(st.Movements) != 0 || st.Totals != (domain.StatementTotals{}) {
		t.Errorf("a cancelled charge left %+v / %+v", st.Movements, st.Totals)
	}
}

// Review T7: a charge cancelled after money was taken against it. The money
// is not a credit — it paid for something — so the statement keeps what was
// collected on it and reads the same balance as the account.
func TestACancelledChargeWithMoneyOnItKeepsWhatWasCollected(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Excursión", 50000)
	charge := f.seedCharge(t, concept.ID(), 50000)
	pay(t, f, charge.ID(), 50000)
	if _, err := f.svc.CancelCharge(context.Background(), charge.ID()); err != nil {
		t.Fatalf("CancelCharge: %v", err)
	}

	st := statementOf(t, f, testStudent)

	account, err := f.svc.GetAccount(context.Background(), testStudent)
	requireNoError(t, err)
	if want := account.TotalBalanceCents; st.Totals.BalanceCents != want {
		t.Errorf("balance = %d, want the account's %d — the cancelled charge's money became a false credit", st.Totals.BalanceCents, want)
	}
}

// Review T7: an unpaid inscription the ladder has climbed is owed at today's
// price. The statement's balance carries the same price the counter quotes.
func TestTheStatementPricesAnUnpaidInscriptionAtTheCounters(t *testing.T) {
	f := ladderFixture(t)
	runOn(t, f, "2026-09-15")
	f.clock = aug(2026, time.November, 15)

	st := statementOf(t, f, testStudent)

	counter, err := f.svc.ExpectedToday(context.Background(), testStudent, f.clock)
	requireNoError(t, err)
	if want := counter.AnnualBalanceCents; st.Totals.BalanceCents != want {
		t.Errorf("balance = %d, the counter's year is %d", st.Totals.BalanceCents, counter.AnnualBalanceCents)
	}
}

// A reason the office typed by hand is internal: the family reads "Descuento".
func TestTheFamilyDoesNotReadAReasonTypedByTheOffice(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Colegiatura", 100000)
	charge := f.seedCharge(t, concept.ID(), 100000)
	if _, err := f.svc.ApplyDiscount(context.Background(), charge.ID(), 20000, "hijo de maestra, autorizó dirección"); err != nil {
		t.Fatalf("ApplyDiscount: %v", err)
	}

	staff := movementsOf(statementOf(t, f, testStudent), domain.MovementDiscount)
	family := movementsOf(familyStatementOf(t, f, testStudent), domain.MovementDiscount)

	if len(staff) != 1 || staff[0].Description != "hijo de maestra, autorizó dirección · "+charge.Description() {
		t.Errorf("office reads %+v", staff)
	}
	if len(family) != 1 || family[0].Description != "Descuento · "+charge.Description() {
		t.Errorf("family reads %+v", family)
	}
}
