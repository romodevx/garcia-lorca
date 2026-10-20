package domain

import (
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// UniformsSaleRefPrefix is how a uniforms counter sale names its charge
// (uniforms:<fulfillment id>, ADR 0008).
const UniformsSaleRefPrefix = "uniforms:"

// SalePayment is what the void rule needs to know of one payment on a sale's
// charge (C2-015).
type SalePayment struct {
	// InDeposit: the payment is one slice of a deposit, not the sale's own.
	InDeposit bool
	// PaidOn is the school day it was taken (ADR 0035).
	PaidOn time.Time
}

// CheckSaleVoid decides whether a returned sale's payments may be voided
// (C2-015): only a uniforms counter sale's charge, only its own loose
// payments, and only on the school day they were taken. An earlier day's
// money is in a cash-up that may be closed — that refund is an expense of the
// day it is handed back, never a rewrite of the past.
func CheckSaleVoid(externalRef string, payments []SalePayment, today time.Time) error {
	if !strings.HasPrefix(externalRef, UniformsSaleRefPrefix) {
		return glerrors.New(glerrors.KindInvalid, "only a uniforms counter sale can be voided this way")
	}
	day := CalendarDay(today)
	for _, p := range payments {
		if p.InDeposit {
			return glerrors.New(glerrors.KindConflict, "this charge was paid through a deposit: cancel the deposit instead")
		}
		if CalendarDay(p.PaidOn).Before(day) {
			return glerrors.New(glerrors.KindConflict, "the sale was paid on an earlier day: record the refund as an expense of today")
		}
	}
	return nil
}
