package client

import (
	"context"
	"encoding/json"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

// T10: expenses (egresos), the period's money and the cash cut's closure.
// Money is integer cents on the wire, always.

// MethodTotals is an amount per method and their sum. A balance may be
// negative.
type MethodTotals struct {
	CashCents     int64 `json:"cashCents"`
	CardCents     int64 `json:"cardCents"`
	TransferCents int64 `json:"transferCents"`
	TotalCents    int64 `json:"totalCents"`
}

// ExpenseCategory is a category expenses are filed under.
type ExpenseCategory struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// ExpenseReceipt describes an attached receipt (not its bytes).
type ExpenseReceipt struct {
	ContentType string `json:"contentType"`
	SizeBytes   int64  `json:"sizeBytes"`
}

// Expense is money the school paid out.
type Expense struct {
	ID            string          `json:"id"`
	SpentOn       string          `json:"spentOn"`
	CategoryID    string          `json:"categoryId"`
	CategoryName  string          `json:"categoryName"`
	AmountCents   int64           `json:"amountCents"`
	Method        string          `json:"method"`
	Description   string          `json:"description"`
	Status        string          `json:"status"`
	CancelReason  string          `json:"cancelReason,omitempty"`
	CancelledAt   string          `json:"cancelledAt,omitempty"`
	Receipt       *ExpenseReceipt `json:"receipt,omitempty"`
	CreatedByName string          `json:"createdByName,omitempty"`
	CreatedAt     string          `json:"createdAt,omitempty"`
	UpdatedAt     string          `json:"updatedAt,omitempty"`
}

// ExpenseEvent is one entry of an expense's history.
type ExpenseEvent struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Before    json.RawMessage `json:"before"`
	After     json.RawMessage `json:"after"`
	Reason    string          `json:"reason,omitempty"`
	ActorName string          `json:"actorName,omitempty"`
	At        string          `json:"at"`
}

// ExpenseInput is an expense as the office types it. By is the signed-in
// caller — never a field of a request body.
type ExpenseInput struct {
	SpentOn     string
	CategoryID  string
	AmountCents int64
	Method      string
	Description string
	// Reason applies to an edit only.
	Reason string
	By     Receiver
}

// ExpenseQuery is a list request.
type ExpenseQuery struct {
	From, To         string
	CategoryID       string
	IncludeCancelled bool
}

// CategoryTotal is what one category took in a period.
type CategoryTotal struct {
	CategoryID   string `json:"categoryId"`
	CategoryName string `json:"categoryName"`
	AmountCents  int64  `json:"amountCents"`
	Count        int32  `json:"count"`
}

// FinanceSummary is a period's money.
type FinanceSummary struct {
	Income          MethodTotals    `json:"income"`
	CreditNoteCents int64           `json:"creditNoteCents"`
	Expenses        MethodTotals    `json:"expenses"`
	Balance         MethodTotals    `json:"balance"`
	ByCategory      []CategoryTotal `json:"expensesByCategory"`
}

// CashCutClosure is one closure of a day's cut.
type CashCutClosure struct {
	ID                string       `json:"id"`
	Day               string       `json:"day"`
	ExpectedCashCents int64        `json:"expectedCashCents"`
	CountedCashCents  int64        `json:"countedCashCents"`
	DifferenceCents   int64        `json:"differenceCents"`
	Income            MethodTotals `json:"income"`
	Expenses          MethodTotals `json:"expenses"`
	CreditNoteCents   int64        `json:"creditNoteCents"`
	Note              string       `json:"note,omitempty"`
	ClosedByName      string       `json:"closedByName,omitempty"`
	ClosedAt          string       `json:"closedAt"`
	ReopenedAt        string       `json:"reopenedAt,omitempty"`
	ReopenedByName    string       `json:"reopenedByName,omitempty"`
	ReopenReason      string       `json:"reopenReason,omitempty"`
}

// --- calls -----------------------------------------------------------------

// CreateExpenseCategory adds a category.
func (c *FinanceClient) CreateExpenseCategory(ctx context.Context, name string) (ExpenseCategory, error) {
	res, err := c.rpc.CreateExpenseCategory(ctx, connect.NewRequest(&financev1.CreateExpenseCategoryRequest{Name: name}))
	if err != nil {
		return ExpenseCategory{}, err
	}
	return toExpenseCategory(res.Msg.GetCategory()), nil
}

// ListExpenseCategories lists the categories.
func (c *FinanceClient) ListExpenseCategories(ctx context.Context, includeInactive bool) ([]ExpenseCategory, error) {
	res, err := c.rpc.ListExpenseCategories(ctx, connect.NewRequest(&financev1.ListExpenseCategoriesRequest{IncludeInactive: includeInactive}))
	if err != nil {
		return nil, err
	}
	out := make([]ExpenseCategory, 0, len(res.Msg.GetCategories()))
	for _, cat := range res.Msg.GetCategories() {
		out = append(out, toExpenseCategory(cat))
	}
	return out, nil
}

// UpdateExpenseCategory renames / (de)activates a category.
func (c *FinanceClient) UpdateExpenseCategory(ctx context.Context, id, name string, active bool) (ExpenseCategory, error) {
	res, err := c.rpc.UpdateExpenseCategory(ctx, connect.NewRequest(&financev1.UpdateExpenseCategoryRequest{Id: id, Name: name, Active: active}))
	if err != nil {
		return ExpenseCategory{}, err
	}
	return toExpenseCategory(res.Msg.GetCategory()), nil
}

// CreateExpense records an expense.
func (c *FinanceClient) CreateExpense(ctx context.Context, in ExpenseInput) (Expense, error) {
	res, err := c.rpc.CreateExpense(ctx, connect.NewRequest(toCreateExpenseRequest(in)))
	if err != nil {
		return Expense{}, err
	}
	return toExpense(res.Msg.GetExpense()), nil
}

// UpdateExpense edits an expense.
func (c *FinanceClient) UpdateExpense(ctx context.Context, id string, in ExpenseInput) (Expense, error) {
	res, err := c.rpc.UpdateExpense(ctx, connect.NewRequest(toUpdateExpenseRequest(id, in)))
	if err != nil {
		return Expense{}, err
	}
	return toExpense(res.Msg.GetExpense()), nil
}

// CancelExpense cancels an expense with its reason.
func (c *FinanceClient) CancelExpense(ctx context.Context, id, reason string, by Receiver) (Expense, error) {
	res, err := c.rpc.CancelExpense(ctx, connect.NewRequest(&financev1.CancelExpenseRequest{
		Id: id, Reason: reason, ActorId: by.UserID, ActorName: by.Name,
	}))
	if err != nil {
		return Expense{}, err
	}
	return toExpense(res.Msg.GetExpense()), nil
}

// ListExpenses lists a range's expenses.
func (c *FinanceClient) ListExpenses(ctx context.Context, q ExpenseQuery) ([]Expense, error) {
	res, err := c.rpc.ListExpenses(ctx, connect.NewRequest(&financev1.ListExpensesRequest{
		From: q.From, To: q.To, CategoryId: q.CategoryID, IncludeCancelled: q.IncludeCancelled,
	}))
	if err != nil {
		return nil, err
	}
	return toExpenses(res.Msg.GetExpenses()), nil
}

// GetExpense returns an expense and its history.
func (c *FinanceClient) GetExpense(ctx context.Context, id string) (Expense, []ExpenseEvent, error) {
	res, err := c.rpc.GetExpense(ctx, connect.NewRequest(&financev1.GetExpenseRequest{Id: id}))
	if err != nil {
		return Expense{}, nil, err
	}
	events := make([]ExpenseEvent, 0, len(res.Msg.GetEvents()))
	for _, ev := range res.Msg.GetEvents() {
		events = append(events, toExpenseEvent(ev))
	}
	return toExpense(res.Msg.GetExpense()), events, nil
}

// AttachExpenseReceipt stores an expense's receipt.
func (c *FinanceClient) AttachExpenseReceipt(ctx context.Context, id string, data []byte, by Receiver) (Expense, error) {
	res, err := c.rpc.AttachExpenseReceipt(ctx, connect.NewRequest(&financev1.AttachExpenseReceiptRequest{
		Id: id, Data: data, ActorId: by.UserID, ActorName: by.Name,
	}))
	if err != nil {
		return Expense{}, err
	}
	return toExpense(res.Msg.GetExpense()), nil
}

// GetExpenseReceipt returns the receipt's bytes and type.
func (c *FinanceClient) GetExpenseReceipt(ctx context.Context, id string) ([]byte, string, error) {
	res, err := c.rpc.GetExpenseReceipt(ctx, connect.NewRequest(&financev1.GetExpenseReceiptRequest{Id: id}))
	if err != nil {
		return nil, "", err
	}
	return res.Msg.GetData(), res.Msg.GetContentType(), nil
}

// RemoveExpenseReceipt detaches and deletes the receipt.
func (c *FinanceClient) RemoveExpenseReceipt(ctx context.Context, id string, by Receiver) (Expense, error) {
	res, err := c.rpc.RemoveExpenseReceipt(ctx, connect.NewRequest(&financev1.RemoveExpenseReceiptRequest{
		Id: id, ActorId: by.UserID, ActorName: by.Name,
	}))
	if err != nil {
		return Expense{}, err
	}
	return toExpense(res.Msg.GetExpense()), nil
}

// FinanceSummary is a period's income, expenses and balance by method.
func (c *FinanceClient) FinanceSummary(ctx context.Context, from, to string) (FinanceSummary, error) {
	res, err := c.rpc.FinanceSummary(ctx, connect.NewRequest(&financev1.FinanceSummaryRequest{From: from, To: to}))
	if err != nil {
		return FinanceSummary{}, err
	}
	return toFinanceSummary(res.Msg), nil
}

// CloseCashCut closes a day with the cash counted.
func (c *FinanceClient) CloseCashCut(ctx context.Context, day string, countedCashCents int64, note string, by Receiver) (CashCutClosure, error) {
	res, err := c.rpc.CloseCashCut(ctx, connect.NewRequest(&financev1.CloseCashCutRequest{
		Day: day, CountedCashCents: countedCashCents, Note: note, ActorId: by.UserID, ActorName: by.Name,
	}))
	if err != nil {
		return CashCutClosure{}, err
	}
	return toCashCutClosure(res.Msg.GetClosure()), nil
}

// ReopenCashCut reopens a closed day.
func (c *FinanceClient) ReopenCashCut(ctx context.Context, day, reason string, by Receiver) (CashCutClosure, error) {
	res, err := c.rpc.ReopenCashCut(ctx, connect.NewRequest(&financev1.ReopenCashCutRequest{
		Day: day, Reason: reason, ActorId: by.UserID, ActorName: by.Name,
	}))
	if err != nil {
		return CashCutClosure{}, err
	}
	return toCashCutClosure(res.Msg.GetClosure()), nil
}

// --- mapping ---------------------------------------------------------------

func toCreateExpenseRequest(in ExpenseInput) *financev1.CreateExpenseRequest {
	return &financev1.CreateExpenseRequest{
		SpentOn: in.SpentOn, CategoryId: in.CategoryID, AmountCents: in.AmountCents, Method: in.Method,
		Description: in.Description, ActorId: in.By.UserID, ActorName: in.By.Name,
	}
}

func toUpdateExpenseRequest(id string, in ExpenseInput) *financev1.UpdateExpenseRequest {
	return &financev1.UpdateExpenseRequest{
		Id: id, SpentOn: in.SpentOn, CategoryId: in.CategoryID, AmountCents: in.AmountCents, Method: in.Method,
		Description: in.Description, Reason: in.Reason, ActorId: in.By.UserID, ActorName: in.By.Name,
	}
}

func toExpenseCategory(c *financev1.ExpenseCategory) ExpenseCategory {
	return ExpenseCategory{ID: c.GetId(), Name: c.GetName(), Active: c.GetActive()}
}

func rfc3339(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return ""
	}
	return ts.AsTime().Format(time.RFC3339)
}

func toExpenses(es []*financev1.Expense) []Expense {
	out := make([]Expense, 0, len(es))
	for _, e := range es {
		out = append(out, toExpense(e))
	}
	return out
}

func toExpense(e *financev1.Expense) Expense {
	out := Expense{
		ID: e.GetId(), SpentOn: e.GetSpentOn(), CategoryID: e.GetCategoryId(), CategoryName: e.GetCategoryName(),
		AmountCents: e.GetAmountCents(), Method: e.GetMethod(), Description: e.GetDescription(), Status: e.GetStatus(),
		CancelReason: e.GetCancelReason(), CancelledAt: rfc3339(e.GetCancelledAt()), CreatedByName: e.GetCreatedByName(),
		CreatedAt: rfc3339(e.GetCreatedAt()), UpdatedAt: rfc3339(e.GetUpdatedAt()),
	}
	if r := e.GetReceipt(); r != nil {
		out.Receipt = &ExpenseReceipt{ContentType: r.GetContentType(), SizeBytes: r.GetSizeBytes()}
	}
	return out
}

// rawJSON passes a history snapshot through; anything that is not JSON
// becomes {} rather than breaking the response.
func rawJSON(s string) json.RawMessage {
	if !json.Valid([]byte(s)) {
		return json.RawMessage("{}")
	}
	return json.RawMessage(s)
}

func toExpenseEvent(ev *financev1.ExpenseEvent) ExpenseEvent {
	return ExpenseEvent{
		ID: ev.GetId(), Kind: ev.GetKind(), Before: rawJSON(ev.GetBeforeJson()), After: rawJSON(ev.GetAfterJson()),
		Reason: ev.GetReason(), ActorName: ev.GetActorName(), At: rfc3339(ev.GetAt()),
	}
}

func toMethodTotals(m *financev1.MethodTotals) MethodTotals {
	return MethodTotals{
		CashCents: m.GetCashCents(), CardCents: m.GetCardCents(), TransferCents: m.GetTransferCents(), TotalCents: m.GetTotalCents(),
	}
}

func toFinanceSummary(m *financev1.FinanceSummaryResponse) FinanceSummary {
	cats := make([]CategoryTotal, 0, len(m.GetExpensesByCategory()))
	for _, c := range m.GetExpensesByCategory() {
		cats = append(cats, CategoryTotal{CategoryID: c.GetCategoryId(), CategoryName: c.GetCategoryName(), AmountCents: c.GetAmountCents(), Count: c.GetCount()})
	}
	return FinanceSummary{
		Income: toMethodTotals(m.GetIncome()), CreditNoteCents: m.GetCreditNoteCents(),
		Expenses: toMethodTotals(m.GetExpenses()), Balance: toMethodTotals(m.GetBalance()), ByCategory: cats,
	}
}

func toCashCutClosure(c *financev1.CashCutClosure) CashCutClosure {
	return CashCutClosure{
		ID: c.GetId(), Day: c.GetDay(), ExpectedCashCents: c.GetExpectedCashCents(), CountedCashCents: c.GetCountedCashCents(),
		DifferenceCents: c.GetDifferenceCents(), Income: toMethodTotals(c.GetIncome()), Expenses: toMethodTotals(c.GetExpenses()),
		CreditNoteCents: c.GetCreditNoteCents(), Note: c.GetNote(), ClosedByName: c.GetClosedByName(), ClosedAt: rfc3339(c.GetClosedAt()),
		ReopenedAt: rfc3339(c.GetReopenedAt()), ReopenedByName: c.GetReopenedByName(), ReopenReason: c.GetReopenReason(),
	}
}
