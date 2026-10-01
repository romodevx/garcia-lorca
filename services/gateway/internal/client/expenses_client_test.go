package client

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

// T10, both directions of the wire.
func TestExpenseCrossesTheWireBothWays(t *testing.T) {
	in := ExpenseInput{
		SpentOn: "2026-10-01", CategoryID: "cat-1", AmountCents: 125050, Method: "cash",
		Description: "Tóner", Reason: "ajuste", By: Receiver{UserID: "u-1", Name: "Caja"},
	}
	c := toCreateExpenseRequest(in)
	if c.GetSpentOn() != "2026-10-01" || c.GetCategoryId() != "cat-1" || c.GetAmountCents() != 125050 || c.GetMethod() != "cash" ||
		c.GetDescription() != "Tóner" || c.GetActorId() != "u-1" || c.GetActorName() != "Caja" {
		t.Errorf("create = %+v", c)
	}
	u := toUpdateExpenseRequest("e-1", in)
	if u.GetId() != "e-1" || u.GetReason() != "ajuste" || u.GetAmountCents() != 125050 || u.GetActorName() != "Caja" || u.GetSpentOn() != "2026-10-01" {
		t.Errorf("update = %+v", u)
	}

	at := timestamppb.New(time.Date(2026, 10, 1, 15, 0, 0, 0, time.UTC))
	got := toExpense(&financev1.Expense{
		Id: "e-1", SpentOn: "2026-10-01", CategoryId: "cat-1", CategoryName: "Papelería", AmountCents: 125050,
		Method: "card", Description: "x", Status: "cancelled", CancelReason: "dup", CancelledAt: at,
		Receipt: &financev1.ExpenseReceipt{ContentType: "application/pdf", SizeBytes: 9}, CreatedByName: "Caja", CreatedAt: at, UpdatedAt: at,
	})
	if got.ID != "e-1" || got.CategoryName != "Papelería" || got.Status != "cancelled" || got.CancelReason != "dup" ||
		got.CancelledAt == "" || got.Receipt == nil || got.Receipt.SizeBytes != 9 || got.CreatedByName != "Caja" || got.CreatedAt == "" || got.UpdatedAt == "" {
		t.Errorf("expense = %+v", got)
	}
	if toExpense(&financev1.Expense{Id: "e-2"}).Receipt != nil {
		t.Error("no receipt must stay nil")
	}
	ev := toExpenseEvent(&financev1.ExpenseEvent{Id: "v", Kind: "edited", BeforeJson: `{"amountCents":1}`, AfterJson: `{"amountCents":2}`, Reason: "r", ActorName: "Caja", At: at})
	if ev.Kind != "edited" || string(ev.Before) != `{"amountCents":1}` || string(ev.After) != `{"amountCents":2}` || ev.ActorName != "Caja" || ev.At == "" {
		t.Errorf("event = %+v", ev)
	}
}

func TestTheCashUpCarriesExpensesAndTheClosure(t *testing.T) {
	at := timestamppb.New(time.Date(2026, 10, 1, 23, 0, 0, 0, time.UTC))
	closure := &financev1.CashCutClosure{
		Id: "cl-1", Day: "2026-10-01", ExpectedCashCents: 1700, CountedCashCents: 1650, DifferenceCents: -50,
		Income: &financev1.MethodTotals{CashCents: 2000, TotalCents: 2000}, Expenses: &financev1.MethodTotals{CashCents: 300, TotalCents: 300},
		CreditNoteCents: 70, Note: "n", ClosedByName: "Caja", ClosedAt: at, ReopenedAt: at, ReopenedByName: "Dir", ReopenReason: "r",
	}
	cut := toCashUp(&financev1.DailyCashUpResponse{
		Expenses:          []*financev1.Expense{{Id: "e-1"}},
		ExpensesByMethod:  &financev1.MethodTotals{CashCents: 300, CardCents: 1, TransferCents: 2, TotalCents: 303},
		ExpectedCashCents: 1700, Closure: closure, ModifiedAfterClose: true,
		Reopened: []*financev1.CashCutClosure{closure}, Day: "2026-10-01",
	})
	if cut.Day != "2026-10-01" {
		t.Errorf("day = %q", cut.Day)
	}
	if len(cut.Expenses) != 1 || cut.ExpensesByMethod.TotalCents != 303 || cut.ExpensesByMethod.TransferCents != 2 || cut.ExpectedCashCents != 1700 || !cut.ModifiedAfterClose {
		t.Errorf("cut = %+v", cut)
	}
	c := cut.Closure
	if c == nil || c.ID != "cl-1" || c.Day != "2026-10-01" || c.CountedCashCents != 1650 || c.DifferenceCents != -50 || c.Income.CashCents != 2000 ||
		c.Expenses.CashCents != 300 || c.CreditNoteCents != 70 || c.Note != "n" || c.ClosedByName != "Caja" || c.ClosedAt == "" ||
		c.ReopenedAt == "" || c.ReopenedByName != "Dir" || c.ReopenReason != "r" || c.ExpectedCashCents != 1700 {
		t.Errorf("closure = %+v", c)
	}
	if len(cut.Reopened) != 1 {
		t.Errorf("reopened = %+v", cut.Reopened)
	}
	if toCashUp(&financev1.DailyCashUpResponse{}).Closure != nil {
		t.Error("an open day has no closure")
	}
	if toCashUp(&financev1.DailyCashUpResponse{}).Expenses == nil {
		t.Error("expenses must be [] on the wire, never null")
	}
}

func TestSummaryCrossesTheWire(t *testing.T) {
	s := toFinanceSummary(&financev1.FinanceSummaryResponse{
		Income: &financev1.MethodTotals{CashCents: 10, CardCents: 20, TransferCents: 30, TotalCents: 60}, CreditNoteCents: 5,
		Expenses: &financev1.MethodTotals{CashCents: 1, TotalCents: 1}, Balance: &financev1.MethodTotals{CashCents: 9, TotalCents: 59},
		ExpensesByCategory: []*financev1.CategoryTotal{{CategoryId: "c", CategoryName: "Luz", AmountCents: 1, Count: 1}},
	})
	if s.Income.TransferCents != 30 || s.CreditNoteCents != 5 || s.Expenses.CashCents != 1 || s.Balance.TotalCents != 59 ||
		len(s.ByCategory) != 1 || s.ByCategory[0].CategoryName != "Luz" || s.ByCategory[0].Count != 1 {
		t.Errorf("summary = %+v", s)
	}
}
