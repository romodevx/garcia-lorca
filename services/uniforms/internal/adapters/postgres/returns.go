package postgres

import (
	"context"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/uniforms/internal/domain"
	"github.com/garcia-lorca/services/uniforms/internal/ports"
)

// returnModel is the `uniform_returns` row: how a counter sale came back
// (migration 0011, C2-015).
type returnModel struct {
	FulfillmentID string    `gorm:"column:fulfillment_id;primaryKey"`
	TenantID      string    `gorm:"column:tenant_id"`
	OrderID       string    `gorm:"column:order_id"`
	Reason        string    `gorm:"column:reason"`
	RefundMode    string    `gorm:"column:refund_mode"`
	RefundMethod  string    `gorm:"column:refund_method"`
	RefundCents   int64     `gorm:"column:refund_cents"`
	RefundStatus  string    `gorm:"column:refund_status"`
	ExpenseID     *string   `gorm:"column:expense_id"`
	DamagedUnits  int       `gorm:"column:damaged_units"`
	ActorUserID   *string   `gorm:"column:actor_user_id"`
	ActorName     string    `gorm:"column:actor_name"`
	ReturnedAt    time.Time `gorm:"column:returned_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (returnModel) TableName() string { return "uniform_returns" }

// ReturnCounterSale takes a counter sale back in one transaction (C2-015).
//
// The delivery row is locked first and its status re-read under the lock: two
// clicks of "Devolver" queue on it, and the second finds it returned and
// answers with what the first did instead of putting the garments back twice.
func (r *Repository) ReturnCounterSale(ctx context.Context, cmd ports.ReturnCounterSaleCommand) (ports.OrderAggregate, error) {
	var out ports.OrderAggregate
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var ful fulfillmentModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", cmd.Fulfillment.ID()).First(&ful).Error; err != nil {
			return translateFindErr(err)
		}
		switch domain.FulfillmentStatus(ful.Status) {
		case domain.StatusReturned:
			agg, err := r.loadAggregate(tx, ful.OrderID)
			out = agg
			return err
		case domain.StatusSoldAtCounter:
		default:
			return glerrors.New(glerrors.KindConflict, "only a counter sale can be returned")
		}
		if err := r.applyReturn(tx, cmd); err != nil {
			return err
		}
		agg, err := r.loadAggregate(tx, ful.OrderID)
		out = agg
		return err
	})
	if err != nil {
		return ports.OrderAggregate{}, err
	}
	return out, nil
}

// applyReturn writes the return under the delivery's lock.
func (r *Repository) applyReturn(tx *gorm.DB, cmd ports.ReturnCounterSaleCommand) error {
	if err := tx.Model(&fulfillmentModel{}).Where("id = ?", cmd.Fulfillment.ID()).Updates(map[string]any{
		"status":     string(domain.StatusReturned),
		"updated_at": gorm.Expr("now()"),
	}).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "returning the delivery", err)
	}
	note := "Devolución " + cmd.Folio + ": " + cmd.Record.Reason
	// Stock rows are locked in one order, (item, size), the order every
	// writer of several sizes takes: two returns — or a return and a sale —
	// sharing two sizes cannot each hold one the other waits for.
	for _, line := range sortedBySize(cmd.Lines) {
		if err := tx.Model(&orderLineModel{}).Where("id = ?", line.ID()).Updates(map[string]any{
			"status":     string(domain.LineReturned),
			"updated_at": gorm.Expr("now()"),
		}).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "returning a line", err)
		}
		if err := r.backToShelf(tx, line, cmd.Damaged[line.ID()], note, cmd.Record.ActorUserID); err != nil {
			return err
		}
	}
	// The decision was stored by BeginReturn; what is learnt only now — how
	// many units came back damaged — is written onto it.
	row := returnRow(r.tenantID, cmd.Record)
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "fulfillment_id"}},
		DoUpdates: clause.Assignments(map[string]any{"damaged_units": cmd.Record.DamagedUnits, "updated_at": gorm.Expr("now()")}),
	}).Omit("returned_at", "updated_at").Create(&row).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "keeping the return", err)
	}
	return nil
}

func returnRow(tenantID string, rec ports.CounterSaleReturn) returnModel {
	return returnModel{
		FulfillmentID: rec.FulfillmentID, TenantID: tenantID, OrderID: rec.OrderID,
		Reason: rec.Reason, RefundMode: rec.RefundMode, RefundMethod: rec.RefundMethod,
		RefundCents: rec.RefundCents, RefundStatus: rec.RefundStatus, ExpenseID: nullable(rec.ExpenseID),
		DamagedUnits: rec.DamagedUnits, ActorUserID: nullable(rec.ActorUserID), ActorName: rec.ActorName,
	}
}

// sortedBySize is the lines in (item, size) order, the lock order.
func sortedBySize(lines []domain.OrderLine) []domain.OrderLine {
	out := slices.Clone(lines)
	slices.SortFunc(out, func(a, b domain.OrderLine) int {
		if c := strings.Compare(a.ItemID(), b.ItemID()); c != 0 {
			return c
		}
		return strings.Compare(a.SizeLabel(), b.SizeLabel())
	})
	return out
}

// BeginReturn stores the decision about a return before finance moves money.
// One already stored wins: the caller acts on what this returns.
func (r *Repository) BeginReturn(ctx context.Context, rec ports.CounterSaleReturn) (ports.CounterSaleReturn, error) {
	var out ports.CounterSaleReturn
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		row := returnRow(r.tenantID, rec)
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Omit("returned_at", "updated_at").Create(&row).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "recording the return's decision", err)
		}
		stored, err := returnsOf(tx, rec.OrderID)
		if err != nil {
			return err
		}
		got, ok := stored[rec.FulfillmentID]
		if !ok {
			return glerrors.New(glerrors.KindInternal, "the return's decision was not stored")
		}
		out = got
		return nil
	})
	return out, err
}

// DropReturnIntent forgets a decision nothing acted on, only while the sale
// is still sold: a returned sale keeps its record whatever happens.
func (r *Repository) DropReturnIntent(ctx context.Context, fulfillmentID string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		err := tx.Exec(`DELETE FROM uniform_returns
			WHERE fulfillment_id = ?
			  AND EXISTS (SELECT 1 FROM uniform_fulfillments f WHERE f.id = uniform_returns.fulfillment_id AND f.status = ?)`,
			fulfillmentID, string(domain.StatusSoldAtCounter)).Error
		if err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "dropping the return's decision", err)
		}
		return nil
	})
}

// backToShelf puts a returned line's units back ('return'). A damaged one is
// written off in the same breath ('adjust'): the ledger keeps both facts —
// it came back, and it cannot be sold — and the count only rises for what can.
func (r *Repository) backToShelf(tx *gorm.DB, line domain.OrderLine, damaged bool, note, actorUserID string) error {
	var stock stockModel
	if err := tx.Where("item_id = ? AND size_label = ?", line.ItemID(), line.SizeLabel()).First(&stock).Error; err != nil {
		return translateFindErr(err)
	}
	if err := r.appendMove(tx, stock.ID, line.ID(), "return", line.Qty(), note, actorUserID); err != nil {
		return err
	}
	if damaged {
		return r.appendMove(tx, stock.ID, line.ID(), "adjust", -line.Qty(), "Merma: "+note, actorUserID)
	}
	if err := tx.Model(&stockModel{}).Where("id = ?", stock.ID).Updates(map[string]any{
		"on_hand":    gorm.Expr("on_hand + ?", line.Qty()),
		"updated_at": gorm.Expr("now()"),
	}).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "putting the garment back", err)
	}
	return nil
}

// SettleReturnRefund marks a return's refund as recorded by its expense.
func (r *Repository) SettleReturnRefund(ctx context.Context, fulfillmentID, expenseID string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		// Only a pending refund is settled: a second click finishing the same
		// refund finds it done and leaves the first expense in place.
		res := tx.Model(&returnModel{}).Where("fulfillment_id = ? AND refund_status = ?", fulfillmentID, ports.RefundPending).
			Updates(map[string]any{
				"refund_status": ports.RefundDone,
				"expense_id":    nullable(expenseID),
				"updated_at":    gorm.Expr("now()"),
			})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "settling the refund", res.Error)
		}
		return nil
	})
}

// returnsOf reads the returns of one order, by fulfillment id.
func returnsOf(tx *gorm.DB, orderID string) (map[string]ports.CounterSaleReturn, error) {
	var rows []returnModel
	if err := tx.Where("order_id = ?", orderID).Find(&rows).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading returns", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	out := make(map[string]ports.CounterSaleReturn, len(rows))
	for _, m := range rows {
		out[m.FulfillmentID] = ports.CounterSaleReturn{
			FulfillmentID: m.FulfillmentID, OrderID: m.OrderID, Reason: m.Reason,
			RefundMode: m.RefundMode, RefundMethod: m.RefundMethod, RefundCents: m.RefundCents,
			RefundStatus: m.RefundStatus, ExpenseID: deref(m.ExpenseID), DamagedUnits: m.DamagedUnits,
			ActorUserID: deref(m.ActorUserID), ActorName: m.ActorName, ReturnedAt: m.ReturnedAt,
		}
	}
	return out, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
