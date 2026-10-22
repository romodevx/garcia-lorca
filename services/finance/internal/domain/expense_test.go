package domain_test

import (
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

var expenseToday = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

func validDraft() domain.ExpenseDraft {
	return domain.ExpenseDraft{
		SpentOn:     expenseToday,
		CategoryID:  "cat-1",
		AmountCents: 125_050,
		Method:      domain.MethodCash,
		Description: "  Hojas y tóner  ",
	}
}

func mustExpense(t *testing.T) domain.Expense {
	t.Helper()
	e, err := domain.NewExpense("exp-1", validDraft(), expenseToday, domain.Receiver{UserID: "u1", Name: "Dirección"})
	if err != nil {
		t.Fatalf("new expense: %v", err)
	}
	return e
}

func TestNewExpenseCategory(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"trims", "  Papelería ", "Papelería", false},
		{"empty", "   ", "", true},
		{"too long", strings.Repeat("a", domain.MaxExpenseCategoryName+1), "", true},
		{"at the limit", strings.Repeat("á", domain.MaxExpenseCategoryName), strings.Repeat("á", domain.MaxExpenseCategoryName), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := domain.NewExpenseCategory("c1", tc.in, true)
			if tc.wantErr {
				if glerrors.KindOf(err) != glerrors.KindInvalid {
					t.Fatalf("want KindInvalid, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected: %v", err)
			}
			if c.Name() != tc.want || !c.Active() || c.ID() != "c1" {
				t.Fatalf("got %+v", c)
			}
		})
	}
}

func TestExpenseCategoryWithUpdate(t *testing.T) {
	c, _ := domain.NewExpenseCategory("c1", "Papelería", true)
	got, err := c.WithUpdate(" Limpieza ", false)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Name() != "Limpieza" || got.Active() || got.ID() != "c1" {
		t.Fatalf("got %+v", got)
	}
	if _, err := c.WithUpdate("", true); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("empty name must be invalid, got %v", err)
	}
}

func TestNewExpense(t *testing.T) {
	e := mustExpense(t)
	if e.ID() != "exp-1" || e.Amount().Int64() != 125_050 || e.Method() != domain.MethodCash {
		t.Fatalf("got %+v", e)
	}
	if e.Description() != "Hojas y tóner" {
		t.Fatalf("description not trimmed: %q", e.Description())
	}
	if e.Status() != domain.ExpenseActive || e.Cancelled() {
		t.Fatalf("a new expense is active")
	}
	if e.CreatedBy().Name != "Dirección" {
		t.Fatalf("creator lost: %+v", e.CreatedBy())
	}
	if _, has := e.Receipt(); has {
		t.Fatalf("a new expense has no receipt")
	}
}

func TestNewExpenseRefuses(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*domain.ExpenseDraft)
	}{
		{"zero amount", func(d *domain.ExpenseDraft) { d.AmountCents = 0 }},
		{"negative amount", func(d *domain.ExpenseDraft) { d.AmountCents = -5 }},
		{"over the money ceiling", func(d *domain.ExpenseDraft) { d.AmountCents = domain.MaxCents + 1 }},
		{"credit note is not money paid out", func(d *domain.ExpenseDraft) { d.Method = domain.MethodCreditNote }},
		{"unknown method", func(d *domain.ExpenseDraft) { d.Method = "cheque" }},
		{"no category", func(d *domain.ExpenseDraft) { d.CategoryID = " " }},
		{"no date", func(d *domain.ExpenseDraft) { d.SpentOn = time.Time{} }},
		{"a future day", func(d *domain.ExpenseDraft) { d.SpentOn = expenseToday.AddDate(0, 0, 1) }},
		{"description too long", func(d *domain.ExpenseDraft) { d.Description = strings.Repeat("x", domain.MaxExpenseDescription+1) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := validDraft()
			tc.mutate(&d)
			if _, err := domain.NewExpense("exp-1", d, expenseToday, domain.Receiver{}); glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Fatalf("want KindInvalid, got %v", err)
			}
		})
	}
}

// The day is a calendar day: an instant late on the same day is the same day.
func TestNewExpenseTruncatesToTheDay(t *testing.T) {
	d := validDraft()
	d.SpentOn = time.Date(2026, 9, 30, 17, 45, 0, 0, time.UTC)
	e, err := domain.NewExpense("exp-1", d, expenseToday.Add(10*time.Hour), domain.Receiver{})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if !e.SpentOn().Equal(time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("spentOn = %v", e.SpentOn())
	}
}

func TestExpenseEdit(t *testing.T) {
	e := mustExpense(t)
	d := validDraft()
	d.AmountCents = 99_00
	d.Method = domain.MethodTransfer
	d.SpentOn = expenseToday.AddDate(0, 0, -2)
	edited, err := e.Edit(d, expenseToday)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if edited.Amount().Int64() != 99_00 || edited.Method() != domain.MethodTransfer || !edited.SpentOn().Equal(d.SpentOn) {
		t.Fatalf("got %+v", edited)
	}
	if edited.ID() != e.ID() || edited.CreatedBy() != e.CreatedBy() {
		t.Fatalf("identity and creator must survive an edit")
	}
	if e.Amount().Int64() != 125_050 {
		t.Fatalf("Edit must not mutate the receiver")
	}
}

func TestExpenseCancel(t *testing.T) {
	e := mustExpense(t)
	at := expenseToday.Add(3 * time.Hour)
	if _, err := e.Cancel("  ", at); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("a cancel needs a reason, got %v", err)
	}
	cancelled, err := e.Cancel(" Duplicado ", at)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if !cancelled.Cancelled() || cancelled.CancelReason() != "Duplicado" || !cancelled.CancelledAt().Equal(at) {
		t.Fatalf("got %+v", cancelled)
	}
	if _, err := cancelled.Cancel("otra vez", at); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("cancelling twice is a conflict, got %v", err)
	}
	if _, err := cancelled.Edit(validDraft(), expenseToday); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("editing a cancelled expense is a conflict, got %v", err)
	}
	if _, err := cancelled.WithReceipt(domain.ExpenseReceiptRef{Key: "k", ContentType: "image/png", SizeBytes: 3}); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("attaching to a cancelled expense is a conflict, got %v", err)
	}
}

func TestExpenseReceiptRef(t *testing.T) {
	e := mustExpense(t)
	ref := domain.ExpenseReceiptRef{Key: "tenant/expenses/exp-1/r1", ContentType: "application/pdf", SizeBytes: 1200}
	with, err := e.WithReceipt(ref)
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	got, has := with.Receipt()
	if !has || got != ref {
		t.Fatalf("receipt = %+v, %v", got, has)
	}
	if _, has := with.WithoutReceipt().Receipt(); has {
		t.Fatalf("WithoutReceipt keeps nothing")
	}
}
