package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/garcia-lorca/services/finance/internal/domain"
)

// What the school earned, by what it charged for. The director's question is
// "how much came in for colegiaturas this month?" — and until now the answer
// was a spreadsheet somebody built by hand.
func TestIncomeIsGroupedByConcept(t *testing.T) {
	f := depositFixture(t)
	deposit(t, f, 670000, "2026-10-10")

	income, err := f.svc.IncomeByConcept(context.Background(),
		time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.October, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("IncomeByConcept: %v", err)
	}

	if income.TotalCents != 670000 {
		t.Errorf("total = %d, want 670000", income.TotalCents)
	}
	byName := map[string]int64{}
	for _, line := range income.Lines {
		byName[line.Concept] = line.AmountCents
	}
	// September's tuition, October's, and the surcharge September earned.
	if byName["Colegiatura"] != 640000 {
		t.Errorf("colegiatura = %d, want 640000", byName["Colegiatura"])
	}
	if byName["Recargo por pago tardío"] != 30000 {
		t.Errorf("recargo = %d, want 30000 · %+v", byName["Recargo por pago tardío"], income.Lines)
	}
}

// Money received outside the window is another month's income. Counting it
// twice is how a school convinces itself it had a good October.
func TestIncomeCountsOnlyThePeriod(t *testing.T) {
	f := depositFixture(t)
	deposit(t, f, 320000, "2026-10-10")

	income, err := f.svc.IncomeByConcept(context.Background(),
		time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.November, 30, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("IncomeByConcept: %v", err)
	}
	if income.TotalCents != 0 {
		t.Errorf("November income = %d, want 0 — that money came in in October", income.TotalCents)
	}
}

// The lines are ordered by what earned most. A director reading a report wants
// the big number first, not an alphabetical list.
func TestIncomeLeadsWithTheBiggestLine(t *testing.T) {
	f := depositFixture(t)
	deposit(t, f, 670000, "2026-10-10")

	income, err := f.svc.IncomeByConcept(context.Background(),
		time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.October, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("IncomeByConcept: %v", err)
	}
	if len(income.Lines) < 2 {
		t.Fatalf("lines = %d, want at least 2", len(income.Lines))
	}
	if income.Lines[0].AmountCents < income.Lines[1].AmountCents {
		t.Errorf("lines are not ordered by size: %+v", income.Lines)
	}
	if income.Lines[0].Concept != domain.KindTuition && income.Lines[0].AmountCents != 640000 {
		t.Errorf("the biggest line is %+v", income.Lines[0])
	}
}
