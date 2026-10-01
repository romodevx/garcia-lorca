package usecase_test

import (
	"context"
	"sort"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

var (
	_ ports.ExpenseRepository = (*fakeExpenseRepo)(nil)
	_ ports.ReceiptStore      = (*fakeReceiptStore)(nil)
	_ usecase.DayTakings      = (*stubTakings)(nil)
)

// fakeExpenseRepo is a behavioural double: duplicate category names conflict,
// UpdateExpense runs the caller's change over the stored row, and a day "has
// changed" when an event touching it is later than the asked instant.
type fakeExpenseRepo struct {
	categories map[string]domain.ExpenseCategory
	expenses   map[string]domain.Expense
	events     []domain.ExpenseEvent
	closures   []domain.CashCutClosure
	income     domain.MethodTotals
	creditNote int64
	// changedDays forces DayChangedSince to answer true for a day.
	changedDays map[string]bool
}

func newFakeExpenseRepo() *fakeExpenseRepo {
	return &fakeExpenseRepo{
		categories:  map[string]domain.ExpenseCategory{},
		expenses:    map[string]domain.Expense{},
		changedDays: map[string]bool{},
	}
}

func (r *fakeExpenseRepo) CreateExpenseCategory(_ context.Context, c domain.ExpenseCategory) (domain.ExpenseCategory, error) {
	for _, existing := range r.categories {
		if strings.EqualFold(existing.Name(), c.Name()) {
			return domain.ExpenseCategory{}, glerrors.New(glerrors.KindConflict, "taken")
		}
	}
	r.categories[c.ID()] = c
	return c, nil
}

func (r *fakeExpenseRepo) GetExpenseCategory(_ context.Context, id string) (domain.ExpenseCategory, error) {
	c, ok := r.categories[id]
	if !ok {
		return domain.ExpenseCategory{}, glerrors.ErrNotFound
	}
	return c, nil
}

func (r *fakeExpenseRepo) ListExpenseCategories(_ context.Context, includeInactive bool) ([]domain.ExpenseCategory, error) {
	var out []domain.ExpenseCategory
	for _, c := range r.categories {
		if c.Active() || includeInactive {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}

func (r *fakeExpenseRepo) UpdateExpenseCategory(_ context.Context, c domain.ExpenseCategory) (domain.ExpenseCategory, error) {
	if _, ok := r.categories[c.ID()]; !ok {
		return domain.ExpenseCategory{}, glerrors.ErrNotFound
	}
	r.categories[c.ID()] = c
	return c, nil
}

func (r *fakeExpenseRepo) CreateExpense(_ context.Context, e domain.Expense, ev domain.ExpenseEvent) (domain.Expense, error) {
	r.expenses[e.ID()] = e
	r.events = append(r.events, ev)
	return e, nil
}

func (r *fakeExpenseRepo) UpdateExpense(_ context.Context, id string, change ports.ExpenseChange) (domain.Expense, error) {
	cur, ok := r.expenses[id]
	if !ok {
		return domain.Expense{}, glerrors.ErrNotFound
	}
	next, ev, err := change(cur)
	if err != nil {
		return domain.Expense{}, err
	}
	r.expenses[id] = next
	r.events = append(r.events, ev)
	return next, nil
}

func (r *fakeExpenseRepo) GetExpense(_ context.Context, id string) (domain.Expense, error) {
	e, ok := r.expenses[id]
	if !ok {
		return domain.Expense{}, glerrors.ErrNotFound
	}
	return e, nil
}

func (r *fakeExpenseRepo) ListExpenses(_ context.Context, f ports.ExpenseFilter) ([]domain.Expense, error) {
	var out []domain.Expense
	for _, e := range r.expenses {
		if e.SpentOn().Before(f.From) || e.SpentOn().After(f.To) {
			continue
		}
		if e.Cancelled() && !f.IncludeCancelled {
			continue
		}
		if f.CategoryID != "" && e.CategoryID() != f.CategoryID {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out, nil
}

func (r *fakeExpenseRepo) ListExpenseEvents(_ context.Context, id string) ([]domain.ExpenseEvent, error) {
	var out []domain.ExpenseEvent
	for _, ev := range r.events {
		if ev.ExpenseID == id {
			out = append(out, ev)
		}
	}
	return out, nil
}

func (r *fakeExpenseRepo) ExpenseTotals(ctx context.Context, from, to time.Time) (domain.MethodTotals, []ports.CategoryTotal, error) {
	live, _ := r.ListExpenses(ctx, ports.ExpenseFilter{From: from, To: to})
	var totals domain.MethodTotals
	byCat := map[string]*ports.CategoryTotal{}
	for _, e := range live {
		totals.Add(e.Method(), e.Amount().Int64())
		ct, ok := byCat[e.CategoryID()]
		if !ok {
			ct = &ports.CategoryTotal{CategoryID: e.CategoryID(), CategoryName: r.categories[e.CategoryID()].Name()}
			byCat[e.CategoryID()] = ct
		}
		ct.AmountCents += e.Amount().Int64()
		ct.Count++
	}
	out := make([]ports.CategoryTotal, 0, len(byCat))
	for _, ct := range byCat {
		out = append(out, *ct)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AmountCents > out[j].AmountCents })
	return totals, out, nil
}

func (r *fakeExpenseRepo) IncomeByMethod(context.Context, time.Time, time.Time) (domain.MethodTotals, int64, error) {
	return r.income, r.creditNote, nil
}

func (r *fakeExpenseRepo) ListCashCutClosures(_ context.Context, day time.Time) ([]domain.CashCutClosure, error) {
	var out []domain.CashCutClosure
	for i := len(r.closures) - 1; i >= 0; i-- {
		if r.closures[i].Day().Equal(domain.CalendarDay(day)) {
			out = append(out, r.closures[i])
		}
	}
	return out, nil
}

func (r *fakeExpenseRepo) SaveCashCutClosure(_ context.Context, c domain.CashCutClosure) error {
	for _, existing := range r.closures {
		if existing.Day().Equal(c.Day()) && !existing.Reopened() {
			return glerrors.New(glerrors.KindConflict, "closed")
		}
	}
	r.closures = append(r.closures, c)
	return nil
}

func (r *fakeExpenseRepo) ReopenCashCutClosure(_ context.Context, c domain.CashCutClosure) error {
	for i, existing := range r.closures {
		if existing.ID() == c.ID() {
			r.closures[i] = c
			return nil
		}
	}
	return glerrors.ErrNotFound
}

func (r *fakeExpenseRepo) DayChangedSince(_ context.Context, day, since time.Time) (bool, error) {
	if r.changedDays[day.Format("2006-01-02")] {
		return true, nil
	}
	for _, ev := range r.events {
		touches := ev.After.SpentOn().Equal(day) || ev.Before.SpentOn().Equal(day)
		if touches && ev.At.After(since) {
			return true, nil
		}
	}
	return false, nil
}

// fakeReceiptStore keeps objects in memory and remembers what was deleted.
type fakeReceiptStore struct {
	objects map[string][]byte
	deleted []string
	putErr  error
}

func newFakeReceiptStore() *fakeReceiptStore {
	return &fakeReceiptStore{objects: map[string][]byte{}}
}

func (s *fakeReceiptStore) Put(_ context.Context, key, _ string, data []byte) error {
	if s.putErr != nil {
		return s.putErr
	}
	s.objects[key] = data
	return nil
}

func (s *fakeReceiptStore) Get(_ context.Context, key string) ([]byte, error) {
	data, ok := s.objects[key]
	if !ok {
		return nil, glerrors.ErrNotFound
	}
	return data, nil
}

func (s *fakeReceiptStore) Delete(_ context.Context, key string) error {
	delete(s.objects, key)
	s.deleted = append(s.deleted, key)
	return nil
}

// stubTakings answers the day's takings with a fixed cut.
type stubTakings struct {
	cut     usecase.CashUp
	askedOn time.Time
}

func (s *stubTakings) DailyCashUp(_ context.Context, day time.Time) (usecase.CashUp, error) {
	s.askedOn = day
	cut := s.cut
	cut.Day = day
	return cut, nil
}
