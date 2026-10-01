package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/expensereport"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
	"github.com/garcia-lorca/services/gateway/internal/report"
)

// Edge bounds of the Egresos module (T10), mirroring finance's domain.
const (
	maxExpenseText     = 500
	maxCategoryName    = 80
	maxReceiptBytes    = 5 << 20
	expenseDayLayout   = "2006-01-02"
	xlsxContentType    = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	receiptSandboxCSP  = "sandbox; default-src 'none'; img-src 'self'; style-src 'unsafe-inline'"
	futureDayTolerance = 24 * time.Hour
)

var expenseMethods = map[string]bool{"cash": true, "card": true, "transfer": true}

// ExpenseService is the finance surface the Egresos module needs
// (consumer-defined; the Connect FinanceClient implements it).
type ExpenseService interface {
	CreateExpenseCategory(ctx context.Context, name string) (client.ExpenseCategory, error)
	ListExpenseCategories(ctx context.Context, includeInactive bool) ([]client.ExpenseCategory, error)
	UpdateExpenseCategory(ctx context.Context, id, name string, active bool) (client.ExpenseCategory, error)
	CreateExpense(ctx context.Context, in client.ExpenseInput) (client.Expense, error)
	UpdateExpense(ctx context.Context, id string, in client.ExpenseInput) (client.Expense, error)
	CancelExpense(ctx context.Context, id, reason string, by client.Receiver) (client.Expense, error)
	ListExpenses(ctx context.Context, q client.ExpenseQuery) ([]client.Expense, error)
	GetExpense(ctx context.Context, id string) (client.Expense, []client.ExpenseEvent, error)
	AttachExpenseReceipt(ctx context.Context, id string, data []byte, by client.Receiver) (client.Expense, error)
	GetExpenseReceipt(ctx context.Context, id string) ([]byte, string, error)
	RemoveExpenseReceipt(ctx context.Context, id string, by client.Receiver) (client.Expense, error)
	FinanceSummary(ctx context.Context, from, to string) (client.FinanceSummary, error)
	CloseCashCut(ctx context.Context, day string, countedCashCents int64, note string, by client.Receiver) (client.CashCutClosure, error)
	ReopenCashCut(ctx context.Context, day, reason string, by client.Receiver) (client.CashCutClosure, error)
}

// ReportSource builds the module's reports (internal/expensereport).
type ReportSource interface {
	Report(ctx context.Context, kind string, anchor time.Time) (expensereport.Report, error)
	Document(ctx context.Context, rep expensereport.Report) (report.Document, error)
}

// ExpensesHandler serves the Egresos module (T10): expenses, their
// categories and receipts, the period's money and its reports, and the
// closing of the day's cash cut. Every route runs behind module_finance AND
// module_expenses plus its permission (cmd/main.go). Who records, edits,
// closes or reopens is always the signed-in caller, never a body field.
type ExpensesHandler struct {
	svc     ExpenseService
	reports ReportSource
	names   UserNamer
	now     func() time.Time
}

// NewExpensesHandler wires the handler. now is injectable for tests.
func NewExpensesHandler(svc ExpenseService, reports ReportSource, names UserNamer, now func() time.Time) *ExpensesHandler {
	return &ExpensesHandler{svc: svc, reports: reports, names: names, now: now}
}

func (h *ExpensesHandler) caller(r *http.Request) client.Receiver {
	p, _ := middleware.PrincipalFrom(r.Context())
	return client.Receiver{UserID: p.UserID, Name: callerName(r.Context(), h.names, p)}
}

// expenseIDOf is the path's id when it has an id's shape; otherwise 404.
func expenseIDOf(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !uuidShape.MatchString(id) {
		http.Error(w, "expense not found", http.StatusNotFound)
		return "", false
	}
	return id, true
}

func tooLong(s string, n int) bool { return len([]rune(s)) > n }

// --- categories ------------------------------------------------------------

type categoryRequest struct {
	Name   string `json:"name"`
	Active *bool  `json:"active"`
}

func (req categoryRequest) invalid() string {
	name := strings.TrimSpace(req.Name)
	switch {
	case name == "":
		return "name is required"
	case tooLong(name, maxCategoryName):
		return "name is at most 80 characters"
	}
	return ""
}

// ListCategories handles GET /api/v1/expense-categories[?includeInactive=true].
func (h *ExpensesHandler) ListCategories(w http.ResponseWriter, r *http.Request) {
	cs, err := h.svc.ListExpenseCategories(r.Context(), r.URL.Query().Get("includeInactive") == "true")
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if cs == nil {
		cs = []client.ExpenseCategory{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": cs})
}

// CreateCategory handles POST /api/v1/expense-categories.
func (h *ExpensesHandler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var req categoryRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if msg := req.invalid(); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	c, err := h.svc.CreateExpenseCategory(r.Context(), strings.TrimSpace(req.Name))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// UpdateCategory handles PUT /api/v1/expense-categories/{id}.
func (h *ExpensesHandler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidShape.MatchString(id) {
		http.Error(w, "category not found", http.StatusNotFound)
		return
	}
	var req categoryRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if msg := req.invalid(); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	if req.Active == nil {
		http.Error(w, "active is required", http.StatusBadRequest)
		return
	}
	c, err := h.svc.UpdateExpenseCategory(r.Context(), id, strings.TrimSpace(req.Name), *req.Active)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// --- expenses --------------------------------------------------------------

type expenseRequest struct {
	SpentOn     string `json:"spentOn"`
	CategoryID  string `json:"categoryId"`
	AmountCents int64  `json:"amountCents"`
	Method      string `json:"method"`
	Description string `json:"description"`
	Reason      string `json:"reason"`
}

// invalid checks the request against the contract at the edge; every money
// and category rule is finance's.
func (h *ExpensesHandler) invalid(req expenseRequest) string {
	day, err := time.Parse(expenseDayLayout, req.SpentOn)
	switch {
	case err != nil:
		return "spentOn must be a date like 2026-10-01"
	case day.After(h.now().Add(futureDayTolerance)):
		return "spentOn cannot be in the future"
	case !uuidShape.MatchString(req.CategoryID):
		return "categoryId must be a category id"
	case req.AmountCents <= 0:
		return "amountCents must be greater than zero"
	case !expenseMethods[req.Method]:
		return "method must be cash, card or transfer"
	case tooLong(strings.TrimSpace(req.Description), maxExpenseText):
		return "description is at most 500 characters"
	case tooLong(strings.TrimSpace(req.Reason), maxExpenseText):
		return "reason is at most 500 characters"
	}
	return ""
}

func (h *ExpensesHandler) decodeExpense(w http.ResponseWriter, r *http.Request) (client.ExpenseInput, bool) {
	var req expenseRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return client.ExpenseInput{}, false
	}
	if msg := h.invalid(req); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return client.ExpenseInput{}, false
	}
	return client.ExpenseInput{
		SpentOn: req.SpentOn, CategoryID: req.CategoryID, AmountCents: req.AmountCents, Method: req.Method,
		Description: strings.TrimSpace(req.Description), Reason: strings.TrimSpace(req.Reason), By: h.caller(r),
	}, true
}

// CreateExpense handles POST /api/v1/expenses.
func (h *ExpensesHandler) CreateExpense(w http.ResponseWriter, r *http.Request) {
	in, ok := h.decodeExpense(w, r)
	if !ok {
		return
	}
	e, err := h.svc.CreateExpense(r.Context(), in)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

// UpdateExpense handles PUT /api/v1/expenses/{id}.
func (h *ExpensesHandler) UpdateExpense(w http.ResponseWriter, r *http.Request) {
	id, ok := expenseIDOf(w, r)
	if !ok {
		return
	}
	in, ok := h.decodeExpense(w, r)
	if !ok {
		return
	}
	e, err := h.svc.UpdateExpense(r.Context(), id, in)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

type reasonRequest struct {
	Reason string `json:"reason"`
}

// decodeReason reads a required reason of at most 500 characters.
func decodeReason(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req reasonRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return "", false
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		http.Error(w, "reason is required", http.StatusBadRequest)
		return "", false
	}
	if tooLong(reason, maxExpenseText) {
		http.Error(w, "reason is at most 500 characters", http.StatusBadRequest)
		return "", false
	}
	return reason, true
}

// CancelExpense handles POST /api/v1/expenses/{id}/cancel.
func (h *ExpensesHandler) CancelExpense(w http.ResponseWriter, r *http.Request) {
	id, ok := expenseIDOf(w, r)
	if !ok {
		return
	}
	reason, ok := decodeReason(w, r)
	if !ok {
		return
	}
	e, err := h.svc.CancelExpense(r.Context(), id, reason, h.caller(r))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// rangeOf reads the required from/to query pair.
func rangeOf(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	_, errFrom := time.Parse(expenseDayLayout, from)
	_, errTo := time.Parse(expenseDayLayout, to)
	if errFrom != nil || errTo != nil {
		http.Error(w, "from and to must be dates like 2026-10-01", http.StatusBadRequest)
		return "", "", false
	}
	return from, to, true
}

// ListExpenses handles GET /api/v1/expenses?from&to[&categoryId][&includeCancelled].
func (h *ExpensesHandler) ListExpenses(w http.ResponseWriter, r *http.Request) {
	from, to, ok := rangeOf(w, r)
	if !ok {
		return
	}
	category := r.URL.Query().Get("categoryId")
	if category != "" && !uuidShape.MatchString(category) {
		http.Error(w, "categoryId must be a category id", http.StatusBadRequest)
		return
	}
	es, err := h.svc.ListExpenses(r.Context(), client.ExpenseQuery{
		From: from, To: to, CategoryID: category, IncludeCancelled: r.URL.Query().Get("includeCancelled") == "true",
	})
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if es == nil {
		es = []client.Expense{}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"expenses": es})
}

// GetExpense handles GET /api/v1/expenses/{id}: the expense and its history.
func (h *ExpensesHandler) GetExpense(w http.ResponseWriter, r *http.Request) {
	id, ok := expenseIDOf(w, r)
	if !ok {
		return
	}
	e, events, err := h.svc.GetExpense(r.Context(), id)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if events == nil {
		events = []client.ExpenseEvent{}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"expense": e, "events": events})
}

// --- receipts --------------------------------------------------------------

// PutReceipt handles PUT /api/v1/expenses/{id}/receipt with the file as the
// raw body. What it IS (JPEG/PNG/PDF) is decided by finance from its bytes.
func (h *ExpensesHandler) PutReceipt(w http.ResponseWriter, r *http.Request) {
	id, ok := expenseIDOf(w, r)
	if !ok {
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxReceiptBytes))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		http.Error(w, "the receipt is larger than 5 MB", http.StatusRequestEntityTooLarge)
		return
	}
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	e, err := h.svc.AttachExpenseReceipt(r.Context(), id, data, h.caller(r))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// GetReceipt handles GET /api/v1/expenses/{id}/receipt: the file, with the
// type its bytes proved to be, never sniffed, never cached, and sandboxed —
// it is somebody's upload, served from the app's own origin.
func (h *ExpensesHandler) GetReceipt(w http.ResponseWriter, r *http.Request) {
	id, ok := expenseIDOf(w, r)
	if !ok {
		return
	}
	data, contentType, err := h.svc.GetExpenseReceipt(r.Context(), id)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	extension := map[string]string{"application/pdf": "pdf", "image/png": "png", "image/jpeg": "jpg"}[contentType]
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", "comprobante-egreso."+extension))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", receiptSandboxCSP)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data) // the status is sent; a client that hung up is not an error to report
}

// DeleteReceipt handles DELETE /api/v1/expenses/{id}/receipt.
func (h *ExpensesHandler) DeleteReceipt(w http.ResponseWriter, r *http.Request) {
	id, ok := expenseIDOf(w, r)
	if !ok {
		return
	}
	e, err := h.svc.RemoveExpenseReceipt(r.Context(), id, h.caller(r))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// --- the period's money and reports ----------------------------------------

// Summary handles GET /api/v1/expenses/summary?from&to: income, expenses and
// balance by method, and expenses by category.
func (h *ExpensesHandler) Summary(w http.ResponseWriter, r *http.Request) {
	from, to, ok := rangeOf(w, r)
	if !ok {
		return
	}
	s, err := h.svc.FinanceSummary(r.Context(), from, to)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if s.ByCategory == nil {
		s.ByCategory = []client.CategoryTotal{}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s)
}

// reportOf reads ?period=daily|weekly|monthly|yearly&date=YYYY-MM-DD (empty
// date: today, on finance's UTC day) and builds the report.
func (h *ExpensesHandler) reportOf(w http.ResponseWriter, r *http.Request) (expensereport.Report, bool) {
	anchor := h.now().UTC()
	if raw := r.URL.Query().Get("date"); raw != "" {
		parsed, err := time.Parse(expenseDayLayout, raw)
		if err != nil {
			http.Error(w, "date must be a date like 2026-10-01", http.StatusBadRequest)
			return expensereport.Report{}, false
		}
		anchor = parsed
	}
	anchor = time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 0, 0, 0, 0, time.UTC)
	if _, err := report.NewPeriod(r.URL.Query().Get("period"), anchor); err != nil {
		http.Error(w, "period must be daily, weekly, monthly or yearly", http.StatusBadRequest)
		return expensereport.Report{}, false
	}
	rep, err := h.reports.Report(r.Context(), r.URL.Query().Get("period"), anchor)
	if err != nil {
		writeConnectError(w, err)
		return expensereport.Report{}, false
	}
	return rep, true
}

// Report handles GET /api/v1/expenses/reports: the report as JSON.
func (h *ExpensesHandler) Report(w http.ResponseWriter, r *http.Request) {
	rep, ok := h.reportOf(w, r)
	if !ok {
		return
	}
	if rep.Expenses == nil {
		rep.Expenses = []client.Expense{}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, rep)
}

// ReportPDF handles GET /api/v1/expenses/reports/pdf.
func (h *ExpensesHandler) ReportPDF(w http.ResponseWriter, r *http.Request) {
	h.serveReport(w, r, "pdf", "application/pdf", report.PDF)
}

// ReportXLSX handles GET /api/v1/expenses/reports/xlsx.
func (h *ExpensesHandler) ReportXLSX(w http.ResponseWriter, r *http.Request) {
	h.serveReport(w, r, "xlsx", xlsxContentType, report.XLSX)
}

func (h *ExpensesHandler) serveReport(w http.ResponseWriter, r *http.Request, extension, contentType string, render func(report.Document) ([]byte, error)) {
	rep, ok := h.reportOf(w, r)
	if !ok {
		return
	}
	doc, err := h.reports.Document(r.Context(), rep)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	body, err := render(doc)
	if err != nil {
		http.Error(w, "the report could not be rendered", http.StatusInternalServerError)
		return
	}
	name := fmt.Sprintf("reporte-finanzas-%s-%s.%s", rep.Period, rep.From, extension)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body) // the status is sent; a client that hung up is not an error to report
}

// --- the cash cut ----------------------------------------------------------

type closeCutRequest struct {
	Day              string `json:"day"`
	CountedCashCents int64  `json:"countedCashCents"`
	Note             string `json:"note"`
}

// optionalDay checks a day that may be empty (today).
func optionalDay(raw string) bool {
	if raw == "" {
		return true
	}
	_, err := time.Parse(expenseDayLayout, raw)
	return err == nil
}

// CloseCashCut handles POST /api/v1/finance/cash-up/close: the day's cut is
// closed with the cash counted in the drawer, by the signed-in caller.
func (h *ExpensesHandler) CloseCashCut(w http.ResponseWriter, r *http.Request) {
	var req closeCutRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	switch {
	case !optionalDay(req.Day):
		http.Error(w, "day must be a date like 2026-10-01", http.StatusBadRequest)
		return
	case req.CountedCashCents < 0:
		http.Error(w, "countedCashCents cannot be negative", http.StatusBadRequest)
		return
	case tooLong(strings.TrimSpace(req.Note), maxExpenseText):
		http.Error(w, "note is at most 500 characters", http.StatusBadRequest)
		return
	}
	c, err := h.svc.CloseCashCut(r.Context(), req.Day, req.CountedCashCents, strings.TrimSpace(req.Note), h.caller(r))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

type reopenCutRequest struct {
	Day    string `json:"day"`
	Reason string `json:"reason"`
}

// ReopenCashCut handles POST /api/v1/finance/cash-up/reopen (the director's:
// finance:write). The reason is required.
func (h *ExpensesHandler) ReopenCashCut(w http.ResponseWriter, r *http.Request) {
	var req reopenCutRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	reason := strings.TrimSpace(req.Reason)
	switch {
	case !optionalDay(req.Day):
		http.Error(w, "day must be a date like 2026-10-01", http.StatusBadRequest)
		return
	case reason == "":
		http.Error(w, "reason is required", http.StatusBadRequest)
		return
	case tooLong(reason, maxExpenseText):
		http.Error(w, "reason is at most 500 characters", http.StatusBadRequest)
		return
	}
	c, err := h.svc.ReopenCashCut(r.Context(), req.Day, reason, h.caller(r))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}
