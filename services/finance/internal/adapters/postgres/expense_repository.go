package postgres

import (
	"context"
	"encoding/json"
	"regexp"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// T10 (migration 0024): expenses, their categories and history, and the cash
// cut's closures. Same Repository, same tenant binding, its own port.

var _ ports.ExpenseRepository = (*Repository)(nil)

// uuidShaped is the canonical 8-4-4-4-12 form. An id of any other shape is
// "no such row", never a query Postgres refuses (22P02) as an internal error.
var uuidShaped = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

const (
	errCategoryTaken = "ya existe una categoría con ese nombre"
	// expenseColumns reads an expense with its category's name.
	expenseColumns = "e.*, c.name AS category_name"
)

// --- categories ------------------------------------------------------------

// CreateExpenseCategory stores a category; a taken name is KindConflict.
func (r *Repository) CreateExpenseCategory(ctx context.Context, c domain.ExpenseCategory) (domain.ExpenseCategory, error) {
	m := expenseCategoryModel{ID: c.ID(), TenantID: r.tenantID, Name: c.Name(), Active: c.Active()}
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&m).Error; err != nil {
			if isUniqueViolation(err) {
				return glerrors.Wrap(glerrors.KindConflict, errCategoryTaken, err)
			}
			return glerrors.Wrap(glerrors.KindInternal, "creating an expense category", err)
		}
		return nil
	})
	if err != nil {
		return domain.ExpenseCategory{}, err
	}
	return c, nil
}

// GetExpenseCategory reads one category, or KindNotFound.
func (r *Repository) GetExpenseCategory(ctx context.Context, id string) (domain.ExpenseCategory, error) {
	if !uuidShaped.MatchString(id) {
		return domain.ExpenseCategory{}, glerrors.ErrNotFound
	}
	var m expenseCategoryModel
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		return nil
	})
	if err != nil {
		return domain.ExpenseCategory{}, err
	}
	return domain.NewExpenseCategory(m.ID, m.Name, m.Active)
}

// ListExpenseCategories lists the categories by name.
func (r *Repository) ListExpenseCategories(ctx context.Context, includeInactive bool) ([]domain.ExpenseCategory, error) {
	var rows []expenseCategoryModel
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Model(&expenseCategoryModel{}).Order("lower(name) ASC").Limit(maxListLimit)
		if !includeInactive {
			q = q.Where("active")
		}
		if err := q.Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing expense categories", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.ExpenseCategory, 0, len(rows))
	for _, m := range rows {
		c, err := domain.NewExpenseCategory(m.ID, m.Name, m.Active)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// UpdateExpenseCategory renames / (de)activates a category.
func (r *Repository) UpdateExpenseCategory(ctx context.Context, c domain.ExpenseCategory) (domain.ExpenseCategory, error) {
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&expenseCategoryModel{}).Where("id = ?", c.ID()).Updates(map[string]any{
			"name": c.Name(), "active": c.Active(), "updated_at": gorm.Expr("now()"),
		})
		if res.Error != nil {
			if isUniqueViolation(res.Error) {
				return glerrors.Wrap(glerrors.KindConflict, errCategoryTaken, res.Error)
			}
			return glerrors.Wrap(glerrors.KindInternal, "updating an expense category", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
	})
	if err != nil {
		return domain.ExpenseCategory{}, err
	}
	return c, nil
}

// --- expenses --------------------------------------------------------------

// CreateExpense stores an expense and its "created" event in one transaction.
func (r *Repository) CreateExpense(ctx context.Context, e domain.Expense, created domain.ExpenseEvent) (domain.Expense, error) {
	var out domain.Expense
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		m := r.fromDomainExpense(e)
		if err := tx.Create(&m).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "recording an expense", err)
		}
		if err := r.insertExpenseEvent(tx, created); err != nil {
			return err
		}
		var err error
		out, err = readExpense(tx, e.ID(), false)
		return err
	})
	if err != nil {
		return domain.Expense{}, err
	}
	return out, nil
}

// UpdateExpense locks the expense, runs change, and stores the result with
// its event — one transaction, so two people editing the same expense queue
// on its row and each history entry's "before" is what that edit replaced.
func (r *Repository) UpdateExpense(ctx context.Context, id string, change ports.ExpenseChange) (domain.Expense, error) {
	if !uuidShaped.MatchString(id) {
		return domain.Expense{}, glerrors.ErrNotFound
	}
	var out domain.Expense
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		current, err := readExpense(tx, id, true)
		if err != nil {
			return err
		}
		next, ev, err := change(current)
		if err != nil {
			return err
		}
		if err := writeExpense(tx, next); err != nil {
			return err
		}
		if err := r.insertExpenseEvent(tx, ev); err != nil {
			return err
		}
		out, err = readExpense(tx, id, false)
		return err
	})
	if err != nil {
		return domain.Expense{}, err
	}
	return out, nil
}

// writeExpense updates every mutable column of an expense. Every column the
// domain can change is listed here: a field added to Expense and not to this
// map would be accepted, validated and silently dropped.
func writeExpense(tx *gorm.DB, e domain.Expense) error {
	ref, has := e.Receipt()
	values := map[string]any{
		"spent_on":             e.SpentOn(),
		"category_id":          e.CategoryID(),
		"amount_cents":         e.Amount().Int64(),
		"method":               e.Method(),
		"description":          e.Description(),
		"status":               e.Status(),
		"cancel_reason":        e.CancelReason(),
		"cancelled_at":         optionalInstant(e.CancelledAt()),
		"receipt_key":          nil,
		"receipt_content_type": nil,
		"receipt_size_bytes":   nil,
		"updated_at":           gorm.Expr("now()"),
	}
	if has {
		values["receipt_key"], values["receipt_content_type"], values["receipt_size_bytes"] = ref.Key, ref.ContentType, ref.SizeBytes
	}
	if err := tx.Model(&expenseModel{}).Where("id = ?", e.ID()).Updates(values).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "updating an expense", err)
	}
	return nil
}

func optionalInstant(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// readExpense reads one expense with its category's name, under a row lock
// when asked.
func readExpense(tx *gorm.DB, id string, lock bool) (domain.Expense, error) {
	q := tx.Table("expenses e").Select(expenseColumns).Joins("JOIN expense_categories c ON c.id = e.category_id").Where("e.id = ?", id)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE", Table: clause.Table{Name: "e"}})
	}
	var rows []expenseModel
	if err := q.Limit(1).Scan(&rows).Error; err != nil {
		return domain.Expense{}, glerrors.Wrap(glerrors.KindInternal, "reading an expense", err)
	}
	if len(rows) == 0 {
		return domain.Expense{}, glerrors.ErrNotFound
	}
	return toDomainExpense(rows[0]), nil
}

// GetExpense reads one expense, or KindNotFound.
func (r *Repository) GetExpense(ctx context.Context, id string) (domain.Expense, error) {
	if !uuidShaped.MatchString(id) {
		return domain.Expense{}, glerrors.ErrNotFound
	}
	var out domain.Expense
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		out, err = readExpense(tx, id, false)
		return err
	})
	return out, err
}

// ListExpenses lists by the filter, newest first, at most maxListLimit.
func (r *Repository) ListExpenses(ctx context.Context, f ports.ExpenseFilter) ([]domain.Expense, error) {
	if f.CategoryID != "" && !uuidShaped.MatchString(f.CategoryID) {
		return nil, nil
	}
	var rows []expenseModel
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Table("expenses e").Select(expenseColumns).
			Joins("JOIN expense_categories c ON c.id = e.category_id").
			Where("e.spent_on >= ? AND e.spent_on <= ?", f.From.Format(dateLayout), f.To.Format(dateLayout))
		if !f.IncludeCancelled {
			q = q.Where("e.status = ?", domain.ExpenseActive)
		}
		if f.CategoryID != "" {
			q = q.Where("e.category_id = ?", f.CategoryID)
		}
		if err := q.Order("e.spent_on DESC, e.created_at DESC, e.id DESC").Limit(maxListLimit).Scan(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing expenses", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Expense, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomainExpense(row))
	}
	return out, nil
}

// ListExpenseEvents is one expense's history, oldest first.
func (r *Repository) ListExpenseEvents(ctx context.Context, expenseID string) ([]domain.ExpenseEvent, error) {
	if !uuidShaped.MatchString(expenseID) {
		return nil, glerrors.ErrNotFound
	}
	var rows []expenseEventModel
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("expense_id = ?", expenseID).Order("at ASC, id ASC").Limit(maxListLimit).Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reading an expense's history", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.ExpenseEvent, 0, len(rows))
	for _, m := range rows {
		ev, err := toDomainExpenseEvent(m)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, nil
}

// ExpenseTotals adds up the live expenses of a period by method and category.
func (r *Repository) ExpenseTotals(ctx context.Context, from, to time.Time) (domain.MethodTotals, []ports.CategoryTotal, error) {
	var byMethod []struct {
		Method string
		Cents  int64
	}
	var byCategory []struct {
		CategoryID   string
		CategoryName string
		Cents        int64
		Count        int
	}
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		live := func() *gorm.DB {
			return tx.Table("expenses e").Where("e.status = ? AND e.spent_on >= ? AND e.spent_on <= ?",
				domain.ExpenseActive, from.Format(dateLayout), to.Format(dateLayout))
		}
		if err := live().Select("e.method AS method, COALESCE(SUM(e.amount_cents), 0) AS cents").
			Group("e.method").Scan(&byMethod).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "summing expenses by method", err)
		}
		if err := live().Select("e.category_id AS category_id, c.name AS category_name, " +
			"COALESCE(SUM(e.amount_cents), 0) AS cents, COUNT(*) AS count").
			Joins("JOIN expense_categories c ON c.id = e.category_id").
			Group("e.category_id, c.name").Order("cents DESC, c.name ASC").Scan(&byCategory).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "summing expenses by category", err)
		}
		return nil
	})
	if err != nil {
		return domain.MethodTotals{}, nil, err
	}
	var totals domain.MethodTotals
	for _, m := range byMethod {
		totals.Add(m.Method, m.Cents)
	}
	out := make([]ports.CategoryTotal, 0, len(byCategory))
	for _, c := range byCategory {
		out = append(out, ports.CategoryTotal{CategoryID: c.CategoryID, CategoryName: c.CategoryName, AmountCents: c.Cents, Count: c.Count})
	}
	return totals, out, nil
}

// IncomeByMethod adds up the money received in a period, on the day it changed
// hands: the tenders of deposits that count (not voided, not superseded) and
// the payments taken outside a deposit — the same money the cash-up counts.
func (r *Repository) IncomeByMethod(ctx context.Context, from, to time.Time) (domain.MethodTotals, int64, error) {
	start := domain.CalendarDay(from)
	end := domain.CalendarDay(to).AddDate(0, 0, 1)
	var rows []struct {
		Method string
		Cents  int64
	}
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		err := tx.Raw(`
SELECT method, COALESCE(SUM(cents), 0) AS cents FROM (
    SELECT t.method, t.amount_cents AS cents
      FROM deposit_tenders t JOIN deposits d ON d.id = t.deposit_id
     WHERE d.paid_at >= ? AND d.paid_at < ? AND d.voided_at IS NULL AND d.superseded_at IS NULL
    UNION ALL
    SELECT p.method, p.amount_cents AS cents
      FROM payments p
     WHERE p.deposit_id IS NULL AND p.paid_at >= ? AND p.paid_at < ?
) money GROUP BY method`, start, end, start, end).Scan(&rows).Error
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "summing income by method", err)
		}
		return nil
	})
	if err != nil {
		return domain.MethodTotals{}, 0, err
	}
	var totals domain.MethodTotals
	var creditNote int64
	for _, row := range rows {
		if row.Method == domain.MethodCreditNote {
			creditNote += row.Cents
			continue
		}
		totals.Add(row.Method, row.Cents)
	}
	return totals, creditNote, nil
}

// --- closures --------------------------------------------------------------

// ListCashCutClosures returns a day's closures, newest first.
func (r *Repository) ListCashCutClosures(ctx context.Context, day time.Time) ([]domain.CashCutClosure, error) {
	var rows []cashCutClosureModel
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("day = ?", day.Format(dateLayout)).Order("closed_at DESC, id DESC").
			Limit(maxListLimit).Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reading the day's closures", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.CashCutClosure, 0, len(rows))
	for _, m := range rows {
		c, err := toDomainClosure(m)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// SaveCashCutClosure stores a new closure; a day already closed (the partial
// unique index on live closures) is KindConflict.
func (r *Repository) SaveCashCutClosure(ctx context.Context, c domain.CashCutClosure) error {
	m, err := r.fromDomainClosure(c)
	if err != nil {
		return err
	}
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&m).Error; err != nil {
			if isUniqueViolation(err) {
				return glerrors.Wrap(glerrors.KindConflict, "el corte de ese día ya está cerrado", err)
			}
			return glerrors.Wrap(glerrors.KindInternal, "closing the cash cut", err)
		}
		return nil
	})
}

// ReopenCashCutClosure stamps a live closure reopened. A closure reopened in
// between is KindConflict: only one reopen wins.
func (r *Repository) ReopenCashCutClosure(ctx context.Context, c domain.CashCutClosure) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&cashCutClosureModel{}).Where("id = ? AND reopened_at IS NULL", c.ID()).Updates(map[string]any{
			"reopened_at":      c.ReopenedAt(),
			"reopened_by":      nullableUUID(c.ReopenedBy().UserID),
			"reopened_by_name": c.ReopenedBy().Name,
			"reopen_reason":    c.ReopenReason(),
		})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reopening the cash cut", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.New(glerrors.KindConflict, "el corte ya estaba reabierto")
		}
		return nil
	})
}

// moneyEvent keeps the expense events that move a day's figures: a record or
// a cancel, and an edit that changed the amount, the method or the day. A
// receipt attached the next morning or a reworded description changes no
// figure, and flagging the cut for it would make the warning noise.
const moneyEvent = `(kind IN ('created', 'cancelled') OR (kind = 'edited' AND (
        before->'amountCents' IS DISTINCT FROM after->'amountCents'
     OR before->'method' IS DISTINCT FROM after->'method'
     OR before->'spentOn' IS DISTINCT FROM after->'spentOn')))`

// DayChangedSince reports whether a movement dated day changed after since.
//
//   - a deposit dated that day captured, voided or superseded after it (a late
//     capture, a void, a T9 correction — whose new deposit is captured then);
//   - a payment taken outside a deposit (a counter sale) captured after it;
//   - an expense event after it that touches the day's money (moneyEvent):
//     dated it, or moved off it.
func (r *Repository) DayChangedSince(ctx context.Context, day, since time.Time) (bool, error) {
	start := domain.CalendarDay(day)
	end := start.AddDate(0, 0, 1)
	var changed bool
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		err := tx.Raw(`
SELECT EXISTS (
    SELECT 1 FROM deposits
     WHERE paid_at >= ? AND paid_at < ?
       AND (created_at > ? OR voided_at > ? OR superseded_at > ?)
) OR EXISTS (
    SELECT 1 FROM payments
     WHERE deposit_id IS NULL AND paid_at >= ? AND paid_at < ? AND created_at > ?
) OR EXISTS (
    SELECT 1 FROM expense_events
     WHERE spent_on = ? AND at > ? AND `+moneyEvent+`
) OR EXISTS (
    SELECT 1 FROM expense_events
     WHERE previous_spent_on = ? AND at > ? AND `+moneyEvent+`
)`, start, end, since, since, since, start, end, since,
			start.Format(dateLayout), since, start.Format(dateLayout), since).
			Scan(&changed).Error
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "checking the day for changes", err)
		}
		return nil
	})
	return changed, err
}

// --- mapping ---------------------------------------------------------------

func (r *Repository) fromDomainExpense(e domain.Expense) expenseModel {
	m := expenseModel{
		ID: e.ID(), TenantID: r.tenantID, SpentOn: e.SpentOn(), CategoryID: e.CategoryID(),
		AmountCents: e.Amount().Int64(), Method: e.Method(), Description: e.Description(), Status: e.Status(),
		CancelReason: e.CancelReason(), CancelledAt: optionalInstant(e.CancelledAt()),
		CreatedBy: nullableUUID(e.CreatedBy().UserID), CreatedByName: e.CreatedBy().Name,
	}
	if ref, has := e.Receipt(); has {
		key, contentType, size := ref.Key, ref.ContentType, ref.SizeBytes
		m.ReceiptKey, m.ReceiptContentType, m.ReceiptSizeBytes = &key, &contentType, &size
	}
	return m
}

func toDomainExpense(m expenseModel) domain.Expense {
	var ref domain.ExpenseReceiptRef
	if m.ReceiptKey != nil && m.ReceiptContentType != nil && m.ReceiptSizeBytes != nil {
		ref = domain.ExpenseReceiptRef{Key: *m.ReceiptKey, ContentType: *m.ReceiptContentType, SizeBytes: *m.ReceiptSizeBytes}
	}
	var cancelledAt time.Time
	if m.CancelledAt != nil {
		cancelledAt = *m.CancelledAt
	}
	by := domain.Receiver{Name: m.CreatedByName}
	if m.CreatedBy != nil {
		by.UserID = *m.CreatedBy
	}
	return domain.RehydrateExpense(m.ID, domain.ExpenseDraft{
		SpentOn: m.SpentOn, CategoryID: m.CategoryID, AmountCents: m.AmountCents, Method: m.Method, Description: m.Description,
	}, m.CategoryName, m.Status, m.CancelReason, cancelledAt, ref, by, m.CreatedAt, m.UpdatedAt)
}

// expenseSnapshotJSON is one side of a history row as stored.
type expenseSnapshotJSON struct {
	SpentOn      string `json:"spentOn"`
	CategoryID   string `json:"categoryId"`
	CategoryName string `json:"categoryName,omitempty"`
	AmountCents  int64  `json:"amountCents"`
	Method       string `json:"method"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	CancelReason string `json:"cancelReason,omitempty"`
	ReceiptKey   string `json:"receiptKey,omitempty"`
	ReceiptType  string `json:"receiptType,omitempty"`
	ReceiptSize  int    `json:"receiptSize,omitempty"`
}

func marshalExpenseSnapshot(e domain.Expense) ([]byte, error) {
	if e.ID() == "" {
		return []byte("{}"), nil
	}
	ref, _ := e.Receipt()
	raw, err := json.Marshal(expenseSnapshotJSON{
		SpentOn: e.SpentOn().Format(dateLayout), CategoryID: e.CategoryID(), CategoryName: e.CategoryName(),
		AmountCents: e.Amount().Int64(), Method: e.Method(), Description: e.Description(), Status: e.Status(),
		CancelReason: e.CancelReason(), ReceiptKey: ref.Key, ReceiptType: ref.ContentType, ReceiptSize: ref.SizeBytes,
	})
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "encoding an expense's history", err)
	}
	return raw, nil
}

func unmarshalExpenseSnapshot(id string, raw []byte) (domain.Expense, error) {
	var s expenseSnapshotJSON
	if err := json.Unmarshal(raw, &s); err != nil {
		return domain.Expense{}, glerrors.Wrap(glerrors.KindInternal, "decoding an expense's history", err)
	}
	if s.SpentOn == "" {
		return domain.Expense{}, nil
	}
	day, err := time.Parse(dateLayout, s.SpentOn)
	if err != nil {
		return domain.Expense{}, glerrors.Wrap(glerrors.KindInternal, "decoding an expense's history", err)
	}
	return domain.RehydrateExpense(id, domain.ExpenseDraft{
		SpentOn: day, CategoryID: s.CategoryID, AmountCents: s.AmountCents, Method: s.Method, Description: s.Description,
	}, s.CategoryName, s.Status, s.CancelReason, time.Time{},
		domain.ExpenseReceiptRef{Key: s.ReceiptKey, ContentType: s.ReceiptType, SizeBytes: s.ReceiptSize},
		domain.Receiver{}, time.Time{}, time.Time{}), nil
}

func (r *Repository) insertExpenseEvent(tx *gorm.DB, ev domain.ExpenseEvent) error {
	before, err := marshalExpenseSnapshot(ev.Before)
	if err != nil {
		return err
	}
	after, err := marshalExpenseSnapshot(ev.After)
	if err != nil {
		return err
	}
	previous := ev.After.SpentOn()
	if ev.Before.ID() != "" {
		previous = ev.Before.SpentOn()
	}
	m := expenseEventModel{
		ID: ev.ID, TenantID: r.tenantID, ExpenseID: ev.ExpenseID, Kind: ev.Kind, Before: before, After: after,
		Reason: ev.Reason, SpentOn: ev.After.SpentOn(), PreviousSpentOn: previous,
		ActorID: nullableUUID(ev.By.UserID), ActorName: ev.By.Name, At: ev.At,
	}
	if err := tx.Create(&m).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "recording an expense's history", err)
	}
	return nil
}

func toDomainExpenseEvent(m expenseEventModel) (domain.ExpenseEvent, error) {
	before, err := unmarshalExpenseSnapshot(m.ExpenseID, m.Before)
	if err != nil {
		return domain.ExpenseEvent{}, err
	}
	after, err := unmarshalExpenseSnapshot(m.ExpenseID, m.After)
	if err != nil {
		return domain.ExpenseEvent{}, err
	}
	by := domain.Receiver{Name: m.ActorName}
	if m.ActorID != nil {
		by.UserID = *m.ActorID
	}
	return domain.ExpenseEvent{
		ID: m.ID, ExpenseID: m.ExpenseID, Kind: m.Kind, Before: before, After: after, Reason: m.Reason, By: by, At: m.At,
	}, nil
}

// closureSnapshotJSON is a closure's figures as stored.
type closureSnapshotJSON struct {
	Income          methodTotalsJSON `json:"income"`
	Expenses        methodTotalsJSON `json:"expenses"`
	CreditNoteCents int64            `json:"creditNoteCents"`
}

type methodTotalsJSON struct {
	Cash     int64 `json:"cash"`
	Card     int64 `json:"card"`
	Transfer int64 `json:"transfer"`
	Total    int64 `json:"total"`
}

func toTotalsJSON(m domain.MethodTotals) methodTotalsJSON {
	return methodTotalsJSON{Cash: m.CashCents, Card: m.CardCents, Transfer: m.TransferCents, Total: m.TotalCents}
}

func (j methodTotalsJSON) toDomain() domain.MethodTotals {
	return domain.MethodTotals{CashCents: j.Cash, CardCents: j.Card, TransferCents: j.Transfer, TotalCents: j.Total}
}

func (r *Repository) fromDomainClosure(c domain.CashCutClosure) (cashCutClosureModel, error) {
	f := c.Figures()
	snapshot, err := json.Marshal(closureSnapshotJSON{
		Income: toTotalsJSON(f.Income), Expenses: toTotalsJSON(f.Expenses), CreditNoteCents: f.CreditNoteCents,
	})
	if err != nil {
		return cashCutClosureModel{}, glerrors.Wrap(glerrors.KindInternal, "encoding the closure", err)
	}
	return cashCutClosureModel{
		ID: c.ID(), TenantID: r.tenantID, Day: c.Day(), ExpectedCashCents: c.ExpectedCashCents(),
		CountedCashCents: c.CountedCashCents(), DifferenceCents: c.DifferenceCents(), Snapshot: snapshot,
		Note: c.Note(), ClosedBy: nullableUUID(c.ClosedBy().UserID), ClosedByName: c.ClosedBy().Name, ClosedAt: c.ClosedAt(),
	}, nil
}

func toDomainClosure(m cashCutClosureModel) (domain.CashCutClosure, error) {
	var s closureSnapshotJSON
	if err := json.Unmarshal(m.Snapshot, &s); err != nil {
		return domain.CashCutClosure{}, glerrors.Wrap(glerrors.KindInternal, "decoding the closure", err)
	}
	closedBy := domain.Receiver{Name: m.ClosedByName}
	if m.ClosedBy != nil {
		closedBy.UserID = *m.ClosedBy
	}
	reopenedBy := domain.Receiver{Name: m.ReopenedByName}
	if m.ReopenedBy != nil {
		reopenedBy.UserID = *m.ReopenedBy
	}
	var reopenedAt time.Time
	if m.ReopenedAt != nil {
		reopenedAt = *m.ReopenedAt
	}
	figures := domain.CutFigures{Income: s.Income.toDomain(), Expenses: s.Expenses.toDomain(), CreditNoteCents: s.CreditNoteCents}
	return domain.RehydrateCashCutClosure(m.ID, m.Day, figures, m.CountedCashCents, m.Note,
		closedBy, m.ClosedAt, reopenedAt, reopenedBy, m.ReopenReason), nil
}
