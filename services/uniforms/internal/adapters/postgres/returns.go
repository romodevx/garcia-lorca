package postgres

import (
	"context"
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
	for _, line := range cmd.Lines {
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
	rec := cmd.Record
	row := returnModel{
		FulfillmentID: rec.FulfillmentID, TenantID: r.tenantID, OrderID: rec.OrderID,
		Reason: rec.Reason, RefundMode: rec.RefundMode, RefundMethod: rec.RefundMethod,
		RefundCents: rec.RefundCents, RefundStatus: rec.RefundStatus, ExpenseID: nullable(rec.ExpenseID),
		DamagedUnits: rec.DamagedUnits, ActorUserID: nullable(rec.ActorUserID), ActorName: rec.ActorName,
	}
	if err := tx.Omit("returned_at", "updated_at").Create(&row).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "keeping the return", err)
	}
	return nil
}

// backToShelf puts a returned line's units back ('return'). A damaged one is
// written off in the same breath ('adjust'): the ledger keeps both facts —
// it came back, and it cannot be sold — and the count only rises for what can.
func (r *Repository) backToShelf(tx *gorm.DB, line domain.OrderLine, damaged bool, note, actorUserID string) error {
	var stock stockModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("item_id = ? AND size_label = ?", line.ItemID(), line.SizeLabel()).First(&stock).Error; err != nil {
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
		res := tx.Model(&returnModel{}).Where("fulfillment_id = ?", fulfillmentID).Updates(map[string]any{
			"refund_status": ports.RefundDone,
			"expense_id":    nullable(expenseID),
			"updated_at":    gorm.Expr("now()"),
		})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "settling the refund", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
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
