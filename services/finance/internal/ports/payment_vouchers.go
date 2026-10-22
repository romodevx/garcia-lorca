package ports

import (
	"context"

	"github.com/garcia-lorca/services/finance/internal/domain"
)

// MaxVoucherListLimit caps one read of payment vouchers.
const MaxVoucherListLimit = 500

// VoucherFilter narrows a read of payment vouchers.
type VoucherFilter struct {
	// Status is pending, applied or rejected; empty is any.
	Status string
	// StudentIDs, when set, are the only pupils whose vouchers are read.
	// Empty reads the whole school — the use case decides who may ask for
	// that (ADR 0040); the repository only filters.
	StudentIDs []string
	// Limit caps the rows; 0 or more than MaxVoucherListLimit is the cap.
	Limit int
}

// PaymentVoucherRepository stores the vouchers families send from the app
// (migration 0032, ADR 0040). Linking a voucher to money happens inside the
// deposit transactions of Repository (RegisterDeposit, VoidDeposit,
// CorrectDeposit), never here.
type PaymentVoucherRepository interface {
	// CreatePaymentVoucher stores a pending voucher and its uploaded event, in
	// one transaction serialised per pupil. A live voucher of the same pupil
	// with the same file digest is returned instead (replayed = true) and
	// nothing is written; a pupil with MaxPendingVouchersPerStudent pending is
	// domain.ErrVoucherLimit.
	CreatePaymentVoucher(ctx context.Context, v domain.PaymentVoucher, uploaded domain.VoucherEvent) (domain.PaymentVoucher, bool, error)
	// LivePaymentVoucherByFile is the pupil's pending or applied voucher with
	// that digest, if any: the check that spares an upload for a retry.
	LivePaymentVoucherByFile(ctx context.Context, studentID string, sha256 []byte) (domain.PaymentVoucher, bool, error)
	// GetPaymentVoucher reads one voucher, or KindNotFound.
	GetPaymentVoucher(ctx context.Context, id string) (domain.PaymentVoucher, error)
	// ListPaymentVouchers reads vouchers oldest first.
	ListPaymentVouchers(ctx context.Context, f VoucherFilter) ([]domain.PaymentVoucher, error)
	// UpdatePaymentVoucher runs change on the voucher under its row lock and
	// stores what it returns with its event. An event with no Kind means
	// nothing changed: nothing is written and the current voucher returns.
	UpdatePaymentVoucher(ctx context.Context, id string,
		change func(cur domain.PaymentVoucher) (domain.PaymentVoucher, domain.VoucherEvent, error)) (domain.PaymentVoucher, error)
	// ListPaymentVoucherEvents reads a voucher's history, oldest first.
	ListPaymentVoucherEvents(ctx context.Context, id string) ([]domain.VoucherEvent, error)
}
