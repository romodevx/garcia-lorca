package domain_test

import (
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

func TestMethodTotalsAdd(t *testing.T) {
	var m domain.MethodTotals
	m.Add(domain.MethodCash, 100)
	m.Add(domain.MethodCard, 50)
	m.Add(domain.MethodTransfer, 25)
	m.Add(domain.MethodCash, 10)
	if m.CashCents != 110 || m.CardCents != 50 || m.TransferCents != 25 || m.TotalCents != 185 {
		t.Fatalf("got %+v", m)
	}
	// "mixed" is a deposit's summary, never a tender's method; a credit note
	// is not money. Neither moves the figures.
	m.Add(domain.MethodCreditNote, 999)
	m.Add("mixed", 999)
	if m.TotalCents != 185 {
		t.Fatalf("non-money methods must not count: %+v", m)
	}
}

func TestMethodTotalsMinus(t *testing.T) {
	in := domain.MethodTotals{CashCents: 1000, CardCents: 500, TransferCents: 0, TotalCents: 1500}
	out := domain.MethodTotals{CashCents: 300, CardCents: 0, TransferCents: 200, TotalCents: 500}
	got := in.Minus(out)
	want := domain.MethodTotals{CashCents: 700, CardCents: 500, TransferCents: -200, TotalCents: 1000}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func cutFigures() domain.CutFigures {
	return domain.CutFigures{
		Income:          domain.MethodTotals{CashCents: 5000_00, CardCents: 1000_00, TotalCents: 6000_00},
		Expenses:        domain.MethodTotals{CashCents: 1200_00, TransferCents: 300_00, TotalCents: 1500_00},
		CreditNoteCents: 200_00,
	}
}

func TestCutFiguresExpectedCash(t *testing.T) {
	if got := cutFigures().ExpectedCashCents(); got != 3800_00 {
		t.Fatalf("expected cash = cash in − cash out, got %d", got)
	}
}

func TestCloseCashCut(t *testing.T) {
	day := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	at := time.Date(2026, 10, 1, 1, 0, 0, 0, time.UTC)
	by := domain.Receiver{UserID: "u1", Name: "Caja"}
	c, err := domain.CloseCashCut("cl-1", day, cutFigures(), 3750_00, " faltan 50 ", by, at, at)
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if c.ExpectedCashCents() != 3800_00 || c.CountedCashCents() != 3750_00 || c.DifferenceCents() != -50_00 {
		t.Fatalf("got expected %d counted %d diff %d", c.ExpectedCashCents(), c.CountedCashCents(), c.DifferenceCents())
	}
	if c.Note() != "faltan 50" || c.ClosedBy() != by || !c.ClosedAt().Equal(at) || c.Reopened() {
		t.Fatalf("got %+v", c)
	}
	if c.Figures() != cutFigures() {
		t.Fatalf("the snapshot must keep the figures as they stood")
	}
}

func TestCloseCashCutRefuses(t *testing.T) {
	day := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 9, 30, 20, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		day     time.Time
		counted int64
		note    string
	}{
		{"negative count", day, -1, ""},
		{"a future day", day.AddDate(0, 0, 1), 0, ""},
		{"no day", time.Time{}, 0, ""},
		{"note too long", day, 0, strings.Repeat("n", domain.MaxCashCutNote+1)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := domain.CloseCashCut("cl-1", tc.day, cutFigures(), tc.counted, tc.note, domain.Receiver{}, now, now); glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Fatalf("want KindInvalid, got %v", err)
			}
		})
	}
}

func TestReopenCashCut(t *testing.T) {
	day := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	at := time.Date(2026, 10, 1, 1, 0, 0, 0, time.UTC)
	c, _ := domain.CloseCashCut("cl-1", day, cutFigures(), 3800_00, "", domain.Receiver{}, at, at)
	if _, err := c.Reopen(" ", domain.Receiver{}, at); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("a reopen needs a reason, got %v", err)
	}
	later := at.Add(time.Hour)
	by := domain.Receiver{UserID: "owner", Name: "Dirección"}
	r, err := c.Reopen(" Se corrigió un pago ", by, later)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if !r.Reopened() || r.ReopenReason() != "Se corrigió un pago" || r.ReopenedBy() != by || !r.ReopenedAt().Equal(later) {
		t.Fatalf("got %+v", r)
	}
	if _, err := r.Reopen("otra", by, later); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("reopening twice is a conflict, got %v", err)
	}
}
