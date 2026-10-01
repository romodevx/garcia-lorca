package main

import (
	"context"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

func (stubFinance) ListExpenses(context.Context, *connect.Request[financev1.ListExpensesRequest]) (*connect.Response[financev1.ListExpensesResponse], error) {
	return connect.NewResponse(&financev1.ListExpensesResponse{}), nil
}

func (stubFinance) CreateExpense(_ context.Context, req *connect.Request[financev1.CreateExpenseRequest]) (*connect.Response[financev1.CreateExpenseResponse], error) {
	return connect.NewResponse(&financev1.CreateExpenseResponse{Expense: &financev1.Expense{Id: "e1000000-0000-0000-0000-000000000001", AmountCents: req.Msg.GetAmountCents()}}), nil
}

func (stubFinance) ListExpenseCategories(context.Context, *connect.Request[financev1.ListExpenseCategoriesRequest]) (*connect.Response[financev1.ListExpenseCategoriesResponse], error) {
	return connect.NewResponse(&financev1.ListExpenseCategoriesResponse{}), nil
}

func (stubFinance) FinanceSummary(context.Context, *connect.Request[financev1.FinanceSummaryRequest]) (*connect.Response[financev1.FinanceSummaryResponse], error) {
	return connect.NewResponse(&financev1.FinanceSummaryResponse{}), nil
}

func (stubFinance) IncomeByConcept(context.Context, *connect.Request[financev1.IncomeByConceptRequest]) (*connect.Response[financev1.IncomeByConceptResponse], error) {
	return connect.NewResponse(&financev1.IncomeByConceptResponse{}), nil
}

func (stubFinance) CloseCashCut(_ context.Context, req *connect.Request[financev1.CloseCashCutRequest]) (*connect.Response[financev1.CloseCashCutResponse], error) {
	return connect.NewResponse(&financev1.CloseCashCutResponse{Closure: &financev1.CashCutClosure{Day: req.Msg.GetDay()}}), nil
}

func (stubFinance) ReopenCashCut(_ context.Context, req *connect.Request[financev1.ReopenCashCutRequest]) (*connect.Response[financev1.ReopenCashCutResponse], error) {
	return connect.NewResponse(&financev1.ReopenCashCutResponse{Closure: &financev1.CashCutClosure{Day: req.Msg.GetDay()}}), nil
}

// T10 through the real mux: the Egresos module needs module_expenses (and
// module_finance, where its data lives); reads answer to expenses:read,
// writes and closing the cut to expenses:write, reopening a closed cut to the
// director's finance:write. finance:read alone does not open the module.
func TestExpensesRoutes(t *testing.T) {
	both := map[string]bool{"module_finance": true, "module_expenses": true}
	financeOnly := map[string]bool{"module_finance": true}
	const expense = `{"spentOn":"2026-01-02","categoryId":"c1000000-0000-0000-0000-000000000001","amountCents":100,"method":"cash"}`
	cases := []struct {
		name, method, path, token, body string
		flags                           map[string]bool
		want                            int
	}{
		{"a reader lists", http.MethodGet, "/api/v1/expenses?from=2026-01-01&to=2026-01-31", "expenses-reader", "", both, http.StatusOK},
		{"finance:read is not enough", http.MethodGet, "/api/v1/expenses?from=2026-01-01&to=2026-01-31", "finance-writer", "", both, http.StatusForbidden},
		{"no token", http.MethodGet, "/api/v1/expenses?from=2026-01-01&to=2026-01-31", "", "", both, http.StatusUnauthorized},
		{"module off hides it", http.MethodGet, "/api/v1/expenses?from=2026-01-01&to=2026-01-31", "expenses-reader", "", financeOnly, http.StatusNotFound},
		{"finance off hides it", http.MethodGet, "/api/v1/expenses?from=2026-01-01&to=2026-01-31", "expenses-reader", "", map[string]bool{"module_expenses": true}, http.StatusNotFound},
		{"a writer records", http.MethodPost, "/api/v1/expenses", "expenses-writer", expense, both, http.StatusCreated},
		{"a reader cannot record", http.MethodPost, "/api/v1/expenses", "expenses-reader", expense, both, http.StatusForbidden},
		{"a reader reads categories", http.MethodGet, "/api/v1/expense-categories", "expenses-reader", "", both, http.StatusOK},
		{"a reader reads the summary", http.MethodGet, "/api/v1/expenses/summary?from=2026-01-01&to=2026-01-31", "expenses-reader", "", both, http.StatusOK},
		{"a reader reads a report", http.MethodGet, "/api/v1/expenses/reports?period=monthly&date=2026-01-15", "expenses-reader", "", both, http.StatusOK},
		{"a reader exports to Excel", http.MethodGet, "/api/v1/expenses/reports/xlsx?period=monthly&date=2026-01-15", "expenses-reader", "", both, http.StatusOK},
		{"a writer closes the cut", http.MethodPost, "/api/v1/finance/cash-up/close", "expenses-writer", `{"day":"2026-01-02","countedCashCents":0}`, both, http.StatusOK},
		{"the counter cannot close", http.MethodPost, "/api/v1/finance/cash-up/close", "cobranza", `{"day":"2026-01-02","countedCashCents":0}`, both, http.StatusForbidden},
		{"closing needs the module", http.MethodPost, "/api/v1/finance/cash-up/close", "expenses-writer", `{"day":"2026-01-02","countedCashCents":0}`, financeOnly, http.StatusNotFound},
		{"the director reopens", http.MethodPost, "/api/v1/finance/cash-up/reopen", "finance-writer", `{"day":"2026-01-02","reason":"x"}`, both, http.StatusOK},
		{"expenses:write cannot reopen", http.MethodPost, "/api/v1/finance/cash-up/reopen", "expenses-writer", `{"day":"2026-01-02","reason":"x"}`, both, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gw := newTestGateway(t, tc.flags)
			if res := doGateway(t, gw, tc.method, tc.path, tc.token, tc.body); res.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", res.StatusCode, tc.want)
			}
		})
	}
}
