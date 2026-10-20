package usecase

import (
	"context"
	"strings"
	"unicode/utf8"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// maxSaleVoidReasonLen bounds the reason a returned sale keeps (migration 0030).
const maxSaleVoidReasonLen = 500

// VoidSalePayments voids the payments of a uniforms counter sale that was
// returned on the day it was sold, and cancels its charge (C2-015). The money
// goes back to the family from today's drawer, so it leaves today's takings;
// the cash-up still lists it as cancelled, with the reason.
//
// "The day it was sold" is the school's day (ADR 0035). A sale of an earlier
// day is refused by the repository: its cash-up may be closed, and that
// refund is an expense of the day it is handed back.
func (s *FinanceService) VoidSalePayments(ctx context.Context, chargeID, reason, actorUserID string) ([]ports.VoidedSalePayment, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, glerrors.New(glerrors.KindInvalid, "a reason is required to void a sale")
	}
	if utf8.RuneCountInString(reason) > maxSaleVoidReasonLen {
		return nil, glerrors.New(glerrors.KindInvalid, "the void reason is too long")
	}
	if !uuidShape.MatchString(chargeID) {
		return nil, glerrors.New(glerrors.KindInvalid, "charge id must be an identifier")
	}
	return s.repo.VoidSalePayments(ctx, chargeID, reason, actorUserID, s.clock.Today())
}
