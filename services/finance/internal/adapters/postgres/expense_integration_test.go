//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// T10 against real SQL. The container is shared by the whole package, so
// every test dates its rows in 2031 (no other test does) and owns its ids.

var expDay = time.Date(2031, 3, 10, 0, 0, 0, 0, time.UTC)

var expActor = domain.Receiver{UserID: "a0000000-0000-0000-0000-0000000000a1", Name: "Caja"}

func expCategory(t *testing.T, tenant, id, name string) domain.ExpenseCategory {
	t.Helper()
	c, err := domain.NewExpenseCategory(id, name, true)
	if err != nil {
		t.Fatalf("category: %v", err)
	}
	stored, err := appRepo(t, tenant).CreateExpenseCategory(context.Background(), c)
	if err != nil {
		t.Fatalf("CreateExpenseCategory: %v", err)
	}
	return stored
}

func expRecord(t *testing.T, tenant, id, eventID, categoryID string, cents int64, method string, day time.Time) domain.Expense {
	t.Helper()
	e, err := domain.NewExpense(id, domain.ExpenseDraft{
		SpentOn: day, CategoryID: categoryID, AmountCents: cents, Method: method, Description: "gasto",
	}, day, expActor)
	if err != nil {
		t.Fatalf("NewExpense: %v", err)
	}
	stored, err := appRepo(t, tenant).CreateExpense(context.Background(), e, domain.ExpenseEvent{
		ID: eventID, ExpenseID: id, Kind: domain.ExpenseEventCreated, After: e, By: expActor, At: day.Add(9 * time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateExpense: %v", err)
	}
	return stored
}

func TestExpenseCategoriesAreUniqueByNameAndTenant(t *testing.T) {
	ctx := context.Background()
	expCategory(t, tenantA, "c1000000-0000-0000-0000-000000000001", "Papelería")
	dup, _ := domain.NewExpenseCategory("c1000000-0000-0000-0000-000000000002", " papelería ", true)
	if _, err := appRepo(t, tenantA).CreateExpenseCategory(ctx, dup); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("same name, other case: want KindConflict, got %v", err)
	}
	// Another school may use the same name.
	other, _ := domain.NewExpenseCategory("c1000000-0000-0000-0000-000000000003", "Papelería", true)
	if _, err := appRepo(t, tenantB).CreateExpenseCategory(ctx, other); err != nil {
		t.Fatalf("tenant B's own category: %v", err)
	}
	if _, err := appRepo(t, tenantB).GetExpenseCategory(ctx, "c1000000-0000-0000-0000-000000000001"); !isNotFound(err) {
		t.Fatalf("RLS: tenant B read tenant A's category: %v", err)
	}
	if _, err := appRepo(t, tenantA).GetExpenseCategory(ctx, "not-a-uuid"); !isNotFound(err) {
		t.Fatalf("a malformed id is NotFound, got %v", err)
	}
}

func TestUpdateExpenseCategoryTwice(t *testing.T) {
	ctx := context.Background()
	c := expCategory(t, tenantA, "c1000000-0000-0000-0000-000000000011", "Limpieza")
	repo := appRepo(t, tenantA)
	for _, name := range []string{"Limpieza y aseo", "Aseo"} {
		next, _ := c.WithUpdate(name, false)
		if _, err := repo.UpdateExpenseCategory(ctx, next); err != nil {
			t.Fatalf("update to %q: %v", name, err)
		}
	}
	got, err := repo.GetExpenseCategory(ctx, c.ID())
	if err != nil || got.Name() != "Aseo" || got.Active() {
		t.Fatalf("got %+v, %v", got, err)
	}
	all, _ := repo.ListExpenseCategories(ctx, true)
	active, _ := repo.ListExpenseCategories(ctx, false)
	if !containsCategory(all, c.ID()) || containsCategory(active, c.ID()) {
		t.Fatalf("inactive categories are listed only when asked")
	}
}

func containsCategory(cs []domain.ExpenseCategory, id string) bool {
	for _, c := range cs {
		if c.ID() == id {
			return true
		}
	}
	return false
}

// Every mutable column survives a round trip, and an update applied TWICE to
// the same row writes the second value (the shape that broke three times in
// finance: a column missing from the update list).
func TestExpenseRoundTripAndHistory(t *testing.T) {
	ctx := context.Background()
	cat := expCategory(t, tenantA, "c1000000-0000-0000-0000-000000000021", "Mantenimiento")
	cat2 := expCategory(t, tenantA, "c1000000-0000-0000-0000-000000000022", "Jardinería")
	id := "e1000000-0000-0000-0000-000000000021"
	created := expRecord(t, tenantA, id, "f1000000-0000-0000-0000-000000000021", cat.ID(), 150000, domain.MethodCash, expDay)
	if created.CategoryName() != "Mantenimiento" || created.CreatedBy() != expActor || created.CreatedAt().IsZero() {
		t.Fatalf("created = %+v", created)
	}
	repo := appRepo(t, tenantA)
	edit := func(eventID string, d domain.ExpenseDraft, at time.Time) domain.Expense {
		got, err := repo.UpdateExpense(ctx, id, func(cur domain.Expense) (domain.Expense, domain.ExpenseEvent, error) {
			next, err := cur.Edit(d, expDay)
			if err != nil {
				return domain.Expense{}, domain.ExpenseEvent{}, err
			}
			return next, domain.ExpenseEvent{ID: eventID, ExpenseID: id, Kind: domain.ExpenseEventEdited, Before: cur, After: next, Reason: "ajuste", By: expActor, At: at}, nil
		})
		if err != nil {
			t.Fatalf("UpdateExpense: %v", err)
		}
		return got
	}
	edit("f1000000-0000-0000-0000-000000000022", domain.ExpenseDraft{
		SpentOn: expDay.AddDate(0, 0, -1), CategoryID: cat2.ID(), AmountCents: 120000, Method: domain.MethodCard, Description: "uno",
	}, expDay.Add(10*time.Hour))
	got := edit("f1000000-0000-0000-0000-000000000023", domain.ExpenseDraft{
		SpentOn: expDay.AddDate(0, 0, -2), CategoryID: cat.ID(), AmountCents: 110000, Method: domain.MethodTransfer, Description: "dos",
	}, expDay.Add(11*time.Hour))
	if !got.SpentOn().Equal(expDay.AddDate(0, 0, -2)) || got.CategoryID() != cat.ID() || got.CategoryName() != "Mantenimiento" ||
		got.Amount().Int64() != 110000 || got.Method() != domain.MethodTransfer || got.Description() != "dos" {
		t.Fatalf("second edit lost: %+v", got)
	}

	withReceipt, err := repo.UpdateExpense(ctx, id, func(cur domain.Expense) (domain.Expense, domain.ExpenseEvent, error) {
		next, err := cur.WithReceipt(domain.ExpenseReceiptRef{Key: "expenses/" + id + "/r1", ContentType: "application/pdf", SizeBytes: 2048})
		return next, domain.ExpenseEvent{ID: "f1000000-0000-0000-0000-000000000024", ExpenseID: id, Kind: domain.ExpenseEventReceiptAttached, Before: cur, After: next, By: expActor, At: expDay.Add(12 * time.Hour)}, err
	})
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if ref, has := withReceipt.Receipt(); !has || ref.SizeBytes != 2048 || ref.ContentType != "application/pdf" {
		t.Fatalf("receipt = %+v", ref)
	}
	cancelled, err := repo.UpdateExpense(ctx, id, func(cur domain.Expense) (domain.Expense, domain.ExpenseEvent, error) {
		next, err := cur.Cancel("duplicado", expDay.Add(13*time.Hour))
		return next, domain.ExpenseEvent{ID: "f1000000-0000-0000-0000-000000000025", ExpenseID: id, Kind: domain.ExpenseEventCancelled, Before: cur, After: next, Reason: "duplicado", By: expActor, At: expDay.Add(13 * time.Hour)}, err
	})
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if !cancelled.Cancelled() || cancelled.CancelReason() != "duplicado" || cancelled.CancelledAt().IsZero() {
		t.Fatalf("cancelled = %+v", cancelled)
	}
	if _, has := cancelled.Receipt(); !has {
		t.Fatalf("cancelling keeps the receipt")
	}

	events, err := repo.ListExpenseEvents(ctx, id)
	if err != nil {
		t.Fatalf("ListExpenseEvents: %v", err)
	}
	kinds := []string{}
	for _, ev := range events {
		kinds = append(kinds, ev.Kind)
	}
	want := []string{"created", "edited", "edited", "receipt_attached", "cancelled"}
	if len(kinds) != len(want) {
		t.Fatalf("kinds = %v", kinds)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("kinds = %v, want %v", kinds, want)
		}
	}
	if events[0].Before.ID() != "" || events[1].Before.Amount().Int64() != 150000 || events[1].After.Amount().Int64() != 120000 {
		t.Fatalf("history sides wrong: %+v", events[1])
	}
	if events[1].By != expActor || events[1].Reason != "ajuste" {
		t.Fatalf("who / why lost: %+v", events[1])
	}

	if _, err := appRepo(t, tenantB).GetExpense(ctx, id); !isNotFound(err) {
		t.Fatalf("RLS: tenant B read tenant A's expense: %v", err)
	}
	if _, err := appRepo(t, tenantB).UpdateExpense(ctx, id, func(cur domain.Expense) (domain.Expense, domain.ExpenseEvent, error) {
		return cur, domain.ExpenseEvent{}, nil
	}); !isNotFound(err) {
		t.Fatalf("RLS: tenant B updated tenant A's expense: %v", err)
	}
}

func TestListExpensesAndTotals(t *testing.T) {
	ctx := context.Background()
	day := time.Date(2031, 4, 15, 0, 0, 0, 0, time.UTC)
	cat := expCategory(t, tenantA, "c1000000-0000-0000-0000-000000000031", "Nómina")
	cat2 := expCategory(t, tenantA, "c1000000-0000-0000-0000-000000000032", "Luz")
	expRecord(t, tenantA, "e1000000-0000-0000-0000-000000000031", "f1000000-0000-0000-0000-000000000031", cat.ID(), 500000, domain.MethodTransfer, day)
	expRecord(t, tenantA, "e1000000-0000-0000-0000-000000000032", "f1000000-0000-0000-0000-000000000032", cat2.ID(), 30000, domain.MethodCash, day)
	gone := expRecord(t, tenantA, "e1000000-0000-0000-0000-000000000033", "f1000000-0000-0000-0000-000000000033", cat2.ID(), 99900, domain.MethodCash, day)
	expRecord(t, tenantA, "e1000000-0000-0000-0000-000000000034", "f1000000-0000-0000-0000-000000000034", cat2.ID(), 1000, domain.MethodCash, day.AddDate(0, 0, 1))
	repo := appRepo(t, tenantA)
	if _, err := repo.UpdateExpense(ctx, gone.ID(), func(cur domain.Expense) (domain.Expense, domain.ExpenseEvent, error) {
		next, err := cur.Cancel("no", day)
		return next, domain.ExpenseEvent{ID: "f1000000-0000-0000-0000-000000000035", ExpenseID: cur.ID(), Kind: domain.ExpenseEventCancelled, Before: cur, After: next, By: expActor, At: day}, err
	}); err != nil {
		t.Fatal(err)
	}

	live, err := repo.ListExpenses(ctx, ports.ExpenseFilter{From: day, To: day})
	if err != nil || len(live) != 2 {
		t.Fatalf("live = %d, %v", len(live), err)
	}
	all, _ := repo.ListExpenses(ctx, ports.ExpenseFilter{From: day, To: day, IncludeCancelled: true})
	byCat, _ := repo.ListExpenses(ctx, ports.ExpenseFilter{From: day, To: day.AddDate(0, 0, 1), CategoryID: cat2.ID()})
	if len(all) != 3 || len(byCat) != 2 {
		t.Fatalf("all = %d, by category = %d", len(all), len(byCat))
	}

	totals, categories, err := repo.ExpenseTotals(ctx, day, day)
	if err != nil {
		t.Fatalf("ExpenseTotals: %v", err)
	}
	want := domain.MethodTotals{CashCents: 30000, TransferCents: 500000, TotalCents: 530000}
	if totals != want {
		t.Fatalf("totals = %+v, want %+v", totals, want)
	}
	if len(categories) != 2 || categories[0].CategoryName != "Nómina" || categories[1].Count != 1 {
		t.Fatalf("categories = %+v", categories)
	}
}

// Income by method counts the same money the cash-up does: live deposits'
// tenders and counter sales; credit notes apart; voided deposits not at all.
func TestIncomeByMethod(t *testing.T) {
	ctx := context.Background()
	tenant, student := tenantA, "80000000-0000-0000-0000-0000000010a1"
	paidOn := time.Date(2031, 5, 20, 15, 0, 0, 0, time.UTC)
	month := seedConceptAndCharge(t, tenant, "60000000-0000-0000-0000-0000000010a1", "colegiatura t10",
		"61000000-0000-0000-0000-0000000010a1", student, "70000000-0000-0000-0000-0000000010a1", 500000, 0,
		time.Date(2031, 5, 10, 0, 0, 0, 0, time.UTC))
	repo := appRepo(t, tenant)

	d := newDeposit(t, "62000000-0000-0000-0000-0000000010a1", tenant, student, 300000, paidOn, "t10-key-1")
	d, err := d.WithTenders([]domain.Tender{
		tender(t, domain.MethodCreditNote, 100000, "NC-1", ""),
		tender(t, domain.MethodCash, 200000, "", ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.RegisterDeposit(ctx, d, func([]ports.ChargeWithPaid) (ports.DepositWrite, error) {
		return ports.DepositWrite{Allocations: []domain.Payment{
			methodPayment(t, "90000000-0000-0000-0000-0000000010a1", tenant, month.ID(), 100000, paidOn, domain.MethodCreditNote, ""),
			methodPayment(t, "90000000-0000-0000-0000-0000000010a2", tenant, month.ID(), 200000, paidOn, domain.MethodCash, ""),
		}}, nil
	}, nil); err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}
	voided := newDeposit(t, "62000000-0000-0000-0000-0000000010a2", tenant, student, 7700, paidOn, "t10-key-2")
	if _, _, err := repo.RegisterDeposit(ctx, voided, func([]ports.ChargeWithPaid) (ports.DepositWrite, error) {
		return ports.DepositWrite{}, nil
	}, nil); err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}
	v, _ := voided.Void("error", paidOn)
	if err := repo.VoidDeposit(ctx, v); err != nil {
		t.Fatalf("VoidDeposit: %v", err)
	}

	income, creditNote, err := repo.IncomeByMethod(ctx, paidOn, paidOn)
	if err != nil {
		t.Fatalf("IncomeByMethod: %v", err)
	}
	if income.CashCents != 200000 || income.TotalCents != 200000 || creditNote != 100000 {
		t.Fatalf("income = %+v, credit note = %d", income, creditNote)
	}
	if other, _, _ := appRepo(t, tenantB).IncomeByMethod(ctx, paidOn, paidOn); other.TotalCents != 0 {
		t.Fatalf("RLS: tenant B sees tenant A's income: %+v", other)
	}
}

func TestCashCutClosuresAndChanges(t *testing.T) {
	ctx := context.Background()
	day := time.Date(2031, 6, 3, 0, 0, 0, 0, time.UTC)
	closedAt := day.Add(20 * time.Hour)
	repo := appRepo(t, tenantA)
	figures := domain.CutFigures{
		Income:   domain.MethodTotals{CashCents: 100000, CardCents: 5000, TotalCents: 105000},
		Expenses: domain.MethodTotals{CashCents: 20000, TotalCents: 20000}, CreditNoteCents: 300,
	}
	first, err := domain.CloseCashCut("d1000000-0000-0000-0000-000000000001", day, figures, 79000, "faltan 10", expActor, closedAt, closedAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveCashCutClosure(ctx, first); err != nil {
		t.Fatalf("SaveCashCutClosure: %v", err)
	}
	again, _ := domain.CloseCashCut("d1000000-0000-0000-0000-000000000002", day, figures, 0, "", expActor, closedAt, closedAt)
	if err := repo.SaveCashCutClosure(ctx, again); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("a second live closure: want KindConflict, got %v", err)
	}
	listed, err := repo.ListCashCutClosures(ctx, day)
	if err != nil || len(listed) != 1 {
		t.Fatalf("listed = %d, %v", len(listed), err)
	}
	got := listed[0]
	if got.Figures() != figures || got.CountedCashCents() != 79000 || got.DifferenceCents() != -1000 || got.ClosedBy() != expActor || got.Note() != "faltan 10" {
		t.Fatalf("closure came back as %+v", got)
	}

	changed, err := repo.DayChangedSince(ctx, day, closedAt)
	if err != nil || changed {
		t.Fatalf("nothing changed yet: %v %v", changed, err)
	}
	// An expense of the day created after the close flags the day; one dated
	// another day does not.
	cat := expCategory(t, tenantA, "c1000000-0000-0000-0000-000000000041", "Cafetería")
	other, _ := domain.NewExpense("e1000000-0000-0000-0000-000000000041", domain.ExpenseDraft{SpentOn: day.AddDate(0, 0, 1), CategoryID: cat.ID(), AmountCents: 1, Method: domain.MethodCash}, day.AddDate(0, 0, 1), expActor)
	if _, err := repo.CreateExpense(ctx, other, domain.ExpenseEvent{ID: "f1000000-0000-0000-0000-000000000041", ExpenseID: other.ID(), Kind: domain.ExpenseEventCreated, After: other, At: closedAt.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if changed, _ := repo.DayChangedSince(ctx, day, closedAt); changed {
		t.Fatalf("another day's expense must not flag this one")
	}
	// A receipt attached, or a description reworded, on an expense of the
	// closed day after closing moves no figure: not a change.
	ofDay, _ := domain.NewExpense("e1000000-0000-0000-0000-000000000042", domain.ExpenseDraft{SpentOn: day, CategoryID: cat.ID(), AmountCents: 7, Method: domain.MethodCash}, day, expActor)
	if _, err := repo.CreateExpense(ctx, ofDay, domain.ExpenseEvent{ID: "f1000000-0000-0000-0000-000000000043", ExpenseID: ofDay.ID(), Kind: domain.ExpenseEventCreated, After: ofDay, At: closedAt.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateExpense(ctx, ofDay.ID(), func(cur domain.Expense) (domain.Expense, domain.ExpenseEvent, error) {
		next, err := cur.WithReceipt(domain.ExpenseReceiptRef{Key: "k", ContentType: "image/png", SizeBytes: 1})
		return next, domain.ExpenseEvent{ID: "f1000000-0000-0000-0000-000000000044", ExpenseID: cur.ID(), Kind: domain.ExpenseEventReceiptAttached, Before: cur, After: next, At: closedAt.Add(time.Hour)}, err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateExpense(ctx, ofDay.ID(), func(cur domain.Expense) (domain.Expense, domain.ExpenseEvent, error) {
		next, err := cur.Edit(domain.ExpenseDraft{SpentOn: day, CategoryID: cat.ID(), AmountCents: 7, Method: domain.MethodCash, Description: "otra"}, day)
		return next, domain.ExpenseEvent{ID: "f1000000-0000-0000-0000-000000000045", ExpenseID: cur.ID(), Kind: domain.ExpenseEventEdited, Before: cur, After: next, At: closedAt.Add(time.Hour)}, err
	}); err != nil {
		t.Fatal(err)
	}
	if changed, _ := repo.DayChangedSince(ctx, day, closedAt); changed {
		t.Fatalf("a receipt or a reworded description must not flag the day")
	}
	// Moving that expense INTO the closed day after closing flags it.
	if _, err := repo.UpdateExpense(ctx, other.ID(), func(cur domain.Expense) (domain.Expense, domain.ExpenseEvent, error) {
		next, err := cur.Edit(domain.ExpenseDraft{SpentOn: day, CategoryID: cat.ID(), AmountCents: 1, Method: domain.MethodCash}, day.AddDate(0, 0, 1))
		return next, domain.ExpenseEvent{ID: "f1000000-0000-0000-0000-000000000042", ExpenseID: cur.ID(), Kind: domain.ExpenseEventEdited, Before: cur, After: next, At: closedAt.Add(2 * time.Hour)}, err
	}); err != nil {
		t.Fatal(err)
	}
	if changed, _ := repo.DayChangedSince(ctx, day, closedAt); !changed {
		t.Fatalf("an expense moved into the closed day must flag it")
	}

	reopened, err := got.Reopen("se movió un gasto", domain.Receiver{UserID: "a0000000-0000-0000-0000-0000000000b2", Name: "Dirección"}, closedAt.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ReopenCashCutClosure(ctx, reopened); err != nil {
		t.Fatalf("ReopenCashCutClosure: %v", err)
	}
	if err := repo.ReopenCashCutClosure(ctx, reopened); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("reopening twice: want KindConflict, got %v", err)
	}
	if err := repo.SaveCashCutClosure(ctx, again); err != nil {
		t.Fatalf("a reopened day closes again: %v", err)
	}
	history, _ := repo.ListCashCutClosures(ctx, day)
	if len(history) != 2 || history[1].ReopenReason() != "se movió un gasto" || history[1].ReopenedBy().Name != "Dirección" {
		t.Fatalf("history = %+v", history)
	}
	if theirs, _ := appRepo(t, tenantB).ListCashCutClosures(ctx, day); len(theirs) != 0 {
		t.Fatalf("RLS: tenant B sees tenant A's closures")
	}
}

// A deposit of a closed day voided after the close flags the day (the T9
// correction marks superseded_at the same way).
func TestDayChangedSinceSeesDepositChanges(t *testing.T) {
	ctx := context.Background()
	tenant, student := tenantA, "80000000-0000-0000-0000-0000000010b1"
	paidOn := time.Date(2031, 7, 1, 12, 0, 0, 0, time.UTC)
	repo := appRepo(t, tenant)
	d := newDeposit(t, "62000000-0000-0000-0000-0000000010b1", tenant, student, 5000, paidOn, "t10-key-b1")
	if _, _, err := repo.RegisterDeposit(ctx, d, func([]ports.ChargeWithPaid) (ports.DepositWrite, error) {
		return ports.DepositWrite{}, nil
	}, nil); err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}
	// 2031 is after created_at, which the database stamped with today's clock.
	closedAt := paidOn.Add(8 * time.Hour)
	if changed, _ := repo.DayChangedSince(ctx, paidOn, closedAt); changed {
		t.Fatalf("captured before the close: not a change")
	}
	v, _ := d.Void("error", closedAt.Add(time.Minute))
	if err := repo.VoidDeposit(ctx, v); err != nil {
		t.Fatal(err)
	}
	if changed, _ := repo.DayChangedSince(ctx, paidOn, closedAt); !changed {
		t.Fatalf("a void after the close must flag the day")
	}
}

// C2-015: an expense another module records on its own behalf carries its
// reference; the same reference twice is the same expense — a double click or
// a lost response must not refund a family twice.
func TestCreateExpenseOnceIsKeyedByItsReference(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	cat := expCategory(t, tenantA, "c1000000-0000-0000-0000-0000000000c5", "Devoluciones c215")
	draft := domain.ExpenseDraft{SpentOn: expDay, CategoryID: cat.ID(), AmountCents: 25000, Method: domain.MethodCash, Description: "Devolución U-2026-000002"}
	mk := func(id, eventID string) (domain.Expense, domain.ExpenseEvent) {
		e, err := domain.NewExpense(id, draft, expDay, expActor)
		if err != nil {
			t.Fatalf("NewExpense: %v", err)
		}
		return e, domain.ExpenseEvent{ID: eventID, ExpenseID: id, Kind: domain.ExpenseEventCreated, After: e, By: expActor, At: expDay}
	}

	e1, ev1 := mk("e1000000-0000-0000-0000-0000000000c5", "e2000000-0000-0000-0000-0000000000c5")
	first, err := repo.CreateExpenseOnce(ctx, e1, ev1, "uniforms-return:f0000000-0000-0000-0000-0000000000c5")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	e2, ev2 := mk("e1000000-0000-0000-0000-0000000000c6", "e2000000-0000-0000-0000-0000000000c6")
	second, err := repo.CreateExpenseOnce(ctx, e2, ev2, "uniforms-return:f0000000-0000-0000-0000-0000000000c5")
	if err != nil {
		t.Fatalf("second: %v", err)
	}

	if second.ID() != first.ID() {
		t.Errorf("second = %s, want the first expense %s", second.ID(), first.ID())
	}
	var n int
	if err := superDB.Raw(`SELECT count(*) FROM expenses WHERE external_ref = ?`, "uniforms-return:f0000000-0000-0000-0000-0000000000c5").Scan(&n).Error; err != nil || n != 1 {
		t.Errorf("rows = %d (%v), want 1", n, err)
	}
}
