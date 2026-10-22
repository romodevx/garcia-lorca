package usecase

import (
	"context"
	"strings"

	"github.com/google/uuid"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// errVouchersUnavailable: this deployment takes no payment vouchers.
var errVouchersUnavailable = glerrors.New(glerrors.KindNotFound, "payment vouchers are not enabled")

// WithPaymentVouchers lets deposits be registered by payment voucher (ADR
// 0040). Without it, a deposit naming a voucher is refused as NotFound.
func (s *FinanceService) WithPaymentVouchers(repo ports.PaymentVoucherRepository) *FinanceService {
	s.vouchers = repo
	return s
}

// voucherChoice checks what the counter said about vouchers and returns the
// input as it is taken, with the choice for the deposit transaction.
//
// A payment BY voucher is refused before any lock when the voucher cannot be
// this pupil's money: missing or another pupil's is NotFound (an id is never
// confirmed), rejected is ErrVoucherRejected. Its key is the voucher's own, so
// the same voucher applied twice is one payment; an applied voucher still goes
// through, to be answered as the replay it is — or as VOUCHER_ALREADY_APPLIED
// when it was linked by hand — under the transaction's locks.
func (s *FinanceService) voucherChoice(ctx context.Context, in DepositInput) (DepositInput, domain.VoucherChoice, error) {
	if in.VoucherID == "" {
		// A voucher's key belongs to its application alone: a payment by hand
		// stored under it would be what that application later "replays".
		if strings.HasPrefix(in.IdempotencyKey, domain.VoucherKeyPrefix) {
			return DepositInput{}, domain.VoucherChoice{}, glerrors.New(glerrors.KindInvalid,
				"that idempotency key is reserved for payments by voucher")
		}
		declined, err := canonicalVoucherIDs(in.DeclinedVoucherIDs)
		if err != nil {
			return DepositInput{}, domain.VoucherChoice{}, err
		}
		return in, domain.VoucherChoice{Declined: declined}, nil
	}
	if len(in.DeclinedVoucherIDs) > 0 {
		return DepositInput{}, domain.VoucherChoice{}, glerrors.New(glerrors.KindInvalid,
			"declined vouchers apply only to a payment registered by hand")
	}
	v, err := s.ownVoucher(ctx, in.VoucherID, in.StudentID)
	if err != nil {
		return DepositInput{}, domain.VoucherChoice{}, err
	}
	if v.Status() == domain.VoucherRejected {
		return DepositInput{}, domain.VoucherChoice{}, domain.ErrVoucherRejected
	}
	key := v.IdempotencyKey()
	if in.IdempotencyKey != "" && in.IdempotencyKey != key {
		return DepositInput{}, domain.VoucherChoice{}, glerrors.New(glerrors.KindInvalid,
			"a payment by voucher takes the voucher's own idempotency key")
	}
	in.IdempotencyKey = key
	if in.Method == "" && len(in.Tenders) == 0 {
		in.Method = domain.MethodTransfer
	}
	return in, domain.VoucherChoice{VoucherID: v.ID()}, nil
}

// ownVoucher reads a voucher that must belong to studentID; anything else is
// ErrVoucherNotOwned.
func (s *FinanceService) ownVoucher(ctx context.Context, id, studentID string) (domain.PaymentVoucher, error) {
	if s.vouchers == nil {
		return domain.PaymentVoucher{}, errVouchersUnavailable
	}
	canonical, err := uuid.Parse(id)
	if err != nil {
		return domain.PaymentVoucher{}, domain.ErrVoucherNotOwned
	}
	v, err := s.vouchers.GetPaymentVoucher(ctx, canonical.String())
	if glerrors.KindOf(err) == glerrors.KindNotFound {
		return domain.PaymentVoucher{}, domain.ErrVoucherNotOwned
	}
	if err != nil {
		return domain.PaymentVoucher{}, err
	}
	if v.StudentID() != studentID {
		return domain.PaymentVoucher{}, domain.ErrVoucherNotOwned
	}
	return v, nil
}

// canonicalVoucherIDs validates voucher ids and returns them in canonical
// (lower-case) form, so a string compare under the lock means the same row.
func canonicalVoucherIDs(ids []string) ([]string, error) {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		u, err := uuid.Parse(id)
		if err != nil {
			return nil, glerrors.New(glerrors.KindInvalid, "a declined voucher id is not valid")
		}
		out = append(out, u.String())
	}
	return out, nil
}
