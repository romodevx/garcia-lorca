package grpcadapter

import (
	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

// toProtoPaymentVoucher is a voucher on the wire (ADR 0040): never its bytes,
// nor where they live.
func toProtoPaymentVoucher(v domain.PaymentVoucher) *financev1.PaymentVoucher {
	out := &financev1.PaymentVoucher{
		Id:              v.ID(),
		StudentId:       v.StudentID(),
		Status:          v.Status(),
		FileContentType: v.File().ContentType,
		FileSizeBytes:   int64(v.File().SizeBytes),
		UploadedBy:      v.UploadedBy().UserID,
		UploadedByName:  v.UploadedBy().Name,
		DepositId:       v.DepositID(),
		DepositFolio:    v.DepositFolio(),
		DecidedBy:       v.DecidedBy().UserID,
		DecidedByName:   v.DecidedBy().Name,
		DecidedAt:       optionalTimestamp(v.DecidedAt()),
		RejectionReason: v.RejectionReason(),
		CreatedAt:       optionalTimestamp(v.CreatedAt()),
	}
	if amount, ok := v.ClaimedAmount(); ok {
		out.ClaimedAmountCents = amount.Int64()
	}
	if day, ok := v.ClaimedPaidOn(); ok {
		out.ClaimedPaidOn = day.Format(dayLayout)
	}
	return out
}
