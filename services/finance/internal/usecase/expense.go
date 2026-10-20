package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// maxExpenseRangeDays bounds a list or a summary: a year and a day, enough
// for the yearly report of a leap year, and a ceiling on one request's cost.
const maxExpenseRangeDays = 366

// DayTakings is the day's income as the cash-up already adds it up
// (FinanceService.DailyCashUp). The cut builds on it rather than counting
// deposits a second way.
type DayTakings interface {
	DailyCashUp(ctx context.Context, day time.Time) (CashUp, error)
}

// ExpenseService records what the school pays out and closes the day's cash
// cut (T10, ADR 0027). It depends on ports only: the expense repository, the
// day's takings, and the private store that keeps receipts.
type ExpenseService struct {
	repo     ports.ExpenseRepository
	takings  DayTakings
	receipts ports.ReceiptStore
	newID    func() string
	clock    SchoolClock
}

// NewExpenseService wires the use case. receipts may be nil: a deployment with
// no bucket records expenses without files, and receipt calls say so.
func NewExpenseService(repo ports.ExpenseRepository, takings DayTakings, receipts ports.ReceiptStore,
	newID func() string, clock SchoolClock,
) *ExpenseService {
	return &ExpenseService{repo: repo, takings: takings, receipts: receipts, newID: newID, clock: clock}
}

// --- categories ------------------------------------------------------------

// CreateCategory adds a category, active. A name already used is KindConflict.
func (s *ExpenseService) CreateCategory(ctx context.Context, name string) (domain.ExpenseCategory, error) {
	c, err := domain.NewExpenseCategory(s.newID(), name, true)
	if err != nil {
		return domain.ExpenseCategory{}, err
	}
	return s.repo.CreateExpenseCategory(ctx, c)
}

// ListCategories lists the categories; inactive ones only when asked.
func (s *ExpenseService) ListCategories(ctx context.Context, includeInactive bool) ([]domain.ExpenseCategory, error) {
	return s.repo.ListExpenseCategories(ctx, includeInactive)
}

// UpdateCategory renames and (de)activates a category. Recorded expenses keep
// pointing at it either way.
func (s *ExpenseService) UpdateCategory(ctx context.Context, id, name string, active bool) (domain.ExpenseCategory, error) {
	current, err := s.repo.GetExpenseCategory(ctx, id)
	if err != nil {
		return domain.ExpenseCategory{}, err
	}
	next, err := current.WithUpdate(name, active)
	if err != nil {
		return domain.ExpenseCategory{}, err
	}
	return s.repo.UpdateExpenseCategory(ctx, next)
}

// --- expenses --------------------------------------------------------------

// CreateExpense records money paid out, filed under an active category, with
// its "created" event.
func (s *ExpenseService) CreateExpense(ctx context.Context, d domain.ExpenseDraft, by domain.Receiver) (domain.Expense, error) {
	if err := s.requireActiveCategory(ctx, d.CategoryID); err != nil {
		return domain.Expense{}, err
	}
	at := s.clock.Now()
	e, err := domain.NewExpense(s.newID(), d, s.clock.Today(), by)
	if err != nil {
		return domain.Expense{}, err
	}
	created := domain.ExpenseEvent{
		ID: s.newID(), ExpenseID: e.ID(), Kind: domain.ExpenseEventCreated, After: e, By: by, At: at,
	}
	return s.repo.CreateExpense(ctx, e, created)
}

// maxExpenseExternalRefLen mirrors the column's CHECK (finance/0031).
const maxExpenseExternalRefLen = 120

// CreateExpenseOnce records an expense that another module asks for on its
// own behalf, keyed by that module's reference (C2-015: a uniforms refund):
// repeating it returns the expense already recorded, so a retry cannot pay
// out twice. An empty reference is an ordinary expense.
func (s *ExpenseService) CreateExpenseOnce(ctx context.Context, d domain.ExpenseDraft, by domain.Receiver, externalRef string) (domain.Expense, error) {
	ref := strings.TrimSpace(externalRef)
	if ref == "" {
		return s.CreateExpense(ctx, d, by)
	}
	if len(ref) > maxExpenseExternalRefLen {
		return domain.Expense{}, glerrors.New(glerrors.KindInvalid, "the expense reference is too long")
	}
	if err := s.requireActiveCategory(ctx, d.CategoryID); err != nil {
		return domain.Expense{}, err
	}
	e, err := domain.NewExpense(s.newID(), d, s.clock.Today(), by)
	if err != nil {
		return domain.Expense{}, err
	}
	created := domain.ExpenseEvent{
		ID: s.newID(), ExpenseID: e.ID(), Kind: domain.ExpenseEventCreated, After: e, By: by, At: s.clock.Now(),
	}
	return s.repo.CreateExpenseOnce(ctx, e, created, ref)
}

// requireActiveCategory refuses a category that does not exist (as bad input,
// not as a NotFound about another resource) or is retired.
func (s *ExpenseService) requireActiveCategory(ctx context.Context, id string) error {
	c, err := s.repo.GetExpenseCategory(ctx, id)
	if glerrors.KindOf(err) == glerrors.KindNotFound {
		return glerrors.New(glerrors.KindInvalid, "la categoría no existe")
	}
	if err != nil {
		return err
	}
	if !c.Active() {
		return glerrors.New(glerrors.KindInvalid, "la categoría está desactivada")
	}
	return nil
}

// UpdateExpense edits an expense; the history keeps it before and after, and
// the optional reason. Moving it to a retired category is refused; keeping
// the one it has is not.
func (s *ExpenseService) UpdateExpense(ctx context.Context, id string, d domain.ExpenseDraft, reason string, by domain.Receiver) (domain.Expense, error) {
	trimmed, err := domain.ValidateExpenseReason(reason)
	if err != nil {
		return domain.Expense{}, err
	}
	current, err := s.repo.GetExpense(ctx, id)
	if err != nil {
		return domain.Expense{}, err
	}
	if current.CategoryID() != d.CategoryID {
		if err := s.requireActiveCategory(ctx, d.CategoryID); err != nil {
			return domain.Expense{}, err
		}
	}
	at, today := s.clock.Now(), s.clock.Today()
	return s.repo.UpdateExpense(ctx, id, func(cur domain.Expense) (domain.Expense, domain.ExpenseEvent, error) {
		// The category check above was made against the expense as read
		// before its lock. If somebody re-filed it in between, that check no
		// longer holds: refuse rather than apply it to a different expense.
		if cur.CategoryID() != current.CategoryID() {
			return domain.Expense{}, domain.ExpenseEvent{}, errExpenseChanged
		}
		next, err := cur.Edit(d, today)
		if err != nil {
			return domain.Expense{}, domain.ExpenseEvent{}, err
		}
		return next, s.event(domain.ExpenseEventEdited, cur, next, trimmed, by, at), nil
	})
}

// CancelExpense cancels an expense with its reason (required). It stays,
// counting for nothing.
func (s *ExpenseService) CancelExpense(ctx context.Context, id, reason string, by domain.Receiver) (domain.Expense, error) {
	at := s.clock.Now()
	return s.repo.UpdateExpense(ctx, id, func(cur domain.Expense) (domain.Expense, domain.ExpenseEvent, error) {
		next, err := cur.Cancel(reason, at)
		if err != nil {
			return domain.Expense{}, domain.ExpenseEvent{}, err
		}
		return next, s.event(domain.ExpenseEventCancelled, cur, next, next.CancelReason(), by, at), nil
	})
}

func (s *ExpenseService) event(kind string, before, after domain.Expense, reason string, by domain.Receiver, at time.Time) domain.ExpenseEvent {
	return domain.ExpenseEvent{
		ID: s.newID(), ExpenseID: after.ID(), Kind: kind, Before: before, After: after, Reason: reason, By: by, At: at,
	}
}

// ExpenseQuery is a list request: a date range (both inclusive, at most a
// year), an optional category, and whether cancelled expenses come too.
type ExpenseQuery struct {
	From, To         time.Time
	CategoryID       string
	IncludeCancelled bool
}

// ListExpenses lists the expenses of a range, newest first (at most 500).
func (s *ExpenseService) ListExpenses(ctx context.Context, q ExpenseQuery) ([]domain.Expense, error) {
	from, to, err := expenseRange(q.From, q.To)
	if err != nil {
		return nil, err
	}
	return s.repo.ListExpenses(ctx, ports.ExpenseFilter{
		From: from, To: to, CategoryID: q.CategoryID, IncludeCancelled: q.IncludeCancelled,
	})
}

// expenseRange validates a period: both days present, in order, at most a year.
func expenseRange(from, to time.Time) (time.Time, time.Time, error) {
	if from.IsZero() || to.IsZero() {
		return time.Time{}, time.Time{}, glerrors.New(glerrors.KindInvalid, "from and to are required")
	}
	from, to = domain.CalendarDay(from), domain.CalendarDay(to)
	if to.Before(from) {
		return time.Time{}, time.Time{}, glerrors.New(glerrors.KindInvalid, "from must not be after to")
	}
	if to.Sub(from) > maxExpenseRangeDays*24*time.Hour {
		return time.Time{}, time.Time{}, glerrors.New(glerrors.KindInvalid, "the period is at most a year")
	}
	return from, to, nil
}

// GetExpense returns an expense and its history, oldest event first.
func (s *ExpenseService) GetExpense(ctx context.Context, id string) (domain.Expense, []domain.ExpenseEvent, error) {
	e, err := s.repo.GetExpense(ctx, id)
	if err != nil {
		return domain.Expense{}, nil, err
	}
	events, err := s.repo.ListExpenseEvents(ctx, id)
	if err != nil {
		return domain.Expense{}, nil, err
	}
	return e, events, nil
}

// --- receipts --------------------------------------------------------------

var errReceiptsUnavailable = glerrors.New(glerrors.KindNotFound, "receipts are not enabled")

// errExpenseChanged: the expense was re-filed by somebody else while this
// edit was being made.
var errExpenseChanged = glerrors.New(glerrors.KindConflict, "el egreso cambió mientras lo editabas; ábrelo de nuevo")

// AttachReceipt stores a receipt (JPEG, PNG or PDF by its bytes, at most 5 MB)
// and points the expense at it, replacing any previous one. The file goes up
// first; if the expense then cannot take it, the upload is deleted, so no
// object is left that nothing references.
func (s *ExpenseService) AttachReceipt(ctx context.Context, id string, data []byte, by domain.Receiver) (domain.Expense, error) {
	if s.receipts == nil {
		return domain.Expense{}, errReceiptsUnavailable
	}
	file, err := domain.NewReceiptFile(data)
	if err != nil {
		return domain.Expense{}, err
	}
	// The expense first: nothing is uploaded for an expense that does not
	// exist or can no longer take a receipt.
	current, err := s.repo.GetExpense(ctx, id)
	if err != nil {
		return domain.Expense{}, err
	}
	if current.Cancelled() {
		return domain.Expense{}, domain.ErrExpenseCancelled
	}
	ref := domain.ExpenseReceiptRef{
		Key:         "expenses/" + id + "/" + s.newID(),
		ContentType: file.ContentType(),
		SizeBytes:   len(file.Data()),
	}
	if err := s.receipts.Put(ctx, ref.Key, ref.ContentType, file.Data()); err != nil {
		return domain.Expense{}, err
	}
	var previous domain.ExpenseReceiptRef
	at := s.clock.Now()
	saved, err := s.repo.UpdateExpense(ctx, id, func(cur domain.Expense) (domain.Expense, domain.ExpenseEvent, error) {
		next, err := cur.WithReceipt(ref)
		if err != nil {
			return domain.Expense{}, domain.ExpenseEvent{}, err
		}
		previous, _ = cur.Receipt()
		return next, s.event(domain.ExpenseEventReceiptAttached, cur, next, "", by, at), nil
	})
	if err != nil {
		return domain.Expense{}, errors.Join(err, s.receipts.Delete(ctx, ref.Key))
	}
	if previous.Key != "" {
		// Best-effort: the new file is attached; a failure here leaves an
		// unreferenced object, which the store logs with its key.
		_ = s.receipts.Delete(ctx, previous.Key)
	}
	return saved, nil
}

// Receipt returns an expense's receipt, or KindNotFound when it has none.
func (s *ExpenseService) Receipt(ctx context.Context, id string) (domain.ReceiptFile, error) {
	if s.receipts == nil {
		return domain.ReceiptFile{}, errReceiptsUnavailable
	}
	e, err := s.repo.GetExpense(ctx, id)
	if err != nil {
		return domain.ReceiptFile{}, err
	}
	ref, has := e.Receipt()
	if !has {
		return domain.ReceiptFile{}, glerrors.New(glerrors.KindNotFound, "the expense has no receipt")
	}
	data, err := s.receipts.Get(ctx, ref.Key)
	if err != nil {
		return domain.ReceiptFile{}, err
	}
	return domain.NewReceiptFile(data)
}

// RemoveReceipt detaches the receipt (recorded in the history) and deletes
// the file: the reference first, so the expense never points at nothing.
func (s *ExpenseService) RemoveReceipt(ctx context.Context, id string, by domain.Receiver) (domain.Expense, error) {
	if s.receipts == nil {
		return domain.Expense{}, errReceiptsUnavailable
	}
	var removed domain.ExpenseReceiptRef
	at := s.clock.Now()
	saved, err := s.repo.UpdateExpense(ctx, id, func(cur domain.Expense) (domain.Expense, domain.ExpenseEvent, error) {
		ref, has := cur.Receipt()
		if !has {
			return domain.Expense{}, domain.ExpenseEvent{}, glerrors.New(glerrors.KindNotFound, "the expense has no receipt")
		}
		removed = ref
		next := cur.WithoutReceipt()
		return next, s.event(domain.ExpenseEventReceiptRemoved, cur, next, "", by, at), nil
	})
	if err != nil {
		return domain.Expense{}, err
	}
	// Best-effort, like the replaced file in AttachReceipt: the detach is
	// committed and is the answer; a failed delete leaves an unreferenced
	// object the store logs with its key.
	_ = s.receipts.Delete(ctx, removed.Key)
	return saved, nil
}

// --- the period's money ----------------------------------------------------

// Summary is a period's money: income and expenses by method, the balance
// between them, what credit notes settled (apart: not income), and the
// expenses by category.
type Summary struct {
	From, To        time.Time
	Income          domain.MethodTotals
	CreditNoteCents int64
	Expenses        domain.MethodTotals
	Balance         domain.MethodTotals
	ByCategory      []ports.CategoryTotal
}

// Summary adds up a period (both days inclusive, at most a year).
func (s *ExpenseService) Summary(ctx context.Context, from, to time.Time) (Summary, error) {
	from, to, err := expenseRange(from, to)
	if err != nil {
		return Summary{}, err
	}
	income, creditNote, err := s.repo.IncomeByMethod(ctx, from, to)
	if err != nil {
		return Summary{}, err
	}
	expenses, byCategory, err := s.repo.ExpenseTotals(ctx, from, to)
	if err != nil {
		return Summary{}, err
	}
	return Summary{
		From: from, To: to, Income: income, CreditNoteCents: creditNote,
		Expenses: expenses, Balance: income.Minus(expenses), ByCategory: byCategory,
	}, nil
}

// --- the cash cut ----------------------------------------------------------

// CashCut is the day's cut with what went out: the takings, the day's
// expenses (cancelled ones listed, not counted), the cash the drawer should
// hold, and the closure if the day is closed.
type CashCut struct {
	CashUp
	Expenses          []domain.Expense
	ExpensesByMethod  domain.MethodTotals
	ExpectedCashCents int64
	// Closure is the day's live closure; nil while the day is open.
	Closure *domain.CashCutClosure
	// ModifiedAfterClose: a movement of the day changed after Closure.
	ModifiedAfterClose bool
	// Reopened are the day's earlier closures, newest first.
	Reopened []domain.CashCutClosure
}

// Figures are the cut's money as a closure snapshots it.
func (c CashCut) Figures() domain.CutFigures {
	var income domain.MethodTotals
	for _, method := range []string{domain.MethodCash, domain.MethodCard, domain.MethodTransfer} {
		income.Add(method, c.ByMethod[method])
	}
	return domain.CutFigures{Income: income, Expenses: c.ExpensesByMethod, CreditNoteCents: c.CreditNoteCents}
}

// CashCut reads a day's cut. A zero day is today.
func (s *ExpenseService) CashCut(ctx context.Context, day time.Time) (CashCut, error) {
	if day.IsZero() {
		day = s.clock.Today()
	}
	day = domain.CalendarDay(day)
	takings, err := s.takings.DailyCashUp(ctx, day)
	if err != nil {
		return CashCut{}, err
	}
	expenses, err := s.repo.ListExpenses(ctx, ports.ExpenseFilter{From: day, To: day, IncludeCancelled: true})
	if err != nil {
		return CashCut{}, err
	}
	cut := CashCut{CashUp: takings, Expenses: expenses}
	for _, e := range expenses {
		if !e.Cancelled() {
			cut.ExpensesByMethod.Add(e.Method(), e.Amount().Int64())
		}
	}
	cut.ExpectedCashCents = cut.Figures().ExpectedCashCents()
	if err := s.attachClosures(ctx, &cut, day); err != nil {
		return CashCut{}, err
	}
	return cut, nil
}

// attachClosures puts the day's live closure and history on the cut, and
// whether anything of the day changed after it was closed.
func (s *ExpenseService) attachClosures(ctx context.Context, cut *CashCut, day time.Time) error {
	closures, err := s.repo.ListCashCutClosures(ctx, day)
	if err != nil {
		return err
	}
	for i := range closures {
		if closures[i].Reopened() {
			cut.Reopened = append(cut.Reopened, closures[i])
			continue
		}
		live := closures[i]
		cut.Closure = &live
	}
	if cut.Closure == nil {
		return nil
	}
	changed, err := s.repo.DayChangedSince(ctx, day, cut.Closure.ClosedAt())
	if err != nil {
		return err
	}
	cut.ModifiedAfterClose = changed
	return nil
}

// CloseCashCut closes a day with the cash counted in the drawer, storing the
// cut's figures as they stand. A day already closed is KindConflict.
func (s *ExpenseService) CloseCashCut(ctx context.Context, day time.Time, countedCashCents int64, note string, by domain.Receiver) (domain.CashCutClosure, error) {
	now := s.clock.Now()
	if day.IsZero() {
		day = s.clock.Today()
	}
	if domain.CalendarDay(day).After(s.clock.Today()) {
		return domain.CashCutClosure{}, glerrors.New(glerrors.KindInvalid, "no se puede cerrar un día futuro")
	}
	cut, err := s.CashCut(ctx, day)
	if err != nil {
		return domain.CashCutClosure{}, err
	}
	if cut.Closure != nil {
		return domain.CashCutClosure{}, glerrors.New(glerrors.KindConflict, "el corte de ese día ya está cerrado")
	}
	closure, err := domain.CloseCashCut(s.newID(), day, cut.Figures(), countedCashCents, note, by, now, now)
	if err != nil {
		return domain.CashCutClosure{}, err
	}
	if err := s.repo.SaveCashCutClosure(ctx, closure); err != nil {
		return domain.CashCutClosure{}, err
	}
	return closure, nil
}

// ReopenCashCut reopens a closed day with its reason. KindNotFound when the
// day is not closed.
func (s *ExpenseService) ReopenCashCut(ctx context.Context, day time.Time, reason string, by domain.Receiver) (domain.CashCutClosure, error) {
	if day.IsZero() {
		day = s.clock.Today()
	}
	closures, err := s.repo.ListCashCutClosures(ctx, domain.CalendarDay(day))
	if err != nil {
		return domain.CashCutClosure{}, err
	}
	for _, c := range closures {
		if c.Reopened() {
			continue
		}
		reopened, err := c.Reopen(reason, by, s.clock.Now())
		if err != nil {
			return domain.CashCutClosure{}, err
		}
		if err := s.repo.ReopenCashCutClosure(ctx, reopened); err != nil {
			return domain.CashCutClosure{}, err
		}
		return reopened, nil
	}
	return domain.CashCutClosure{}, glerrors.New(glerrors.KindNotFound, "el corte de ese día no está cerrado")
}
