package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

var expNow = time.Date(2026, 10, 1, 15, 0, 0, 0, time.UTC)

var expToday = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

var cashier = domain.Receiver{UserID: "u-cash", Name: "Caja"}

type expenseFixture struct {
	repo     *fakeExpenseRepo
	store    *fakeReceiptStore
	takings  *stubTakings
	svc      *usecase.ExpenseService
	clock    time.Time
	sequence int
}

func newExpenseFixture() *expenseFixture {
	f := &expenseFixture{repo: newFakeExpenseRepo(), store: newFakeReceiptStore(), takings: &stubTakings{}, clock: expNow}
	f.takings.cut = usecase.CashUp{ByMethod: map[string]int64{}}
	f.svc = usecase.NewExpenseService(f.repo, f.takings, f.store,
		func() string { f.sequence++; return fmt.Sprintf("id-%d", f.sequence) },
		func() time.Time { return f.clock })
	return f
}

func (f *expenseFixture) category(t *testing.T, name string) domain.ExpenseCategory {
	t.Helper()
	c, err := f.svc.CreateCategory(context.Background(), name)
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	return c
}

func (f *expenseFixture) expense(t *testing.T, categoryID string, cents int64, method string, day time.Time) domain.Expense {
	t.Helper()
	e, err := f.svc.CreateExpense(context.Background(), domain.ExpenseDraft{
		SpentOn: day, CategoryID: categoryID, AmountCents: cents, Method: method, Description: "gasto",
	}, cashier)
	if err != nil {
		t.Fatalf("create expense: %v", err)
	}
	return e
}

func TestCreateCategoryRefusesADuplicateName(t *testing.T) {
	f := newExpenseFixture()
	f.category(t, "Papelería")
	if _, err := f.svc.CreateCategory(context.Background(), "papelería"); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("want KindConflict, got %v", err)
	}
}

func TestUpdateCategoryDeactivates(t *testing.T) {
	f := newExpenseFixture()
	c := f.category(t, "Papelería")
	got, err := f.svc.UpdateCategory(context.Background(), c.ID(), "Papelería y tóner", false)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Active() || got.Name() != "Papelería y tóner" {
		t.Fatalf("got %+v", got)
	}
	active, _ := f.svc.ListCategories(context.Background(), false)
	if len(active) != 0 {
		t.Fatalf("an inactive category is hidden by default, got %d", len(active))
	}
}

func TestCreateExpenseRecordsTheCreatedEvent(t *testing.T) {
	f := newExpenseFixture()
	c := f.category(t, "Papelería")
	e := f.expense(t, c.ID(), 500_00, domain.MethodCash, expToday)
	_, events, err := f.svc.GetExpense(context.Background(), e.ID())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(events) != 1 || events[0].Kind != domain.ExpenseEventCreated || events[0].By != cashier || !events[0].At.Equal(expNow) {
		t.Fatalf("events = %+v", events)
	}
	if events[0].After.Amount().Int64() != 500_00 {
		t.Fatalf("the event keeps the expense as created")
	}
}

func TestCreateExpenseNeedsAnActiveCategory(t *testing.T) {
	f := newExpenseFixture()
	c := f.category(t, "Papelería")
	if _, err := f.svc.UpdateCategory(context.Background(), c.ID(), c.Name(), false); err != nil {
		t.Fatal(err)
	}
	draft := domain.ExpenseDraft{SpentOn: expToday, CategoryID: c.ID(), AmountCents: 1, Method: domain.MethodCash}
	if _, err := f.svc.CreateExpense(context.Background(), draft, cashier); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("an inactive category: want KindInvalid, got %v", err)
	}
	draft.CategoryID = "no-such"
	if _, err := f.svc.CreateExpense(context.Background(), draft, cashier); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("an unknown category: want KindInvalid (not NotFound), got %v", err)
	}
}

func TestUpdateExpenseKeepsBeforeAndAfter(t *testing.T) {
	f := newExpenseFixture()
	c := f.category(t, "Papelería")
	e := f.expense(t, c.ID(), 500_00, domain.MethodCash, expToday)
	f.clock = expNow.Add(time.Hour)
	edited, err := f.svc.UpdateExpense(context.Background(), e.ID(), domain.ExpenseDraft{
		SpentOn: expToday.AddDate(0, 0, -1), CategoryID: c.ID(), AmountCents: 450_00, Method: domain.MethodCard, Description: "corrige",
	}, " tecleé mal ", cashier)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if edited.Amount().Int64() != 450_00 || edited.Method() != domain.MethodCard {
		t.Fatalf("got %+v", edited)
	}
	_, events, _ := f.svc.GetExpense(context.Background(), e.ID())
	last := events[len(events)-1]
	if last.Kind != domain.ExpenseEventEdited || last.Reason != "tecleé mal" {
		t.Fatalf("event = %+v", last)
	}
	if last.Before.Amount().Int64() != 500_00 || last.After.Amount().Int64() != 450_00 {
		t.Fatalf("history must hold both sides: %+v", last)
	}
}

// Moving an expense to an inactive category is refused; keeping the one it
// already has is not (the category may have been retired since).
func TestUpdateExpenseCategoryRules(t *testing.T) {
	f := newExpenseFixture()
	old := f.category(t, "Viejo")
	e := f.expense(t, old.ID(), 100, domain.MethodCash, expToday)
	retired := f.category(t, "Retirado")
	ctx := context.Background()
	if _, err := f.svc.UpdateCategory(ctx, old.ID(), old.Name(), false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.UpdateCategory(ctx, retired.ID(), retired.Name(), false); err != nil {
		t.Fatal(err)
	}
	keep := domain.ExpenseDraft{SpentOn: expToday, CategoryID: old.ID(), AmountCents: 200, Method: domain.MethodCash}
	if _, err := f.svc.UpdateExpense(ctx, e.ID(), keep, "", cashier); err != nil {
		t.Fatalf("keeping a retired category is allowed: %v", err)
	}
	move := keep
	move.CategoryID = retired.ID()
	if _, err := f.svc.UpdateExpense(ctx, e.ID(), move, "", cashier); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("moving to an inactive category: want KindInvalid, got %v", err)
	}
}

func TestCancelExpense(t *testing.T) {
	f := newExpenseFixture()
	c := f.category(t, "Papelería")
	e := f.expense(t, c.ID(), 500_00, domain.MethodCash, expToday)
	got, err := f.svc.CancelExpense(context.Background(), e.ID(), "duplicado", cashier)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if !got.Cancelled() || !got.CancelledAt().Equal(expNow) {
		t.Fatalf("got %+v", got)
	}
	if _, _, err := f.svc.GetExpense(context.Background(), e.ID()); err != nil {
		t.Fatalf("a cancelled expense is kept: %v", err)
	}
	_, events, _ := f.svc.GetExpense(context.Background(), e.ID())
	if events[len(events)-1].Kind != domain.ExpenseEventCancelled {
		t.Fatalf("no cancel event: %+v", events)
	}
}

func TestListExpensesValidatesTheRange(t *testing.T) {
	f := newExpenseFixture()
	if _, err := f.svc.ListExpenses(context.Background(), usecase.ExpenseQuery{From: expToday, To: expToday.AddDate(0, 0, -1)}); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("from after to: want KindInvalid, got %v", err)
	}
	if _, err := f.svc.ListExpenses(context.Background(), usecase.ExpenseQuery{From: expToday.AddDate(-2, 0, 0), To: expToday}); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("over a year: want KindInvalid, got %v", err)
	}
}

func TestAttachReceiptStoresTheFileAndReplacesThePrevious(t *testing.T) {
	f := newExpenseFixture()
	c := f.category(t, "Papelería")
	e := f.expense(t, c.ID(), 500_00, domain.MethodCash, expToday)
	ctx := context.Background()
	pdf := []byte("%PDF-1.4 receipt")
	first, err := f.svc.AttachReceipt(ctx, e.ID(), pdf, cashier)
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	ref, has := first.Receipt()
	if !has || ref.ContentType != "application/pdf" || ref.SizeBytes != len(pdf) {
		t.Fatalf("ref = %+v", ref)
	}
	if string(f.store.objects[ref.Key]) != string(pdf) {
		t.Fatalf("the bytes went nowhere")
	}
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n', 1}
	second, err := f.svc.AttachReceipt(ctx, e.ID(), png, cashier)
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	ref2, _ := second.Receipt()
	if ref2.Key == ref.Key {
		t.Fatalf("a replacement gets its own key")
	}
	if _, still := f.store.objects[ref.Key]; still {
		t.Fatalf("the previous file must be removed")
	}
	got, err := f.svc.Receipt(ctx, e.ID())
	if err != nil || got.ContentType() != "image/png" {
		t.Fatalf("read back: %v %q", err, got.ContentType())
	}
	_, events, _ := f.svc.GetExpense(ctx, e.ID())
	if events[len(events)-1].Kind != domain.ExpenseEventReceiptAttached {
		t.Fatalf("no history for the receipt: %+v", events)
	}
}

func TestAttachReceiptRefusesAnythingButImagesAndPDF(t *testing.T) {
	f := newExpenseFixture()
	c := f.category(t, "Papelería")
	e := f.expense(t, c.ID(), 500_00, domain.MethodCash, expToday)
	if _, err := f.svc.AttachReceipt(context.Background(), e.ID(), []byte("<svg onload=alert(1)>"), cashier); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("want KindInvalid, got %v", err)
	}
	if len(f.store.objects) != 0 {
		t.Fatalf("a refused file must not be stored")
	}
}

func TestAttachReceiptUploadsNothingForAnExpenseThatCannotTakeIt(t *testing.T) {
	f := newExpenseFixture()
	c := f.category(t, "Papelería")
	e := f.expense(t, c.ID(), 500_00, domain.MethodCash, expToday)
	ctx := context.Background()
	if _, err := f.svc.CancelExpense(ctx, e.ID(), "duplicado", cashier); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.AttachReceipt(ctx, e.ID(), []byte("%PDF-1.4"), cashier); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("want KindConflict, got %v", err)
	}
	if len(f.store.objects) != 0 || len(f.store.deleted) != 0 {
		t.Fatalf("nothing is uploaded for a cancelled expense: %d stored, %d deleted", len(f.store.objects), len(f.store.deleted))
	}
	if _, err := f.svc.AttachReceipt(ctx, "no-such", []byte("%PDF-1.4"), cashier); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Fatalf("an unknown expense: want KindNotFound, got %v", err)
	}
	if len(f.store.objects) != 0 {
		t.Fatalf("nothing is uploaded for an unknown expense")
	}
}

func TestReceiptsUnavailableWithoutAStore(t *testing.T) {
	repo := newFakeExpenseRepo()
	svc := usecase.NewExpenseService(repo, &stubTakings{}, nil, func() string { return "x" }, func() time.Time { return expNow })
	if _, err := svc.AttachReceipt(context.Background(), "e", []byte("%PDF-"), cashier); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Fatalf("want KindNotFound, got %v", err)
	}
}

func TestRemoveReceipt(t *testing.T) {
	f := newExpenseFixture()
	c := f.category(t, "Papelería")
	e := f.expense(t, c.ID(), 500_00, domain.MethodCash, expToday)
	ctx := context.Background()
	with, _ := f.svc.AttachReceipt(ctx, e.ID(), []byte("%PDF-1.4"), cashier)
	ref, _ := with.Receipt()
	got, err := f.svc.RemoveReceipt(ctx, e.ID(), cashier)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, has := got.Receipt(); has {
		t.Fatalf("still has a receipt")
	}
	if _, still := f.store.objects[ref.Key]; still {
		t.Fatalf("the file must go")
	}
	if _, err := f.svc.RemoveReceipt(ctx, e.ID(), cashier); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Fatalf("removing nothing: want KindNotFound, got %v", err)
	}
}

func TestSummaryIsIncomeMinusExpensesByMethod(t *testing.T) {
	f := newExpenseFixture()
	c := f.category(t, "Papelería")
	f.expense(t, c.ID(), 300_00, domain.MethodCash, expToday)
	f.expense(t, c.ID(), 200_00, domain.MethodTransfer, expToday)
	cancelled := f.expense(t, c.ID(), 999_00, domain.MethodCash, expToday)
	if _, err := f.svc.CancelExpense(context.Background(), cancelled.ID(), "no", cashier); err != nil {
		t.Fatal(err)
	}
	f.repo.income = domain.MethodTotals{CashCents: 1000_00, CardCents: 500_00, TotalCents: 1500_00}
	f.repo.creditNote = 50_00

	s, err := f.svc.Summary(context.Background(), expToday, expToday)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if s.Expenses.TotalCents != 500_00 {
		t.Fatalf("a cancelled expense counts for nothing: %+v", s.Expenses)
	}
	want := domain.MethodTotals{CashCents: 700_00, CardCents: 500_00, TransferCents: -200_00, TotalCents: 1000_00}
	if s.Balance != want {
		t.Fatalf("balance = %+v, want %+v", s.Balance, want)
	}
	if s.CreditNoteCents != 50_00 || s.Income.TotalCents != 1500_00 {
		t.Fatalf("credit notes stay apart from income: %+v", s)
	}
	if len(s.ByCategory) != 1 || s.ByCategory[0].AmountCents != 500_00 || s.ByCategory[0].Count != 2 {
		t.Fatalf("by category = %+v", s.ByCategory)
	}
}

func dayCut() usecase.CashUp {
	return usecase.CashUp{
		ByMethod:        map[string]int64{domain.MethodCash: 2000_00, domain.MethodCard: 700_00},
		TotalCents:      2700_00,
		CreditNoteCents: 100_00,
	}
}

func TestCashCutAddsTheDaysExpensesAndExpectedCash(t *testing.T) {
	f := newExpenseFixture()
	f.takings.cut = dayCut()
	c := f.category(t, "Papelería")
	f.expense(t, c.ID(), 300_00, domain.MethodCash, expToday)
	f.expense(t, c.ID(), 100_00, domain.MethodCard, expToday)
	f.expense(t, c.ID(), 50_00, domain.MethodCash, expToday.AddDate(0, 0, -1))

	cut, err := f.svc.CashCut(context.Background(), expToday)
	if err != nil {
		t.Fatalf("cut: %v", err)
	}
	if cut.ExpensesByMethod.CashCents != 300_00 || cut.ExpensesByMethod.TotalCents != 400_00 {
		t.Fatalf("only the day's expenses: %+v", cut.ExpensesByMethod)
	}
	if cut.ExpectedCashCents != 1700_00 {
		t.Fatalf("expected cash = 2000 − 300, got %d", cut.ExpectedCashCents)
	}
	if len(cut.Expenses) != 2 || cut.Closure != nil || cut.ModifiedAfterClose {
		t.Fatalf("got %+v", cut)
	}
}

// The cut lists cancelled expenses (they explain the receipts) but does not
// count them.
func TestCashCutListsCancelledExpensesWithoutCountingThem(t *testing.T) {
	f := newExpenseFixture()
	c := f.category(t, "Papelería")
	e := f.expense(t, c.ID(), 300_00, domain.MethodCash, expToday)
	if _, err := f.svc.CancelExpense(context.Background(), e.ID(), "no", cashier); err != nil {
		t.Fatal(err)
	}
	cut, _ := f.svc.CashCut(context.Background(), expToday)
	if len(cut.Expenses) != 1 || cut.ExpensesByMethod.TotalCents != 0 {
		t.Fatalf("got %d listed, %d counted", len(cut.Expenses), cut.ExpensesByMethod.TotalCents)
	}
}

func TestCloseCashCutKeepsTheSnapshot(t *testing.T) {
	f := newExpenseFixture()
	f.takings.cut = dayCut()
	c := f.category(t, "Papelería")
	f.expense(t, c.ID(), 300_00, domain.MethodCash, expToday)
	f.clock = expNow.Add(2 * time.Hour)

	closure, err := f.svc.CloseCashCut(context.Background(), expToday, 1650_00, "faltan 50", cashier)
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if closure.ExpectedCashCents() != 1700_00 || closure.DifferenceCents() != -50_00 || closure.ClosedBy() != cashier {
		t.Fatalf("got expected %d diff %d", closure.ExpectedCashCents(), closure.DifferenceCents())
	}
	if closure.Figures().CreditNoteCents != 100_00 || closure.Figures().Income.CardCents != 700_00 {
		t.Fatalf("snapshot = %+v", closure.Figures())
	}
	cut, _ := f.svc.CashCut(context.Background(), expToday)
	if cut.Closure == nil || cut.Closure.ID() != closure.ID() || cut.ModifiedAfterClose {
		t.Fatalf("the cut must show its closure, untouched: %+v", cut)
	}
	if _, err := f.svc.CloseCashCut(context.Background(), expToday, 0, "", cashier); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("closing twice: want KindConflict, got %v", err)
	}
}

func TestCashCutSaysModifiedAfterClose(t *testing.T) {
	f := newExpenseFixture()
	c := f.category(t, "Papelería")
	e := f.expense(t, c.ID(), 300_00, domain.MethodCash, expToday)
	f.clock = expNow.Add(time.Hour)
	if _, err := f.svc.CloseCashCut(context.Background(), expToday, 0, "", cashier); err != nil {
		t.Fatal(err)
	}
	f.clock = expNow.Add(2 * time.Hour)
	if _, err := f.svc.CancelExpense(context.Background(), e.ID(), "error", cashier); err != nil {
		t.Fatal(err)
	}
	cut, err := f.svc.CashCut(context.Background(), expToday)
	if err != nil {
		t.Fatal(err)
	}
	if !cut.ModifiedAfterClose {
		t.Fatalf("an expense of the day cancelled after closing must flag the cut")
	}
}

// A payment correction (T9), a void or a late capture is a deposit change the
// repository detects; the use case only asks it.
func TestCashCutAsksTheRepositoryAboutDepositChanges(t *testing.T) {
	f := newExpenseFixture()
	if _, err := f.svc.CloseCashCut(context.Background(), expToday, 0, "", cashier); err != nil {
		t.Fatal(err)
	}
	f.repo.changedDays[expToday.Format("2006-01-02")] = true
	cut, _ := f.svc.CashCut(context.Background(), expToday)
	if !cut.ModifiedAfterClose {
		t.Fatalf("want modified after close")
	}
}

func TestReopenCashCut(t *testing.T) {
	f := newExpenseFixture()
	ctx := context.Background()
	if _, err := f.svc.ReopenCashCut(ctx, expToday, "motivo", cashier); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Fatalf("an open day cannot be reopened: want KindNotFound, got %v", err)
	}
	first, err := f.svc.CloseCashCut(ctx, expToday, 0, "", cashier)
	if err != nil {
		t.Fatal(err)
	}
	f.clock = expNow.Add(time.Hour)
	owner := domain.Receiver{UserID: "owner", Name: "Dirección"}
	reopened, err := f.svc.ReopenCashCut(ctx, expToday, "se corrigió un pago", owner)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if !reopened.Reopened() || reopened.ID() != first.ID() || reopened.ReopenedBy() != owner {
		t.Fatalf("got %+v", reopened)
	}
	cut, _ := f.svc.CashCut(ctx, expToday)
	if cut.Closure != nil || len(cut.Reopened) != 1 {
		t.Fatalf("an open day with one reopened closure: %+v", cut)
	}
	f.clock = expNow.Add(2 * time.Hour)
	if _, err := f.svc.CloseCashCut(ctx, expToday, 10_00, "", cashier); err != nil {
		t.Fatalf("a reopened day closes again: %v", err)
	}
}

func TestCloseCashCutRefusesAFutureDay(t *testing.T) {
	f := newExpenseFixture()
	if _, err := f.svc.CloseCashCut(context.Background(), expToday.AddDate(0, 0, 1), 0, "", cashier); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("want KindInvalid, got %v", err)
	}
}

func TestCashCutZeroDayIsToday(t *testing.T) {
	f := newExpenseFixture()
	if _, err := f.svc.CashCut(context.Background(), time.Time{}); err != nil {
		t.Fatal(err)
	}
	if !f.takings.askedOn.Equal(expToday) {
		t.Fatalf("asked %v, want today", f.takings.askedOn)
	}
}

var errBoom = errors.New("boom")

func TestAttachReceiptReportsAStoreFailure(t *testing.T) {
	f := newExpenseFixture()
	c := f.category(t, "Papelería")
	e := f.expense(t, c.ID(), 1, domain.MethodCash, expToday)
	f.store.putErr = errBoom
	if _, err := f.svc.AttachReceipt(context.Background(), e.ID(), []byte("%PDF-"), cashier); !errors.Is(err, errBoom) {
		t.Fatalf("want the store's error, got %v", err)
	}
	got, _, _ := f.svc.GetExpense(context.Background(), e.ID())
	if _, has := got.Receipt(); has {
		t.Fatalf("a failed upload attaches nothing")
	}
}

// A detach that committed is the answer even when the file could not be
// deleted: reporting it as a failure would send the office to retry a removal
// that already happened.
func TestRemoveReceiptSucceedsWhenTheFileDeleteFails(t *testing.T) {
	f := newExpenseFixture()
	c := f.category(t, "Papelería")
	e := f.expense(t, c.ID(), 1, domain.MethodCash, expToday)
	ctx := context.Background()
	if _, err := f.svc.AttachReceipt(ctx, e.ID(), []byte("%PDF-1.4"), cashier); err != nil {
		t.Fatal(err)
	}
	f.store.deleteErr = errBoom
	got, err := f.svc.RemoveReceipt(ctx, e.ID(), cashier)
	if err != nil {
		t.Fatalf("want success, got %v", err)
	}
	if _, has := got.Receipt(); has {
		t.Fatalf("still attached")
	}
}

// An expense re-filed by somebody else between the read and the lock is a
// conflict, not an edit applied over a category check that no longer holds.
func TestUpdateExpenseRefusesAnExpenseReFiledMeanwhile(t *testing.T) {
	f := newExpenseFixture()
	a := f.category(t, "A")
	b := f.category(t, "B")
	e := f.expense(t, a.ID(), 1, domain.MethodCash, expToday)
	ctx := context.Background()
	f.repo.beforeUpdate = func() {
		moved, _ := f.repo.expenses[e.ID()].Edit(domain.ExpenseDraft{SpentOn: expToday, CategoryID: b.ID(), AmountCents: 1, Method: domain.MethodCash}, expToday)
		f.repo.expenses[e.ID()] = moved
	}
	_, err := f.svc.UpdateExpense(ctx, e.ID(), domain.ExpenseDraft{SpentOn: expToday, CategoryID: a.ID(), AmountCents: 5, Method: domain.MethodCash}, "", cashier)
	if glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("want KindConflict, got %v", err)
	}
}
