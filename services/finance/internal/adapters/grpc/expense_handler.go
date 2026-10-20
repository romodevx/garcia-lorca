package grpcadapter

import (
	"context"
	"encoding/json"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// dayLayout is the wire format of a calendar day.
const dayLayout = "2006-01-02"

// WithExpenses wires the expenses and the cash cut (T10). Without it those
// RPCs answer Unimplemented and DailyCashUp is the takings alone.
func (h *FinanceHandler) WithExpenses(svc *usecase.ExpenseService) *FinanceHandler {
	h.expenses = svc
	return h
}

var errExpensesUnwired = connect.NewError(connect.CodeUnimplemented, glerrors.New(glerrors.KindInternal, "expenses are not enabled"))

// parseDay reads a required YYYY-MM-DD.
func parseDay(raw string) (time.Time, error) {
	d, err := time.Parse(dayLayout, raw)
	if err != nil {
		return time.Time{}, glerrors.New(glerrors.KindInvalid, "dates must be YYYY-MM-DD")
	}
	return d, nil
}

// parseOptionalDay reads YYYY-MM-DD; empty is the zero time (today, for the
// use case).
func parseOptionalDay(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	return parseDay(raw)
}

func actorOf(id, name string) domain.Receiver { return domain.Receiver{UserID: id, Name: name} }

// --- categories ------------------------------------------------------------

// CreateExpenseCategory adds a category.
func (h *FinanceHandler) CreateExpenseCategory(ctx context.Context, req *connect.Request[financev1.CreateExpenseCategoryRequest]) (*connect.Response[financev1.CreateExpenseCategoryResponse], error) {
	if h.expenses == nil {
		return nil, errExpensesUnwired
	}
	c, err := h.expenses.CreateCategory(ctx, req.Msg.GetName())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.CreateExpenseCategoryResponse{Category: toProtoCategory(c)}), nil
}

// ListExpenseCategories lists the categories.
func (h *FinanceHandler) ListExpenseCategories(ctx context.Context, req *connect.Request[financev1.ListExpenseCategoriesRequest]) (*connect.Response[financev1.ListExpenseCategoriesResponse], error) {
	if h.expenses == nil {
		return nil, errExpensesUnwired
	}
	cs, err := h.expenses.ListCategories(ctx, req.Msg.GetIncludeInactive())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*financev1.ExpenseCategory, 0, len(cs))
	for _, c := range cs {
		out = append(out, toProtoCategory(c))
	}
	return connect.NewResponse(&financev1.ListExpenseCategoriesResponse{Categories: out}), nil
}

// UpdateExpenseCategory renames / (de)activates a category.
func (h *FinanceHandler) UpdateExpenseCategory(ctx context.Context, req *connect.Request[financev1.UpdateExpenseCategoryRequest]) (*connect.Response[financev1.UpdateExpenseCategoryResponse], error) {
	if h.expenses == nil {
		return nil, errExpensesUnwired
	}
	m := req.Msg
	c, err := h.expenses.UpdateCategory(ctx, m.GetId(), m.GetName(), m.GetActive())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.UpdateExpenseCategoryResponse{Category: toProtoCategory(c)}), nil
}

func toProtoCategory(c domain.ExpenseCategory) *financev1.ExpenseCategory {
	return &financev1.ExpenseCategory{Id: c.ID(), Name: c.Name(), Active: c.Active()}
}

// --- expenses --------------------------------------------------------------

func draftOf(spentOn, categoryID string, amountCents int64, method, description string) (domain.ExpenseDraft, error) {
	day, err := parseDay(spentOn)
	if err != nil {
		return domain.ExpenseDraft{}, err
	}
	return domain.ExpenseDraft{SpentOn: day, CategoryID: categoryID, AmountCents: amountCents, Method: method, Description: description}, nil
}

// CreateExpense records an expense.
func (h *FinanceHandler) CreateExpense(ctx context.Context, req *connect.Request[financev1.CreateExpenseRequest]) (*connect.Response[financev1.CreateExpenseResponse], error) {
	if h.expenses == nil {
		return nil, errExpensesUnwired
	}
	m := req.Msg
	d, err := draftOf(m.GetSpentOn(), m.GetCategoryId(), m.GetAmountCents(), m.GetMethod(), m.GetDescription())
	if err != nil {
		return nil, connectErr(err)
	}
	e, err := h.expenses.CreateExpenseOnce(ctx, d, actorOf(m.GetActorId(), m.GetActorName()), m.GetExternalRef())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.CreateExpenseResponse{Expense: toProtoExpense(e)}), nil
}

// UpdateExpense edits an expense.
func (h *FinanceHandler) UpdateExpense(ctx context.Context, req *connect.Request[financev1.UpdateExpenseRequest]) (*connect.Response[financev1.UpdateExpenseResponse], error) {
	if h.expenses == nil {
		return nil, errExpensesUnwired
	}
	m := req.Msg
	d, err := draftOf(m.GetSpentOn(), m.GetCategoryId(), m.GetAmountCents(), m.GetMethod(), m.GetDescription())
	if err != nil {
		return nil, connectErr(err)
	}
	e, err := h.expenses.UpdateExpense(ctx, m.GetId(), d, m.GetReason(), actorOf(m.GetActorId(), m.GetActorName()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.UpdateExpenseResponse{Expense: toProtoExpense(e)}), nil
}

// CancelExpense cancels an expense.
func (h *FinanceHandler) CancelExpense(ctx context.Context, req *connect.Request[financev1.CancelExpenseRequest]) (*connect.Response[financev1.CancelExpenseResponse], error) {
	if h.expenses == nil {
		return nil, errExpensesUnwired
	}
	m := req.Msg
	e, err := h.expenses.CancelExpense(ctx, m.GetId(), m.GetReason(), actorOf(m.GetActorId(), m.GetActorName()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.CancelExpenseResponse{Expense: toProtoExpense(e)}), nil
}

// ListExpenses lists a range's expenses.
func (h *FinanceHandler) ListExpenses(ctx context.Context, req *connect.Request[financev1.ListExpensesRequest]) (*connect.Response[financev1.ListExpensesResponse], error) {
	if h.expenses == nil {
		return nil, errExpensesUnwired
	}
	m := req.Msg
	from, err := parseDay(m.GetFrom())
	if err != nil {
		return nil, connectErr(err)
	}
	to, err := parseDay(m.GetTo())
	if err != nil {
		return nil, connectErr(err)
	}
	es, err := h.expenses.ListExpenses(ctx, usecase.ExpenseQuery{
		From: from, To: to, CategoryID: m.GetCategoryId(), IncludeCancelled: m.GetIncludeCancelled(),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.ListExpensesResponse{Expenses: toProtoExpenses(es)}), nil
}

// GetExpense returns an expense and its history.
func (h *FinanceHandler) GetExpense(ctx context.Context, req *connect.Request[financev1.GetExpenseRequest]) (*connect.Response[financev1.GetExpenseResponse], error) {
	if h.expenses == nil {
		return nil, errExpensesUnwired
	}
	e, events, err := h.expenses.GetExpense(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*financev1.ExpenseEvent, 0, len(events))
	for _, ev := range events {
		pe, err := toProtoExpenseEvent(ev)
		if err != nil {
			return nil, connectErr(err)
		}
		out = append(out, pe)
	}
	return connect.NewResponse(&financev1.GetExpenseResponse{Expense: toProtoExpense(e), Events: out}), nil
}

// AttachExpenseReceipt stores an expense's receipt.
func (h *FinanceHandler) AttachExpenseReceipt(ctx context.Context, req *connect.Request[financev1.AttachExpenseReceiptRequest]) (*connect.Response[financev1.AttachExpenseReceiptResponse], error) {
	if h.expenses == nil {
		return nil, errExpensesUnwired
	}
	m := req.Msg
	e, err := h.expenses.AttachReceipt(ctx, m.GetId(), m.GetData(), actorOf(m.GetActorId(), m.GetActorName()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.AttachExpenseReceiptResponse{Expense: toProtoExpense(e)}), nil
}

// GetExpenseReceipt returns the receipt's bytes and type.
func (h *FinanceHandler) GetExpenseReceipt(ctx context.Context, req *connect.Request[financev1.GetExpenseReceiptRequest]) (*connect.Response[financev1.GetExpenseReceiptResponse], error) {
	if h.expenses == nil {
		return nil, errExpensesUnwired
	}
	f, err := h.expenses.Receipt(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.GetExpenseReceiptResponse{Data: f.Data(), ContentType: f.ContentType()}), nil
}

// RemoveExpenseReceipt detaches and deletes the receipt.
func (h *FinanceHandler) RemoveExpenseReceipt(ctx context.Context, req *connect.Request[financev1.RemoveExpenseReceiptRequest]) (*connect.Response[financev1.RemoveExpenseReceiptResponse], error) {
	if h.expenses == nil {
		return nil, errExpensesUnwired
	}
	m := req.Msg
	e, err := h.expenses.RemoveReceipt(ctx, m.GetId(), actorOf(m.GetActorId(), m.GetActorName()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.RemoveExpenseReceiptResponse{Expense: toProtoExpense(e)}), nil
}

func toProtoExpenses(es []domain.Expense) []*financev1.Expense {
	out := make([]*financev1.Expense, 0, len(es))
	for _, e := range es {
		out = append(out, toProtoExpense(e))
	}
	return out
}

func toProtoExpense(e domain.Expense) *financev1.Expense {
	out := &financev1.Expense{
		Id: e.ID(), SpentOn: e.SpentOn().Format(dayLayout), CategoryId: e.CategoryID(), CategoryName: e.CategoryName(),
		AmountCents: e.Amount().Int64(), Method: e.Method(), Description: e.Description(), Status: e.Status(),
		CancelReason: e.CancelReason(), CreatedByName: e.CreatedBy().Name,
		CreatedAt: optionalTimestamp(e.CreatedAt()), UpdatedAt: optionalTimestamp(e.UpdatedAt()),
		CancelledAt: optionalTimestamp(e.CancelledAt()),
	}
	if ref, has := e.Receipt(); has {
		out.Receipt = &financev1.ExpenseReceipt{ContentType: ref.ContentType, SizeBytes: int64(ref.SizeBytes)}
	}
	return out
}

func optionalTimestamp(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

// expenseSnapshot is the expense as the history shows it on the wire.
type expenseSnapshot struct {
	SpentOn      string `json:"spentOn"`
	CategoryID   string `json:"categoryId"`
	CategoryName string `json:"categoryName,omitempty"`
	AmountCents  int64  `json:"amountCents"`
	Method       string `json:"method"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	HasReceipt   bool   `json:"hasReceipt"`
}

// snapshotJSON renders one side of a history entry; the zero expense (the
// "before" of a create) is "{}".
func snapshotJSON(e domain.Expense) (string, error) {
	if e.ID() == "" {
		return "{}", nil
	}
	_, has := e.Receipt()
	raw, err := json.Marshal(expenseSnapshot{
		SpentOn: e.SpentOn().Format(dayLayout), CategoryID: e.CategoryID(), CategoryName: e.CategoryName(),
		AmountCents: e.Amount().Int64(), Method: e.Method(), Description: e.Description(), Status: e.Status(), HasReceipt: has,
	})
	if err != nil {
		return "", glerrors.Wrap(glerrors.KindInternal, "rendering an expense's history", err)
	}
	return string(raw), nil
}

func toProtoExpenseEvent(ev domain.ExpenseEvent) (*financev1.ExpenseEvent, error) {
	before, err := snapshotJSON(ev.Before)
	if err != nil {
		return nil, err
	}
	after, err := snapshotJSON(ev.After)
	if err != nil {
		return nil, err
	}
	return &financev1.ExpenseEvent{
		Id: ev.ID, Kind: ev.Kind, BeforeJson: before, AfterJson: after, Reason: ev.Reason,
		ActorName: ev.By.Name, At: optionalTimestamp(ev.At),
	}, nil
}

// --- the period and the cut ------------------------------------------------

// FinanceSummary is a period's income, expenses and balance by method.
func (h *FinanceHandler) FinanceSummary(ctx context.Context, req *connect.Request[financev1.FinanceSummaryRequest]) (*connect.Response[financev1.FinanceSummaryResponse], error) {
	if h.expenses == nil {
		return nil, errExpensesUnwired
	}
	from, err := parseDay(req.Msg.GetFrom())
	if err != nil {
		return nil, connectErr(err)
	}
	to, err := parseDay(req.Msg.GetTo())
	if err != nil {
		return nil, connectErr(err)
	}
	s, err := h.expenses.Summary(ctx, from, to)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.FinanceSummaryResponse{
		Income: toProtoTotals(s.Income), CreditNoteCents: s.CreditNoteCents, Expenses: toProtoTotals(s.Expenses),
		Balance: toProtoTotals(s.Balance), ExpensesByCategory: toProtoCategoryTotals(s.ByCategory),
	}), nil
}

func toProtoTotals(m domain.MethodTotals) *financev1.MethodTotals {
	return &financev1.MethodTotals{
		CashCents: m.CashCents, CardCents: m.CardCents, TransferCents: m.TransferCents, TotalCents: m.TotalCents,
	}
}

func toProtoCategoryTotals(cs []ports.CategoryTotal) []*financev1.CategoryTotal {
	out := make([]*financev1.CategoryTotal, 0, len(cs))
	for _, c := range cs {
		out = append(out, &financev1.CategoryTotal{
			CategoryId: c.CategoryID, CategoryName: c.CategoryName, AmountCents: c.AmountCents, Count: int32(c.Count), //nolint:gosec // bounded by the 500-row cap per period read
		})
	}
	return out
}

// cashCutResponse renders the day's cut with what went out and its closure.
func cashCutResponse(cut usecase.CashCut) *financev1.DailyCashUpResponse {
	res := takingsResponse(cut.CashUp)
	res.Expenses = toProtoExpenses(cut.Expenses)
	res.ExpensesByMethod = toProtoTotals(cut.ExpensesByMethod)
	res.ExpectedCashCents = cut.ExpectedCashCents
	res.ModifiedAfterClose = cut.ModifiedAfterClose
	if cut.Closure != nil {
		res.Closure = toProtoClosure(*cut.Closure)
	}
	for _, c := range cut.Reopened {
		res.Reopened = append(res.Reopened, toProtoClosure(c))
	}
	return res
}

func toProtoClosure(c domain.CashCutClosure) *financev1.CashCutClosure {
	f := c.Figures()
	return &financev1.CashCutClosure{
		Id: c.ID(), Day: c.Day().Format(dayLayout), ExpectedCashCents: c.ExpectedCashCents(),
		CountedCashCents: c.CountedCashCents(), DifferenceCents: c.DifferenceCents(),
		Income: toProtoTotals(f.Income), Expenses: toProtoTotals(f.Expenses), CreditNoteCents: f.CreditNoteCents,
		Note: c.Note(), ClosedByName: c.ClosedBy().Name, ClosedAt: optionalTimestamp(c.ClosedAt()),
		ReopenedAt: optionalTimestamp(c.ReopenedAt()), ReopenedByName: c.ReopenedBy().Name, ReopenReason: c.ReopenReason(),
	}
}

// CloseCashCut closes a day with the cash counted.
func (h *FinanceHandler) CloseCashCut(ctx context.Context, req *connect.Request[financev1.CloseCashCutRequest]) (*connect.Response[financev1.CloseCashCutResponse], error) {
	if h.expenses == nil {
		return nil, errExpensesUnwired
	}
	m := req.Msg
	day, err := parseOptionalDay(m.GetDay())
	if err != nil {
		return nil, connectErr(err)
	}
	c, err := h.expenses.CloseCashCut(ctx, day, m.GetCountedCashCents(), m.GetNote(), actorOf(m.GetActorId(), m.GetActorName()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.CloseCashCutResponse{Closure: toProtoClosure(c)}), nil
}

// ReopenCashCut reopens a closed day.
func (h *FinanceHandler) ReopenCashCut(ctx context.Context, req *connect.Request[financev1.ReopenCashCutRequest]) (*connect.Response[financev1.ReopenCashCutResponse], error) {
	if h.expenses == nil {
		return nil, errExpensesUnwired
	}
	m := req.Msg
	day, err := parseOptionalDay(m.GetDay())
	if err != nil {
		return nil, connectErr(err)
	}
	c, err := h.expenses.ReopenCashCut(ctx, day, m.GetReason(), actorOf(m.GetActorId(), m.GetActorName()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.ReopenCashCutResponse{Closure: toProtoClosure(c)}), nil
}
