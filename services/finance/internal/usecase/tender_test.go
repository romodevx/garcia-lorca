package usecase_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// T8 — a credit note the school issued by hand, completed with cash.

var oct10 = time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC)

func tenderedDeposit(t *testing.T, f *fixture, amount int64, key string, tenders ...usecase.TenderInput) (usecase.DepositResult, error) {
	t.Helper()
	return f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: amount, PaidAt: oct10,
		IdempotencyKey: key, Tenders: tenders,
	})
}

// The family owes September ($3,200) and its thirty days ($300): a $2,000
// credit note covers part, and $1,500 in cash completes it. Every payment row
// carries exactly one method — September is split where the note runs out.
func TestACreditNoteCompletedWithCashSplitsTheAllocations(t *testing.T) {
	f := depositFixture(t)

	got, err := tenderedDeposit(t, f, 350000, "nc-1",
		usecase.TenderInput{Method: domain.MethodCreditNote, AmountCents: 200000, Reference: "NC-7"},
		usecase.TenderInput{Method: domain.MethodCash, AmountCents: 150000},
	)
	if err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}
	if got.Deposit.Method() != domain.MethodMixed || len(got.Deposit.Tenders()) != 2 {
		t.Errorf("deposit = %s with %d tenders, want mixed with 2", got.Deposit.Method(), len(got.Deposit.Tenders()))
	}
	byMethod := map[string]int64{}
	for _, p := range got.Allocations {
		byMethod[p.Method()] += p.Amount().Int64()
	}
	if byMethod[domain.MethodCreditNote] != 200000 || byMethod[domain.MethodCash] != 150000 {
		t.Errorf("allocations by method = %v, want 200000 credit note + 150000 cash", byMethod)
	}
	for _, p := range got.Allocations {
		if p.Method() == domain.MethodCreditNote && p.Reference() != "NC-7" {
			t.Errorf("the credit note slice lost its reference: %q", p.Reference())
		}
	}
}

// The strict rule: methods that do not add up to the amount to pay are refused,
// and nothing is written.
func TestTendersThatDoNotAddUpAreRefused(t *testing.T) {
	f := depositFixture(t)

	_, err := tenderedDeposit(t, f, 350000, "nc-short",
		usecase.TenderInput{Method: domain.MethodCreditNote, AmountCents: 200000},
		usecase.TenderInput{Method: domain.MethodCash, AmountCents: 100000},
	)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("want KindInvalid, got %v", err)
	}
	if len(f.repo.deposits) != 0 {
		t.Errorf("a refused payment stored %d deposits", len(f.repo.deposits))
	}
}

// The same attempt retried is the same payment: idempotency still holds.
func TestATenderedDepositReplays(t *testing.T) {
	f := depositFixture(t)
	tenders := []usecase.TenderInput{
		{Method: domain.MethodCreditNote, AmountCents: 200000},
		{Method: domain.MethodCard, AmountCents: 150000, CardType: domain.CardDebit},
	}
	if _, err := tenderedDeposit(t, f, 350000, "nc-retry", tenders...); err != nil {
		t.Fatalf("first: %v", err)
	}
	again, err := tenderedDeposit(t, f, 350000, "nc-retry", tenders...)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !again.Replayed || len(f.repo.deposits) != 1 {
		t.Errorf("replayed = %v with %d deposits, want one deposit replayed", again.Replayed, len(f.repo.deposits))
	}
}

// The cash-up counts money. A credit note is reported on its own line and is
// in none of the money totals; card and cash count only their own part.
func TestTheCashUpKeepsCreditNotesOutOfTheTakings(t *testing.T) {
	f := depositFixture(t)
	if _, err := tenderedDeposit(t, f, 350000, "nc-cut",
		usecase.TenderInput{Method: domain.MethodCreditNote, AmountCents: 200000},
		usecase.TenderInput{Method: domain.MethodCash, AmountCents: 150000},
	); err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}

	cut, err := f.svc.DailyCashUp(context.Background(), oct10)
	if err != nil {
		t.Fatalf("DailyCashUp: %v", err)
	}
	if cut.TotalCents != 150000 {
		t.Errorf("total = %d, want 150000 — the credit note brought no money", cut.TotalCents)
	}
	if cut.ByMethod[domain.MethodCash] != 150000 {
		t.Errorf("cash = %d, want 150000", cut.ByMethod[domain.MethodCash])
	}
	if cut.CreditNoteCents != 200000 {
		t.Errorf("credit notes = %d, want 200000 on their own line", cut.CreditNoteCents)
	}
	if _, inMethods := cut.ByMethod[domain.MethodCreditNote]; inMethods {
		t.Error("the credit note leaked into the money by method")
	}
}

// Income by concept is money. What a credit note settled is reported apart.
func TestIncomeReportsCreditNotesApart(t *testing.T) {
	f := depositFixture(t)
	if _, err := tenderedDeposit(t, f, 350000, "nc-income",
		usecase.TenderInput{Method: domain.MethodCreditNote, AmountCents: 200000},
		usecase.TenderInput{Method: domain.MethodCash, AmountCents: 150000},
	); err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}

	income, err := f.svc.IncomeByConcept(context.Background(),
		time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.October, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("IncomeByConcept: %v", err)
	}
	if income.TotalCents != 150000 || income.CreditNoteCents != 200000 {
		t.Errorf("income = %d, credit notes = %d; want 150000 and 200000", income.TotalCents, income.CreditNoteCents)
	}
	var sum int64
	for _, l := range income.Lines {
		sum += l.AmountCents
	}
	if sum != income.TotalCents {
		t.Errorf("lines add up to %d, total says %d", sum, income.TotalCents)
	}
}

// The statement shows the credit note as what it is: a credit_note line, beside
// the cash that completed the payment. The balance moves by both.
func TestTheStatementShowsTheCreditNoteAsItsOwnLine(t *testing.T) {
	f := depositFixture(t)
	if _, err := tenderedDeposit(t, f, 350000, "nc-statement",
		usecase.TenderInput{Method: domain.MethodCreditNote, AmountCents: 200000, Reference: "NC-7"},
		usecase.TenderInput{Method: domain.MethodCash, AmountCents: 150000},
	); err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}

	st, err := f.svc.GetStatement(context.Background(), testStudent)
	if err != nil {
		t.Fatalf("GetStatement: %v", err)
	}
	var note, cash int64
	for _, m := range st.Movements {
		switch {
		case m.Kind == domain.MovementCreditNote:
			note += m.AmountCents
			if m.Reference != "NC-7" || m.Method != domain.MethodCreditNote {
				t.Errorf("credit note line = %+v", m)
			}
		case m.Kind == domain.MovementPayment && m.Method == domain.MethodCash && m.DepositID != "":
			cash += m.AmountCents
		}
	}
	if note != 200000 || cash != 150000 {
		t.Errorf("credit note = %d, cash = %d; want 200000 and 150000", note, cash)
	}
}

// A payment straight against one charge (Finanzas' Registrar pago, a uniforms
// sale) combines methods too: one payment row per tender, atomically.
func TestARegisteredPaymentCombinesTenders(t *testing.T) {
	f := depositFixture(t)
	open := firstOpenCharge(t, f)

	payments, charge, err := f.svc.RegisterTenderedPayment(context.Background(), usecase.TenderedPaymentInput{
		ChargeID: open.Charge.ID(), AmountCents: 100000, IdempotencyKey: "pay-mix",
		Tenders: []usecase.TenderInput{
			{Method: domain.MethodCreditNote, AmountCents: 40000},
			{Method: domain.MethodTransfer, AmountCents: 60000, Reference: "SPEI-1"},
		},
	})
	if err != nil {
		t.Fatalf("RegisterTenderedPayment: %v", err)
	}
	if len(payments) != 2 || payments[0].Method() != domain.MethodCreditNote || payments[1].Reference() != "SPEI-1" {
		t.Fatalf("payments = %+v", payments)
	}
	if charge.PaidCents != open.PaidCents+100000 {
		t.Errorf("paid = %d, want %d", charge.PaidCents, open.PaidCents+100000)
	}

	again, _, err := f.svc.RegisterTenderedPayment(context.Background(), usecase.TenderedPaymentInput{
		ChargeID: open.Charge.ID(), AmountCents: 100000, IdempotencyKey: "pay-mix",
		Tenders: []usecase.TenderInput{
			{Method: domain.MethodCreditNote, AmountCents: 40000},
			{Method: domain.MethodTransfer, AmountCents: 60000, Reference: "SPEI-1"},
		},
	})
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if len(again) != 2 || f.repo.paidFor(open.Charge.ID()) != open.PaidCents+100000 {
		t.Errorf("a retry collected again: %d rows, paid %d", len(again), f.repo.paidFor(open.Charge.ID()))
	}

	if _, _, err := f.svc.RegisterTenderedPayment(context.Background(), usecase.TenderedPaymentInput{
		ChargeID: open.Charge.ID(), AmountCents: 50000,
		Tenders:  []usecase.TenderInput{{Method: domain.MethodCash, AmountCents: 40000}},
	}); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("tenders short of the amount: want KindInvalid, got %v", err)
	}
}

func firstOpenCharge(t *testing.T, f *fixture) ports.ChargeWithPaid {
	t.Helper()
	charges, err := f.svc.ListCharges(context.Background(), testStudent, "")
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	for _, c := range charges {
		if c.Charge.Effective().Int64()-c.PaidCents > 100000 && c.Charge.Status() != domain.StatusCancelled {
			return c
		}
	}
	t.Fatal("the fixture left no charge open")
	return ports.ChargeWithPaid{}
}
