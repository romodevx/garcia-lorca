package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/expensereport"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
	"github.com/garcia-lorca/services/gateway/internal/report"
)

const expenseID = "e1000000-0000-0000-0000-000000000001"

type fakeExpenses struct {
	input      client.ExpenseInput
	updatedID  string
	cancel     string
	by         client.Receiver
	query      client.ExpenseQuery
	uploaded   []byte
	receipt    []byte
	receiptCT  string
	closeDay   string
	counted    int64
	reopenWhy  string
	categoryIn string
	err        error
}

func (f *fakeExpenses) CreateExpenseCategory(_ context.Context, name string) (client.ExpenseCategory, error) {
	f.categoryIn = name
	return client.ExpenseCategory{ID: "c-1", Name: name, Active: true}, f.err
}

func (f *fakeExpenses) ListExpenseCategories(context.Context, bool) ([]client.ExpenseCategory, error) {
	return []client.ExpenseCategory{{ID: "c-1", Name: "Papelería", Active: true}}, f.err
}

func (f *fakeExpenses) UpdateExpenseCategory(_ context.Context, id, name string, active bool) (client.ExpenseCategory, error) {
	return client.ExpenseCategory{ID: id, Name: name, Active: active}, f.err
}

func (f *fakeExpenses) CreateExpense(_ context.Context, in client.ExpenseInput) (client.Expense, error) {
	f.input = in
	return client.Expense{ID: expenseID, AmountCents: in.AmountCents}, f.err
}

func (f *fakeExpenses) UpdateExpense(_ context.Context, id string, in client.ExpenseInput) (client.Expense, error) {
	f.updatedID, f.input = id, in
	return client.Expense{ID: id}, f.err
}

func (f *fakeExpenses) CancelExpense(_ context.Context, _ string, reason string, by client.Receiver) (client.Expense, error) {
	f.cancel, f.by = reason, by
	return client.Expense{ID: expenseID, Status: "cancelled"}, f.err
}

func (f *fakeExpenses) ListExpenses(_ context.Context, q client.ExpenseQuery) ([]client.Expense, error) {
	f.query = q
	return nil, f.err
}

func (f *fakeExpenses) GetExpense(context.Context, string) (client.Expense, []client.ExpenseEvent, error) {
	return client.Expense{ID: expenseID}, []client.ExpenseEvent{{Kind: "created", Before: json.RawMessage("{}"), After: json.RawMessage(`{"amountCents":1}`)}}, f.err
}

func (f *fakeExpenses) AttachExpenseReceipt(_ context.Context, _ string, data []byte, by client.Receiver) (client.Expense, error) {
	f.uploaded, f.by = data, by
	return client.Expense{ID: expenseID, Receipt: &client.ExpenseReceipt{ContentType: "application/pdf", SizeBytes: int64(len(data))}}, f.err
}

func (f *fakeExpenses) GetExpenseReceipt(context.Context, string) ([]byte, string, error) {
	return f.receipt, f.receiptCT, f.err
}

func (f *fakeExpenses) RemoveExpenseReceipt(context.Context, string, client.Receiver) (client.Expense, error) {
	return client.Expense{ID: expenseID}, f.err
}

func (f *fakeExpenses) FinanceSummary(_ context.Context, from, to string) (client.FinanceSummary, error) {
	f.query = client.ExpenseQuery{From: from, To: to}
	return client.FinanceSummary{Balance: client.MethodTotals{TotalCents: -5}}, f.err
}

func (f *fakeExpenses) CloseCashCut(_ context.Context, day string, counted int64, _ string, by client.Receiver) (client.CashCutClosure, error) {
	f.closeDay, f.counted, f.by = day, counted, by
	return client.CashCutClosure{Day: day, CountedCashCents: counted}, f.err
}

func (f *fakeExpenses) ReopenCashCut(_ context.Context, day, reason string, by client.Receiver) (client.CashCutClosure, error) {
	f.closeDay, f.reopenWhy, f.by = day, reason, by
	return client.CashCutClosure{Day: day, ReopenReason: reason}, f.err
}

type fakeReports struct {
	kind   string
	anchor time.Time
}

func (f *fakeReports) Report(_ context.Context, kind string, anchor time.Time) (expensereport.Report, error) {
	f.kind, f.anchor = kind, anchor
	if _, err := report.NewPeriod(kind, anchor); err != nil {
		return expensereport.Report{}, err
	}
	return expensereport.Report{Period: kind, From: "2026-10-01", To: "2026-10-31", Label: "Octubre de 2026"}, nil
}

func (f *fakeReports) Document(context.Context, expensereport.Report) (report.Document, error) {
	return report.Document{Title: "Ingresos y egresos", Tables: []report.Table{{Title: "Resumen", Columns: []report.Column{{Header: "A"}}}}}, nil
}

func newExpensesHandler(svc *fakeExpenses, reps *fakeReports) *handler.ExpensesHandler {
	return handler.NewExpensesHandler(svc, reps, fakeNamer{user: client.User{DisplayName: "Laura Caja"}},
		func() time.Time { return time.Date(2026, 10, 15, 20, 0, 0, 0, time.UTC) })
}

func expRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.SetPathValue("id", expenseID)
	return req.WithContext(middleware.WithPrincipal(req.Context(),
		middleware.Principal{UserID: "u-me", Email: "caja@colegio.mx", Permissions: []string{"expenses:write"}}))
}

func TestCreateExpenseStampsTheCaller(t *testing.T) {
	svc := &fakeExpenses{}
	rec := httptest.NewRecorder()
	newExpensesHandler(svc, &fakeReports{}).CreateExpense(rec, expRequest(http.MethodPost, "/api/v1/expenses",
		`{"spentOn":"2026-10-01","categoryId":"c1000000-0000-0000-0000-000000000001","amountCents":125050,"method":"cash","description":" Tóner ","createdBy":"u-other"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	in := svc.input
	if in.AmountCents != 125050 || in.Method != "cash" || in.Description != "Tóner" || in.SpentOn != "2026-10-01" {
		t.Fatalf("input = %+v", in)
	}
	if in.By != (client.Receiver{UserID: "u-me", Name: "Laura Caja"}) {
		t.Fatalf("by = %+v: the caller comes from the session, never the body", in.By)
	}
}

func TestCreateExpenseValidatesAtTheEdge(t *testing.T) {
	ok := `"categoryId":"c1000000-0000-0000-0000-000000000001","amountCents":100,"method":"cash","spentOn":"2026-10-01"`
	tests := map[string]string{
		"no body":        ``,
		"zero amount":    `{"categoryId":"c1000000-0000-0000-0000-000000000001","amountCents":0,"method":"cash","spentOn":"2026-10-01"}`,
		"credit note":    `{"categoryId":"c1000000-0000-0000-0000-000000000001","amountCents":1,"method":"credit_note","spentOn":"2026-10-01"}`,
		"bad date":       `{"categoryId":"c1000000-0000-0000-0000-000000000001","amountCents":1,"method":"cash","spentOn":"01/10/2026"}`,
		"bad category":   `{"categoryId":"papeleria","amountCents":1,"method":"cash","spentOn":"2026-10-01"}`,
		"long text":      `{` + ok + `,"description":"` + strings.Repeat("x", 501) + `"}`,
		"future far day": `{"categoryId":"c1000000-0000-0000-0000-000000000001","amountCents":1,"method":"cash","spentOn":"2027-01-01"}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newExpensesHandler(&fakeExpenses{}, &fakeReports{}).CreateExpense(rec, expRequest(http.MethodPost, "/api/v1/expenses", body))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
		})
	}
}

func TestUpdateAndCancelExpense(t *testing.T) {
	svc := &fakeExpenses{}
	h := newExpensesHandler(svc, &fakeReports{})
	rec := httptest.NewRecorder()
	h.UpdateExpense(rec, expRequest(http.MethodPut, "/api/v1/expenses/"+expenseID,
		`{"spentOn":"2026-10-01","categoryId":"c1000000-0000-0000-0000-000000000001","amountCents":99,"method":"card","reason":"tecleé mal"}`))
	if rec.Code != http.StatusOK || svc.updatedID != expenseID || svc.input.Reason != "tecleé mal" || svc.input.By.Name != "Laura Caja" {
		t.Fatalf("update: %d %+v", rec.Code, svc.input)
	}
	rec = httptest.NewRecorder()
	h.CancelExpense(rec, expRequest(http.MethodPost, "/api/v1/expenses/"+expenseID+"/cancel", `{"reason":"  "}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("cancel without reason: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.CancelExpense(rec, expRequest(http.MethodPost, "/api/v1/expenses/"+expenseID+"/cancel", `{"reason":"duplicado"}`))
	if rec.Code != http.StatusOK || svc.cancel != "duplicado" || svc.by.UserID != "u-me" {
		t.Fatalf("cancel: %d %q", rec.Code, svc.cancel)
	}
}

func TestExpenseIDMustBeAnID(t *testing.T) {
	req := expRequest(http.MethodGet, "/api/v1/expenses/x", "")
	req.SetPathValue("id", "../etc")
	rec := httptest.NewRecorder()
	newExpensesHandler(&fakeExpenses{}, &fakeReports{}).GetExpense(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestListExpensesNeedsARange(t *testing.T) {
	svc := &fakeExpenses{}
	h := newExpensesHandler(svc, &fakeReports{})
	rec := httptest.NewRecorder()
	h.ListExpenses(rec, expRequest(http.MethodGet, "/api/v1/expenses?from=2026-10-01", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("no to: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ListExpenses(rec, expRequest(http.MethodGet, "/api/v1/expenses?from=2026-10-01&to=2026-10-31&includeCancelled=true&categoryId=c1000000-0000-0000-0000-000000000001", ""))
	if rec.Code != http.StatusOK || !svc.query.IncludeCancelled || svc.query.CategoryID == "" {
		t.Fatalf("list: %d %+v", rec.Code, svc.query)
	}
	if !strings.Contains(rec.Body.String(), `"expenses":[]`) {
		t.Fatalf("an empty list is [], not null: %s", rec.Body.String())
	}
}

func TestReceiptUploadIsBoundedAndStamped(t *testing.T) {
	svc := &fakeExpenses{}
	h := newExpensesHandler(svc, &fakeReports{})
	rec := httptest.NewRecorder()
	big := expRequest(http.MethodPut, "/api/v1/expenses/"+expenseID+"/receipt", "")
	big.Body = http.NoBody
	big.Body = httptestBody(bytes.Repeat([]byte{'a'}, 5<<20+1))
	h.PutReceipt(rec, big)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("over 5 MB: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.PutReceipt(rec, expRequest(http.MethodPut, "/api/v1/expenses/"+expenseID+"/receipt", "%PDF-1.4"))
	if rec.Code != http.StatusOK || string(svc.uploaded) != "%PDF-1.4" || svc.by.Name != "Laura Caja" {
		t.Fatalf("upload: %d", rec.Code)
	}
}

func httptestBody(b []byte) *readCloser { return &readCloser{bytes.NewReader(b)} }

type readCloser struct{ *bytes.Reader }

func (readCloser) Close() error { return nil }

// The receipt is the uploader's file: served with its proven type, never
// sniffed, never cached, and sandboxed so a hostile PDF or image cannot run
// script against the app's origin.
func TestGetReceiptIsServedSafely(t *testing.T) {
	svc := &fakeExpenses{receipt: []byte("%PDF-1.4"), receiptCT: "application/pdf"}
	rec := httptest.NewRecorder()
	newExpensesHandler(svc, &fakeReports{}).GetReceipt(rec, expRequest(http.MethodGet, "/api/v1/expenses/"+expenseID+"/receipt", ""))
	if rec.Code != http.StatusOK || rec.Body.String() != "%PDF-1.4" {
		t.Fatalf("status = %d", rec.Code)
	}
	h := rec.Header()
	if h.Get("Content-Type") != "application/pdf" || h.Get("X-Content-Type-Options") != "nosniff" ||
		!strings.Contains(h.Get("Cache-Control"), "no-store") || !strings.Contains(h.Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("headers = %v", h)
	}
}

func TestGetReceiptMapsNotFound(t *testing.T) {
	svc := &fakeExpenses{err: connect.NewError(connect.CodeNotFound, nil)}
	rec := httptest.NewRecorder()
	newExpensesHandler(svc, &fakeReports{}).GetReceipt(rec, expRequest(http.MethodGet, "/x", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCategoriesValidateTheName(t *testing.T) {
	svc := &fakeExpenses{}
	h := newExpensesHandler(svc, &fakeReports{})
	for _, body := range []string{`{"name":"  "}`, `{"name":"` + strings.Repeat("a", 81) + `"}`} {
		rec := httptest.NewRecorder()
		h.CreateCategory(rec, expRequest(http.MethodPost, "/api/v1/expense-categories", body))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", body, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	h.CreateCategory(rec, expRequest(http.MethodPost, "/api/v1/expense-categories", `{"name":" Luz "}`))
	if rec.Code != http.StatusCreated || svc.categoryIn != "Luz" {
		t.Fatalf("create: %d %q", rec.Code, svc.categoryIn)
	}
}

func TestSummaryForwardsTheRange(t *testing.T) {
	svc := &fakeExpenses{}
	rec := httptest.NewRecorder()
	newExpensesHandler(svc, &fakeReports{}).Summary(rec, expRequest(http.MethodGet, "/api/v1/expenses/summary?from=2026-10-01&to=2026-10-31", ""))
	if rec.Code != http.StatusOK || svc.query.From != "2026-10-01" || !strings.Contains(rec.Body.String(), `"totalCents":-5`) {
		t.Fatalf("summary: %d %s", rec.Code, rec.Body.String())
	}
}

func TestCloseAndReopenTheCashCut(t *testing.T) {
	svc := &fakeExpenses{}
	h := newExpensesHandler(svc, &fakeReports{})
	rec := httptest.NewRecorder()
	h.CloseCashCut(rec, expRequest(http.MethodPost, "/api/v1/finance/cash-up/close", `{"day":"2026-10-01","countedCashCents":-1}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("negative count: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.CloseCashCut(rec, expRequest(http.MethodPost, "/api/v1/finance/cash-up/close", `{"day":"2026-10-01","countedCashCents":165000,"note":"ok","closedBy":"x"}`))
	if rec.Code != http.StatusOK || svc.closeDay != "2026-10-01" || svc.counted != 165000 || svc.by.Name != "Laura Caja" {
		t.Fatalf("close: %d %+v", rec.Code, svc)
	}
	rec = httptest.NewRecorder()
	h.ReopenCashCut(rec, expRequest(http.MethodPost, "/api/v1/finance/cash-up/reopen", `{"day":"2026-10-01"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("reopen without reason: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ReopenCashCut(rec, expRequest(http.MethodPost, "/api/v1/finance/cash-up/reopen", `{"day":"2026-10-01","reason":"pago corregido"}`))
	if rec.Code != http.StatusOK || svc.reopenWhy != "pago corregido" {
		t.Fatalf("reopen: %d", rec.Code)
	}
}

func TestReportsInThreeFormats(t *testing.T) {
	reps := &fakeReports{}
	h := newExpensesHandler(&fakeExpenses{}, reps)

	rec := httptest.NewRecorder()
	h.Report(rec, expRequest(http.MethodGet, "/api/v1/expenses/reports?period=monthly&date=2026-10-15", ""))
	if rec.Code != http.StatusOK || reps.kind != "monthly" || !strings.Contains(rec.Body.String(), "Octubre de 2026") {
		t.Fatalf("json: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.Report(rec, expRequest(http.MethodGet, "/api/v1/expenses/reports?period=hourly", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad period: %d", rec.Code)
	}
	// No date is today, on finance's (UTC) day.
	rec = httptest.NewRecorder()
	h.Report(rec, expRequest(http.MethodGet, "/api/v1/expenses/reports?period=daily", ""))
	if !reps.anchor.Equal(time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("anchor = %v", reps.anchor)
	}

	rec = httptest.NewRecorder()
	h.ReportPDF(rec, expRequest(http.MethodGet, "/api/v1/expenses/reports/pdf?period=monthly&date=2026-10-15", ""))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/pdf" ||
		!strings.Contains(rec.Header().Get("Content-Disposition"), "reporte-finanzas-monthly-2026-10-01.pdf") || !bytes.HasPrefix(rec.Body.Bytes(), []byte("%PDF-")) {
		t.Fatalf("pdf: %d %v", rec.Code, rec.Header())
	}
	rec = httptest.NewRecorder()
	h.ReportXLSX(rec, expRequest(http.MethodGet, "/api/v1/expenses/reports/xlsx?period=monthly&date=2026-10-15", ""))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" ||
		!strings.Contains(rec.Header().Get("Content-Disposition"), ".xlsx") || !bytes.HasPrefix(rec.Body.Bytes(), []byte("PK")) {
		t.Fatalf("xlsx: %d %v", rec.Code, rec.Header())
	}
}
