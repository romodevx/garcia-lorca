package usecase

import (
	"context"
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/uniforms/internal/domain"
	"github.com/garcia-lorca/services/uniforms/internal/ports"
)

// maxReturnReasonLen bounds the reason a return keeps (migration 0011).
const maxReturnReasonLen = 200

// refundMethods are how money may go back on a later day: an expense takes no
// credit notes (finance/0024).
var refundMethods = map[string]bool{"cash": true, "card": true, "transfer": true}

// ReturnCounterSaleInput is a counter sale coming back whole (C2-015).
type ReturnCounterSaleInput struct {
	OrderID string
	Reason  string
	// DamagedLineIDs are written off instead of going back on sale.
	DamagedLineIDs []string
	// RefundMethod is how the money goes back on a later day; ignored the day
	// of the sale, when its payments are voided instead.
	RefundMethod string
	ActorUserID  string
	ActorName    string
}

// ReturnCounterSale takes a counter sale back whole (C2-015, owner 2026-10-19).
//
// The order of operations is the design:
//
//  1. decide everything, touching nothing (reason, the sale, its day, lines);
//  2. the day of the sale, void its payments in finance FIRST: if finance
//     refuses, the garments stay sold and nothing half-happened; the void is
//     idempotent, so a retry after step 3 failed is safe;
//  3. ONE transaction here: delivery and lines returned, garments back on the
//     shelf (or written off), the record kept;
//  4. another day, the refund is an expense of today — recorded LAST, because
//     the garments are already back; a failure leaves it pending, the delivery
//     flagged, and pressing "Devolver" again finishes it.
func (s *Service) ReturnCounterSale(ctx context.Context, in ReturnCounterSaleInput) (ports.OrderAggregate, error) {
	reason := strings.TrimSpace(in.Reason)
	if reason == "" || len([]rune(reason)) > maxReturnReasonLen {
		return ports.OrderAggregate{}, glerrors.New(glerrors.KindInvalid, "a return needs a reason of up to 200 characters")
	}
	agg, err := s.repo.GetOrder(ctx, in.OrderID, nil)
	if err != nil {
		return ports.OrderAggregate{}, err
	}
	if len(agg.Fulfillments) != 1 || agg.Order.Source() != domain.SourceCounter {
		return ports.OrderAggregate{}, glerrors.New(glerrors.KindConflict, "only a counter sale can be returned")
	}
	ful := agg.Fulfillments[0]
	if ful.Status() == domain.StatusReturned {
		return s.finishRefund(ctx, agg, ful)
	}
	returned, err := ful.ReturnAtCounter()
	if err != nil {
		return ports.OrderAggregate{}, err
	}
	cmd, err := s.returnCommand(agg, returned, reason, in)
	if err != nil {
		return ports.OrderAggregate{}, err
	}
	if cmd.Record.RefundMode == ports.RefundVoid {
		if err := s.finance.VoidSalePayments(ctx, ful.FinanceChargeID(), reason, in.ActorUserID); err != nil {
			return ports.OrderAggregate{}, err
		}
	}
	saved, err := s.repo.ReturnCounterSale(ctx, cmd)
	if err != nil {
		return ports.OrderAggregate{}, err
	}
	return s.finishRefund(ctx, saved, ful)
}

// returnCommand decides how the money goes back and which garments are
// written off, before anything moves.
func (s *Service) returnCommand(agg ports.OrderAggregate, returned domain.Fulfillment, reason string, in ReturnCounterSaleInput) (ports.ReturnCounterSaleCommand, error) {
	mode, method := s.refundModeOf(returned), ""
	if mode == ports.RefundExpense {
		method = strings.TrimSpace(in.RefundMethod)
		if !refundMethods[method] {
			return ports.ReturnCounterSaleCommand{}, glerrors.New(glerrors.KindInvalid,
				"the sale is of an earlier day: say how the money goes back (cash, card or transfer)")
		}
	}
	lines, damaged, units, err := returnedLines(agg, returned.ID(), in.DamagedLineIDs)
	if err != nil {
		return ports.ReturnCounterSaleCommand{}, err
	}
	status := ports.RefundDone
	if mode == ports.RefundExpense {
		status = ports.RefundPending
	}
	return ports.ReturnCounterSaleCommand{
		Fulfillment: returned, Lines: lines, Damaged: damaged, Folio: agg.Order.Folio(),
		Record: ports.CounterSaleReturn{
			FulfillmentID: returned.ID(), OrderID: agg.Order.ID(), Reason: reason,
			RefundMode: mode, RefundMethod: method, RefundCents: returned.AmountCents(), RefundStatus: status,
			DamagedUnits: units, ActorUserID: in.ActorUserID, ActorName: strings.TrimSpace(in.ActorName),
		},
	}, nil
}

// refundModeOf: no money reached finance (an anonymous sale, or one flagged
// before its charge existed) → none; the school's same day → void; else expense.
func (s *Service) refundModeOf(f domain.Fulfillment) string {
	if f.FinanceChargeID() == "" {
		return ports.RefundNone
	}
	if f.PaidAt().In(s.zone).Format(dayLayout) == s.now().In(s.zone).Format(dayLayout) {
		return ports.RefundVoid
	}
	return ports.RefundExpense
}

// dayLayout is a school day, YYYY-MM-DD.
const dayLayout = "2006-01-02"

// returnedLines turns the delivery's lines returned and checks every damaged
// one belongs to it.
func returnedLines(agg ports.OrderAggregate, fulfillmentID string, damagedIDs []string) ([]domain.OrderLine, map[string]bool, int, error) {
	lines := make([]domain.OrderLine, 0, len(agg.Lines))
	qtyOf := make(map[string]int, len(agg.Lines))
	for _, l := range agg.Lines {
		if l.FulfillmentID() != fulfillmentID {
			continue
		}
		back, err := l.Return()
		if err != nil {
			return nil, nil, 0, err
		}
		lines = append(lines, back)
		qtyOf[l.ID()] = l.Qty()
	}
	damaged := make(map[string]bool, len(damagedIDs))
	units := 0
	for _, raw := range damagedIDs {
		id := strings.TrimSpace(raw)
		qty, ours := qtyOf[id]
		if !ours {
			return nil, nil, 0, glerrors.New(glerrors.KindInvalid, "a damaged garment must be a line of this sale")
		}
		if !damaged[id] {
			damaged[id] = true
			units += qty
		}
	}
	return lines, damaged, units, nil
}

// finishRefund records a pending refund as today's expense. A failure is not
// the caller's error — the return happened — so it leaves the refund pending
// and the delivery flagged, and the answer says so.
func (s *Service) finishRefund(ctx context.Context, agg ports.OrderAggregate, ful domain.Fulfillment) (ports.OrderAggregate, error) {
	rec, ok := agg.Returns[ful.ID()]
	if !ok || rec.RefundStatus != ports.RefundPending {
		return agg, nil
	}
	expenseID, err := s.finance.RecordRefund(ctx, ports.RefundInput{
		AmountCents: rec.RefundCents, Method: rec.RefundMethod, SpentOn: s.now().In(s.zone).Format(dayLayout),
		Description: "Devolución " + agg.Order.Folio() + ": " + rec.Reason,
		ActorUserID: rec.ActorUserID, ActorName: rec.ActorName,
	})
	if err != nil {
		s.flag(ctx, ful.ID(), "la devolución se hizo pero el reembolso no quedó como egreso en Finanzas")
		return s.repo.GetOrder(ctx, agg.Order.ID(), nil)
	}
	if err := s.repo.SettleReturnRefund(ctx, ful.ID(), expenseID); err != nil {
		s.flag(ctx, ful.ID(), "el reembolso quedó como egreso pero no se ligó a la devolución")
	}
	return s.repo.GetOrder(ctx, agg.Order.ID(), nil)
}
