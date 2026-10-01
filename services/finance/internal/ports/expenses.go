package ports

import (
	"context"
	"time"

	"github.com/garcia-lorca/services/finance/internal/domain"
)

// The seams of T10 (expenses and the cash cut), apart from FinanceRepository:
// a use case that records expenses has no business with deposits' locks, and
// the expense screens have no business with tuition plans.

// ExpenseFilter narrows a list of expenses: a date range (both inclusive), an
// optional category, and whether cancelled ones come too.
type ExpenseFilter struct {
	From, To         time.Time
	CategoryID       string
	IncludeCancelled bool
}

// CategoryTotal is what one category took in a period.
type CategoryTotal struct {
	CategoryID   string
	CategoryName string
	AmountCents  int64
	Count        int
}

// ExpenseChange turns the expense as it stands (read under its row lock) into
// the expense to store and the history row that explains the change.
type ExpenseChange func(current domain.Expense) (domain.Expense, domain.ExpenseEvent, error)

// ExpenseRepository keeps expenses, their categories and history, and the
// cash cut's closures.
type ExpenseRepository interface {
	// CreateExpenseCategory stores a category; a name the school already uses
	// (case-insensitive) is KindConflict.
	CreateExpenseCategory(ctx context.Context, c domain.ExpenseCategory) (domain.ExpenseCategory, error)
	// GetExpenseCategory reads one, or KindNotFound.
	GetExpenseCategory(ctx context.Context, id string) (domain.ExpenseCategory, error)
	// ListExpenseCategories lists them by name; inactive ones only when asked.
	ListExpenseCategories(ctx context.Context, includeInactive bool) ([]domain.ExpenseCategory, error)
	// UpdateExpenseCategory renames / (de)activates one; KindNotFound or
	// KindConflict (name taken).
	UpdateExpenseCategory(ctx context.Context, c domain.ExpenseCategory) (domain.ExpenseCategory, error)

	// CreateExpense stores a new expense and its "created" event in one
	// transaction, and returns it as read back (category name, timestamps).
	CreateExpense(ctx context.Context, e domain.Expense, created domain.ExpenseEvent) (domain.Expense, error)
	// UpdateExpense locks the expense, runs change on it and stores the result
	// with its event, in one transaction. KindNotFound when there is none.
	UpdateExpense(ctx context.Context, id string, change ExpenseChange) (domain.Expense, error)
	// GetExpense reads one, or KindNotFound.
	GetExpense(ctx context.Context, id string) (domain.Expense, error)
	// ListExpenses lists by the filter, newest first, at most 500.
	ListExpenses(ctx context.Context, f ExpenseFilter) ([]domain.Expense, error)
	// ListExpenseEvents is one expense's history, oldest first.
	ListExpenseEvents(ctx context.Context, expenseID string) ([]domain.ExpenseEvent, error)
	// ExpenseTotals adds up the live expenses of a period by method and by
	// category (biggest first).
	ExpenseTotals(ctx context.Context, from, to time.Time) (domain.MethodTotals, []CategoryTotal, error)
	// IncomeByMethod adds up the money received in a period — tenders of the
	// deposits that count (not voided, not superseded) and payments taken
	// outside a deposit — by method, on the day it changed hands. What credit
	// notes settled comes back apart: it is not money.
	IncomeByMethod(ctx context.Context, from, to time.Time) (domain.MethodTotals, int64, error)

	// ListCashCutClosures returns every closure of a day, newest first: the
	// live one (if any) and the reopened ones.
	ListCashCutClosures(ctx context.Context, day time.Time) ([]domain.CashCutClosure, error)
	// SaveCashCutClosure stores a new closure. A day already closed (a live
	// closure exists) is KindConflict.
	SaveCashCutClosure(ctx context.Context, c domain.CashCutClosure) error
	// ReopenCashCutClosure stores the reopen of a live closure; KindConflict
	// when it was reopened in between.
	ReopenCashCutClosure(ctx context.Context, c domain.CashCutClosure) error
	// DayChangedSince reports whether a movement dated day changed after
	// since: a deposit captured, voided or superseded, a payment taken outside
	// a deposit, or an expense created, edited, cancelled or moved to or from
	// the day.
	DayChangedSince(ctx context.Context, day, since time.Time) (bool, error)
}

// ReceiptStore keeps receipt files in a private object store (a GCS bucket;
// fake-gcs locally). The bucket is never exposed: the gateway serves the bytes
// only after its own permission check.
type ReceiptStore interface {
	Put(ctx context.Context, key, contentType string, data []byte) error
	// Get returns the object, or KindNotFound.
	Get(ctx context.Context, key string) ([]byte, error)
	// Delete removes the object; a missing one is not an error.
	Delete(ctx context.Context, key string) error
}
