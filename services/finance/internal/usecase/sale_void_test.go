package usecase_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// C2-015: a sale returned on its own day has its money voided, and the reason
// stays on the record; without one the void is refused before anything moves.
func TestVoidSalePaymentsNeedsAReason(t *testing.T) {
	f := newFixture(t)

	_, err := f.svc.VoidSalePayments(context.Background(), "d0000000-0000-4000-8000-000000000001", "   ", testActor)

	requireKind(t, err, glerrors.KindInvalid)
	if f.repo.voidSaleCalls != 0 {
		t.Fatal("a refused void reached the repository")
	}
}

// "The day of the sale" is the school's day, never the server's: at 18:30 in
// Morelia the UTC date is already tomorrow.
func TestVoidSalePaymentsJudgesTheDayByTheSchoolClock(t *testing.T) {
	f := newFixture(t)
	f.clock = time.Date(2026, time.October, 20, 0, 30, 0, 0, time.UTC) // 18:30 of the 19th in Morelia

	if _, err := f.svc.VoidSalePayments(context.Background(), "d0000000-0000-4000-8000-000000000001", "  Talla equivocada ", testActor); err != nil {
		t.Fatalf("VoidSalePayments: %v", err)
	}

	if want := time.Date(2026, time.October, 19, 0, 0, 0, 0, time.UTC); !f.repo.voidSaleToday.Equal(want) {
		t.Errorf("today = %v, want the school's %v", f.repo.voidSaleToday, want)
	}
	if f.repo.voidSaleReason != "Talla equivocada" || f.repo.voidSaleActor != testActor {
		t.Errorf("forwarded reason %q, actor %q", f.repo.voidSaleReason, f.repo.voidSaleActor)
	}
}

// The cash-up lists a voided sale as cancelled on its own day, and counts it
// for nothing: the money went back to the family.
func TestDailyCashUpListsVoidedSalesAsCancelled(t *testing.T) {
	f := newFixture(t)
	day := time.Date(2026, time.October, 19, 0, 0, 0, 0, time.UTC)
	f.repo.voidedSales = []ports.VoidedSalePayment{{
		PaymentID: "p1", ChargeID: "c1", Description: "Uniformes · U-2026-000002",
		AmountCents: 25000, Method: domain.MethodCash, PaidOn: day, Reason: "Talla equivocada",
	}}

	cut, err := f.svc.DailyCashUp(context.Background(), day)
	if err != nil {
		t.Fatalf("DailyCashUp: %v", err)
	}

	if len(cut.CancelledSales) != 1 || cut.CancelledSales[0].Reason != "Talla equivocada" {
		t.Fatalf("cancelled sales = %+v, want the voided sale", cut.CancelledSales)
	}
	if cut.TotalCents != 0 {
		t.Errorf("total = %d, a voided sale is not takings", cut.TotalCents)
	}
}
