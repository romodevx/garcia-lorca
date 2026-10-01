package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

type flagSet map[string]bool

func (f flagSet) Enabled(_ context.Context, key string) bool { return f[key] }

func cutRequest(perms ...string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/finance/cash-up?day=2026-10-01", nil)
	return req.WithContext(middleware.WithPrincipal(req.Context(), middleware.Principal{UserID: "u", Permissions: perms}))
}

// T10: the day's expenses and closures ride on the cash-up, but only for a
// caller who may read expenses, and only with module_expenses on. The cashier
// (collections:read) keeps the takings and sees none of it.
func TestCashUpShowsExpensesOnlyToExpenseReaders(t *testing.T) {
	cut := client.CashUp{
		CashCents: 100, Expenses: []client.Expense{{ID: "e1", Description: "Nómina"}},
		ExpensesByMethod: client.MethodTotals{CashCents: 30, TotalCents: 30}, ExpectedCashCents: 70,
		Closure: &client.CashCutClosure{ID: "cl"}, ModifiedAfterClose: true, Reopened: []client.CashCutClosure{{ID: "old"}},
	}
	cases := []struct {
		name  string
		flags flagSet
		perms []string
		shown bool
	}{
		{"an expense reader with the module on", flagSet{"module_expenses": true}, []string{"collections:read", "expenses:read"}, true},
		{"the cashier", flagSet{"module_expenses": true}, []string{"collections:read"}, false},
		{"the module off", flagSet{}, []string{"finance:read", "expenses:read"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := handler.NewFinanceHandler(&fakeFinance{cashUp: cut}, handler.WithCutExtras(tc.flags))
			rec := httptest.NewRecorder()
			h.DailyCashUp(rec, cutRequest(tc.perms...))
			body := rec.Body.String()
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d", rec.Code)
			}
			if got := strings.Contains(body, "Nómina"); got != tc.shown {
				t.Fatalf("expenses shown = %v, want %v: %s", got, tc.shown, body)
			}
			if got := strings.Contains(body, `"closure"`); got != tc.shown {
				t.Fatalf("closure shown = %v, want %v", got, tc.shown)
			}
			if !tc.shown && (strings.Contains(body, `"expectedCashCents":70`) || strings.Contains(body, "old")) {
				t.Fatalf("leaked: %s", body)
			}
		})
	}
}
