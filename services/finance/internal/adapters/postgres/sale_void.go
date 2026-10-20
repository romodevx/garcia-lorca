package postgres

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// uniformsSaleRefPrefix is how a uniforms counter sale names its charge
// (uniforms:<fulfillment id>, ADR 0008). Only such a charge may be voided by
// VoidSalePayments: a tuition payment is corrected through its deposit.
const uniformsSaleRefPrefix = "uniforms:"

// voidedSalePaymentModel is a sale payment moved out of `payments` by a
// same-day return (migration 0030).
type voidedSalePaymentModel struct {
	ID             string    `gorm:"column:id;primaryKey"`
	TenantID       string    `gorm:"column:tenant_id"`
	ChargeID       string    `gorm:"column:charge_id"`
	AmountCents    int64     `gorm:"column:amount_cents"`
	Method         string    `gorm:"column:method"`
	CardType       string    `gorm:"column:card_type"`
	Reference      string    `gorm:"column:reference"`
	IdempotencyKey string    `gorm:"column:idempotency_key"`
	ReceivedBy     *string   `gorm:"column:received_by"`
	PaidAt         time.Time `gorm:"column:paid_at"`
	PaidOn         time.Time `gorm:"column:paid_on"`
	Reason         string    `gorm:"column:reason"`
	VoidedBy       *string   `gorm:"column:voided_by"`
	VoidedAt       time.Time `gorm:"column:voided_at"`
}

func (voidedSalePaymentModel) TableName() string { return "voided_sale_payments" }

// VoidSalePayments voids a returned sale's payments and cancels its charge.
//
// The payments are MOVED, not flagged: every reader of `payments` (paid totals,
// the cash-up, income, receipts) counts what is there as money, and teaching
// each of them a void column is a dozen chances to forget one. The row is kept
// whole in voided_sale_payments with why, who and when, so the day's cancelled
// list and a closed cash-up still account for it.
func (r *Repository) VoidSalePayments(ctx context.Context, chargeID, reason, actorUserID string, today time.Time) ([]ports.VoidedSalePayment, error) {
	var out []ports.VoidedSalePayment
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var charge chargeModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", chargeID).First(&charge).Error; err != nil {
			return translateFindErr(err)
		}
		if !strings.HasPrefix(charge.ExternalRef, uniformsSaleRefPrefix) {
			return glerrors.New(glerrors.KindInvalid, "only a uniforms counter sale can be voided this way")
		}
		var rows []paymentModel
		if err := tx.Where("charge_id = ?", chargeID).Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reading the sale's payments", err)
		}
		if len(rows) == 0 && charge.Status == domain.StatusCancelled {
			// Already voided: a retry of a return that went through.
			done, err := voidedOf(tx, "charge_id = ?", chargeID)
			out = done
			return err
		}
		archive, err := voidableSalePayments(rows, reason, actorUserID, today)
		if err != nil {
			return err
		}
		if len(archive) > 0 {
			if err := tx.Omit("voided_at").Create(&archive).Error; err != nil {
				return glerrors.Wrap(glerrors.KindInternal, "keeping the voided payments", err)
			}
			if err := tx.Where("charge_id = ? AND deposit_id IS NULL", chargeID).Delete(&paymentModel{}).Error; err != nil {
				return glerrors.Wrap(glerrors.KindInternal, "voiding the sale's payments", err)
			}
		}
		if err := tx.Model(&chargeModel{}).Where("id = ?", chargeID).Updates(map[string]any{
			"status":              domain.StatusCancelled,
			"cancellation_reason": "Devolución: " + reason,
			"cancelled_at":        gorm.Expr("now()"),
			"cancelled_by":        nullableUUID(actorUserID),
			"updated_at":          gorm.Expr("now()"),
		}).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "cancelling the sale's charge", err)
		}
		done, err := voidedOf(tx, "charge_id = ?", chargeID)
		out = done
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// voidableSalePayments turns a sale's payments into their archive rows, or
// says why they cannot be voided: a deposit's slice is never a sale's, and a
// payment of an earlier day is already in a cash-up that may be closed — that
// return is an expense of its own day (C2-015).
func voidableSalePayments(rows []paymentModel, reason, actorUserID string, today time.Time) ([]voidedSalePaymentModel, error) {
	day := domain.CalendarDay(today)
	out := make([]voidedSalePaymentModel, 0, len(rows))
	for _, p := range rows {
		if p.DepositID != nil {
			return nil, glerrors.New(glerrors.KindConflict, "this charge was paid through a deposit: cancel the deposit instead")
		}
		if domain.CalendarDay(p.PaidOn).Before(day) {
			return nil, glerrors.New(glerrors.KindConflict, "the sale was paid on an earlier day: record the refund as an expense of today")
		}
		out = append(out, voidedSalePaymentModel{
			ID: p.ID, TenantID: p.TenantID, ChargeID: p.ChargeID, AmountCents: p.AmountCents,
			Method: p.Method, CardType: p.CardType, Reference: p.Reference, IdempotencyKey: p.IdempotencyKey,
			ReceivedBy: p.ReceivedBy, PaidAt: p.PaidAt, PaidOn: p.PaidOn,
			Reason: reason, VoidedBy: nullableUUID(actorUserID),
		})
	}
	return out, nil
}

// ListVoidedSalePaymentsOn returns the voided sale payments taken on day.
func (r *Repository) ListVoidedSalePaymentsOn(ctx context.Context, day time.Time) ([]ports.VoidedSalePayment, error) {
	var out []ports.VoidedSalePayment
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		list, err := voidedOf(tx, "paid_on = ?", domain.CalendarDay(day).Format(dateLayout))
		out = list
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// voidedOf reads archive rows by one condition, with the description of the
// charge each one paid.
func voidedOf(tx *gorm.DB, where string, arg any) ([]ports.VoidedSalePayment, error) {
	var rows []voidedSalePaymentModel
	if err := tx.Where(where, arg).Order("paid_at ASC, id ASC").Limit(maxListLimit).Find(&rows).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading the voided sale payments", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	chargeIDs := make([]string, 0, len(rows))
	for _, m := range rows {
		chargeIDs = append(chargeIDs, m.ChargeID)
	}
	var described []struct {
		ID          string
		Description string
	}
	if err := tx.Model(&chargeModel{}).Select("id, description").Where("id IN ?", chargeIDs).
		Scan(&described).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "reading what the voided payments paid", err)
	}
	descriptionOf := make(map[string]string, len(described))
	for _, d := range described {
		descriptionOf[d.ID] = d.Description
	}
	out := make([]ports.VoidedSalePayment, 0, len(rows))
	for _, m := range rows {
		voidedBy := ""
		if m.VoidedBy != nil {
			voidedBy = *m.VoidedBy
		}
		out = append(out, ports.VoidedSalePayment{
			PaymentID: m.ID, ChargeID: m.ChargeID, Description: descriptionOf[m.ChargeID],
			AmountCents: m.AmountCents, Method: m.Method, PaidOn: domain.CalendarDay(m.PaidOn),
			Reason: m.Reason, VoidedBy: voidedBy, VoidedAt: m.VoidedAt,
		})
	}
	return out, nil
}
