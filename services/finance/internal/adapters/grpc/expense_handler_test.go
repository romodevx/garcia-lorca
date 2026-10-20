package grpcadapter_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	grpcadapter "github.com/garcia-lorca/services/finance/internal/adapters/grpc"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// The expense handlers translate only; these tests drive a real
// ExpenseService over a small in-memory repository and read the wire.

type memExpenses struct {
	categories map[string]domain.ExpenseCategory
	expenses   map[string]domain.Expense
	events     []domain.ExpenseEvent
	closures   []domain.CashCutClosure
	changed    bool
	refs       map[string]string // C2-015: external reference -> expense id
}

func newMemExpenses() *memExpenses {
	return &memExpenses{categories: map[string]domain.ExpenseCategory{}, expenses: map[string]domain.Expense{}}
}

func (m *memExpenses) CreateExpenseCategory(_ context.Context, c domain.ExpenseCategory) (domain.ExpenseCategory, error) {
	m.categories[c.ID()] = c
	return c, nil
}

func (m *memExpenses) GetExpenseCategory(_ context.Context, id string) (domain.ExpenseCategory, error) {
	c, ok := m.categories[id]
	if !ok {
		return domain.ExpenseCategory{}, glerrors.ErrNotFound
	}
	return c, nil
}

func (m *memExpenses) ListExpenseCategories(context.Context, bool) ([]domain.ExpenseCategory, error) {
	out := make([]domain.ExpenseCategory, 0, len(m.categories))
	for _, c := range m.categories {
		out = append(out, c)
	}
	return out, nil
}

func (m *memExpenses) UpdateExpenseCategory(_ context.Context, c domain.ExpenseCategory) (domain.ExpenseCategory, error) {
	m.categories[c.ID()] = c
	return c, nil
}

// withName reads an expense back the way the database does: with its
// category's name.
func (m *memExpenses) withName(e domain.Expense) domain.Expense {
	ref, _ := e.Receipt()
	return domain.RehydrateExpense(e.ID(), domain.ExpenseDraft{
		SpentOn: e.SpentOn(), CategoryID: e.CategoryID(), AmountCents: e.Amount().Int64(), Method: e.Method(), Description: e.Description(),
	}, m.categories[e.CategoryID()].Name(), e.Status(), e.CancelReason(), e.CancelledAt(), ref, e.CreatedBy(), fixedNow, fixedNow)
}

func (m *memExpenses) CreateExpense(_ context.Context, e domain.Expense, ev domain.ExpenseEvent) (domain.Expense, error) {
	m.expenses[e.ID()] = e
	m.events = append(m.events, ev)
	return m.withName(e), nil
}

func (m *memExpenses) CreateExpenseOnce(ctx context.Context, e domain.Expense, ev domain.ExpenseEvent, externalRef string) (domain.Expense, error) {
	if m.refs == nil {
		m.refs = map[string]string{}
	}
	if id, ok := m.refs[externalRef]; ok {
		return m.withName(m.expenses[id]), nil
	}
	m.refs[externalRef] = e.ID()
	return m.CreateExpense(ctx, e, ev)
}

func (m *memExpenses) UpdateExpense(_ context.Context, id string, change ports.ExpenseChange) (domain.Expense, error) {
	cur, ok := m.expenses[id]
	if !ok {
		return domain.Expense{}, glerrors.ErrNotFound
	}
	next, ev, err := change(cur)
	if err != nil {
		return domain.Expense{}, err
	}
	m.expenses[id] = next
	m.events = append(m.events, ev)
	return m.withName(next), nil
}

func (m *memExpenses) GetExpense(_ context.Context, id string) (domain.Expense, error) {
	e, ok := m.expenses[id]
	if !ok {
		return domain.Expense{}, glerrors.ErrNotFound
	}
	return m.withName(e), nil
}

func (m *memExpenses) ListExpenses(_ context.Context, f ports.ExpenseFilter) ([]domain.Expense, error) {
	var out []domain.Expense
	for _, e := range m.expenses {
		if !e.SpentOn().Before(f.From) && !e.SpentOn().After(f.To) && (f.IncludeCancelled || !e.Cancelled()) {
			out = append(out, m.withName(e))
		}
	}
	return out, nil
}

func (m *memExpenses) ListExpenseEvents(_ context.Context, id string) ([]domain.ExpenseEvent, error) {
	var out []domain.ExpenseEvent
	for _, ev := range m.events {
		if ev.ExpenseID == id {
			out = append(out, ev)
		}
	}
	return out, nil
}

func (m *memExpenses) ExpenseTotals(context.Context, time.Time, time.Time) (domain.MethodTotals, []ports.CategoryTotal, error) {
	return domain.MethodTotals{CashCents: 300, TotalCents: 300}, []ports.CategoryTotal{{CategoryID: "c", CategoryName: "Papelería", AmountCents: 300, Count: 1}}, nil
}

func (m *memExpenses) IncomeByMethod(context.Context, time.Time, time.Time) (domain.MethodTotals, int64, error) {
	return domain.MethodTotals{CashCents: 1000, CardCents: 200, TotalCents: 1200}, 50, nil
}

func (m *memExpenses) ListCashCutClosures(context.Context, time.Time) ([]domain.CashCutClosure, error) {
	out := make([]domain.CashCutClosure, 0, len(m.closures))
	for i := len(m.closures) - 1; i >= 0; i-- {
		out = append(out, m.closures[i])
	}
	return out, nil
}

func (m *memExpenses) SaveCashCutClosure(_ context.Context, c domain.CashCutClosure) error {
	m.closures = append(m.closures, c)
	return nil
}

func (m *memExpenses) ReopenCashCutClosure(_ context.Context, c domain.CashCutClosure) error {
	for i := range m.closures {
		if m.closures[i].ID() == c.ID() {
			m.closures[i] = c
		}
	}
	return nil
}

func (m *memExpenses) DayChangedSince(context.Context, time.Time, time.Time) (bool, error) {
	return m.changed, nil
}

type memStore struct{ objects map[string][]byte }

func (s *memStore) Put(_ context.Context, key, _ string, data []byte) error {
	s.objects[key] = data
	return nil
}

func (s *memStore) Get(_ context.Context, key string) ([]byte, error) {
	d, ok := s.objects[key]
	if !ok {
		return nil, glerrors.ErrNotFound
	}
	return d, nil
}

func (s *memStore) Delete(_ context.Context, key string) error {
	delete(s.objects, key)
	return nil
}

type fixedTakings struct{}

func (fixedTakings) DailyCashUp(_ context.Context, day time.Time) (usecase.CashUp, error) {
	return usecase.CashUp{Day: day, ByMethod: map[string]int64{domain.MethodCash: 2000, domain.MethodTransfer: 500}, TotalCents: 2500, CreditNoteCents: 70}, nil
}

func newExpenseHandler(t *testing.T) (*grpcadapter.FinanceHandler, *memExpenses) {
	t.Helper()
	repo := newMemExpenses()
	n := 0
	svc := usecase.NewExpenseService(repo, fixedTakings{}, &memStore{objects: map[string][]byte{}},
		func() string { n++; return fmt.Sprintf("00000000-0000-0000-0000-%012d", n) },
		usecase.NewSchoolClock(func() time.Time { return fixedNow }, schoolZone))
	return grpcadapter.NewFinanceHandler(nil, usecase.NewHealthService("finance")).WithExpenses(svc), repo
}

func createCategory(t *testing.T, h *grpcadapter.FinanceHandler) string {
	t.Helper()
	res, err := h.CreateExpenseCategory(context.Background(), connect.NewRequest(&financev1.CreateExpenseCategoryRequest{Name: "Papelería"}))
	if err != nil {
		t.Fatalf("category: %v", err)
	}
	return res.Msg.GetCategory().GetId()
}

func createExpense(t *testing.T, h *grpcadapter.FinanceHandler, categoryID string) *financev1.Expense {
	t.Helper()
	res, err := h.CreateExpense(context.Background(), connect.NewRequest(&financev1.CreateExpenseRequest{
		SpentOn: "2026-09-15", CategoryId: categoryID, AmountCents: 300, Method: "cash",
		Description: "hojas", ActorId: "u1", ActorName: "Caja",
	}))
	if err != nil {
		t.Fatalf("expense: %v", err)
	}
	return res.Msg.GetExpense()
}

func TestCreateExpenseRoundTrip(t *testing.T) {
	h, _ := newExpenseHandler(t)
	cat := createCategory(t, h)
	e := createExpense(t, h, cat)
	if e.GetSpentOn() != "2026-09-15" || e.GetAmountCents() != 300 || e.GetMethod() != "cash" || e.GetDescription() != "hojas" {
		t.Fatalf("got %+v", e)
	}
	if e.GetCategoryName() != "Papelería" || e.GetCreatedByName() != "Caja" || e.GetStatus() != "active" || e.GetReceipt() != nil {
		t.Fatalf("got %+v", e)
	}
	got, err := h.GetExpense(context.Background(), connect.NewRequest(&financev1.GetExpenseRequest{Id: e.GetId()}))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	events := got.Msg.GetEvents()
	if len(events) != 1 || events[0].GetKind() != "created" || events[0].GetActorName() != "Caja" {
		t.Fatalf("events = %+v", events)
	}
	if events[0].GetBeforeJson() != "{}" || events[0].GetAfterJson() == "" {
		t.Fatalf("snapshots = %q / %q", events[0].GetBeforeJson(), events[0].GetAfterJson())
	}
}

func TestExpenseBadDateIsInvalidArgument(t *testing.T) {
	h, _ := newExpenseHandler(t)
	cat := createCategory(t, h)
	_, err := h.CreateExpense(context.Background(), connect.NewRequest(&financev1.CreateExpenseRequest{
		SpentOn: "15/09/2026", CategoryId: cat, AmountCents: 1, Method: "cash",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("want InvalidArgument, got %v", err)
	}
}

func TestCancelExpenseAndEditRoundTrip(t *testing.T) {
	h, _ := newExpenseHandler(t)
	cat := createCategory(t, h)
	e := createExpense(t, h, cat)
	ctx := context.Background()
	up, err := h.UpdateExpense(ctx, connect.NewRequest(&financev1.UpdateExpenseRequest{
		Id: e.GetId(), SpentOn: "2026-09-14", CategoryId: cat, AmountCents: 250, Method: "transfer", Description: "x", Reason: "error",
	}))
	if err != nil || up.Msg.GetExpense().GetAmountCents() != 250 || up.Msg.GetExpense().GetSpentOn() != "2026-09-14" {
		t.Fatalf("update: %v %+v", err, up)
	}
	c, err := h.CancelExpense(ctx, connect.NewRequest(&financev1.CancelExpenseRequest{Id: e.GetId(), Reason: "duplicado"}))
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if c.Msg.GetExpense().GetStatus() != "cancelled" || c.Msg.GetExpense().GetCancelReason() != "duplicado" || c.Msg.GetExpense().GetCancelledAt() == nil {
		t.Fatalf("got %+v", c.Msg.GetExpense())
	}
	if _, err := h.CancelExpense(ctx, connect.NewRequest(&financev1.CancelExpenseRequest{Id: e.GetId(), Reason: "otra"})); connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Fatalf("cancel twice: want AlreadyExists, got %v", err)
	}
}

func TestExpenseReceiptRoundTrip(t *testing.T) {
	h, _ := newExpenseHandler(t)
	e := createExpense(t, h, createCategory(t, h))
	ctx := context.Background()
	pdf := []byte("%PDF-1.4 ticket")
	at, err := h.AttachExpenseReceipt(ctx, connect.NewRequest(&financev1.AttachExpenseReceiptRequest{Id: e.GetId(), Data: pdf}))
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if r := at.Msg.GetExpense().GetReceipt(); r.GetContentType() != "application/pdf" || r.GetSizeBytes() != int64(len(pdf)) {
		t.Fatalf("receipt = %+v", r)
	}
	got, err := h.GetExpenseReceipt(ctx, connect.NewRequest(&financev1.GetExpenseReceiptRequest{Id: e.GetId()}))
	if err != nil || string(got.Msg.GetData()) != string(pdf) || got.Msg.GetContentType() != "application/pdf" {
		t.Fatalf("get: %v", err)
	}
	rm, err := h.RemoveExpenseReceipt(ctx, connect.NewRequest(&financev1.RemoveExpenseReceiptRequest{Id: e.GetId()}))
	if err != nil || rm.Msg.GetExpense().GetReceipt() != nil {
		t.Fatalf("remove: %v", err)
	}
}

func TestFinanceSummaryRoundTrip(t *testing.T) {
	h, _ := newExpenseHandler(t)
	res, err := h.FinanceSummary(context.Background(), connect.NewRequest(&financev1.FinanceSummaryRequest{From: "2026-09-01", To: "2026-09-30"}))
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	m := res.Msg
	if m.GetIncome().GetTotalCents() != 1200 || m.GetExpenses().GetCashCents() != 300 || m.GetBalance().GetCashCents() != 700 || m.GetBalance().GetTotalCents() != 900 {
		t.Fatalf("got %+v", m)
	}
	if m.GetCreditNoteCents() != 50 || len(m.GetExpensesByCategory()) != 1 || m.GetExpensesByCategory()[0].GetCount() != 1 {
		t.Fatalf("got %+v", m)
	}
	if _, err := h.FinanceSummary(context.Background(), connect.NewRequest(&financev1.FinanceSummaryRequest{From: "2026-09-01"})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("missing to: want InvalidArgument, got %v", err)
	}
}

func TestDailyCashUpCarriesExpensesAndTheClosure(t *testing.T) {
	h, repo := newExpenseHandler(t)
	createExpense(t, h, createCategory(t, h))
	ctx := context.Background()
	closed, err := h.CloseCashCut(ctx, connect.NewRequest(&financev1.CloseCashCutRequest{
		Day: "2026-09-15", CountedCashCents: 1650, Note: "ok", ActorId: "u1", ActorName: "Caja",
	}))
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	cl := closed.Msg.GetClosure()
	if cl.GetDay() != "2026-09-15" || cl.GetExpectedCashCents() != 1700 || cl.GetCountedCashCents() != 1650 || cl.GetDifferenceCents() != -50 {
		t.Fatalf("closure = %+v", cl)
	}
	if cl.GetClosedByName() != "Caja" || cl.GetIncome().GetTransferCents() != 500 || cl.GetCreditNoteCents() != 70 {
		t.Fatalf("closure = %+v", cl)
	}
	repo.changed = true
	res, err := h.DailyCashUp(ctx, connect.NewRequest(&financev1.DailyCashUpRequest{Day: "2026-09-15"}))
	if err != nil {
		t.Fatalf("cash-up: %v", err)
	}
	m := res.Msg
	if m.GetDay() != "2026-09-15" {
		t.Fatalf("the cut names its day: %q", m.GetDay())
	}
	if len(m.GetExpenses()) != 1 || m.GetExpensesByMethod().GetCashCents() != 300 || m.GetExpectedCashCents() != 1700 {
		t.Fatalf("got %+v", m)
	}
	if m.GetClosure().GetId() != cl.GetId() || !m.GetModifiedAfterClose() || m.GetCashCents() != 2000 {
		t.Fatalf("got %+v", m)
	}
	re, err := h.ReopenCashCut(ctx, connect.NewRequest(&financev1.ReopenCashCutRequest{Day: "2026-09-15", Reason: "pago corregido", ActorName: "Dirección"}))
	if err != nil || re.Msg.GetClosure().GetReopenedByName() != "Dirección" || re.Msg.GetClosure().GetReopenedAt() == nil {
		t.Fatalf("reopen: %v", err)
	}
	after, _ := h.DailyCashUp(ctx, connect.NewRequest(&financev1.DailyCashUpRequest{Day: "2026-09-15"}))
	if after.Msg.GetClosure() != nil || len(after.Msg.GetReopened()) != 1 || after.Msg.GetModifiedAfterClose() {
		t.Fatalf("got %+v", after.Msg)
	}
}

func TestExpensesUnwiredIsUnimplemented(t *testing.T) {
	h := grpcadapter.NewFinanceHandler(nil, usecase.NewHealthService("finance"))
	_, err := h.ListExpenseCategories(context.Background(), connect.NewRequest(&financev1.ListExpenseCategoriesRequest{}))
	if connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Fatalf("want Unimplemented, got %v", err)
	}
}
