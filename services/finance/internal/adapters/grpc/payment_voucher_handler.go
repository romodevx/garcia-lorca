package grpcadapter

import (
	"context"

	connect "connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// WithPaymentVouchers wires the payment vouchers (ADR 0040). Without it those
// RPCs answer Unimplemented; RegisterDeposit by voucher is the FinanceService's.
func (h *FinanceHandler) WithPaymentVouchers(svc *usecase.PaymentVoucherService) *FinanceHandler {
	h.vouchers = svc
	return h
}

var errVouchersUnwired = connect.NewError(connect.CodeUnimplemented, glerrors.New(glerrors.KindInternal, "payment vouchers are not enabled"))

// CreatePaymentVoucher stores a family's voucher.
func (h *FinanceHandler) CreatePaymentVoucher(ctx context.Context, req *connect.Request[financev1.CreatePaymentVoucherRequest]) (*connect.Response[financev1.CreatePaymentVoucherResponse], error) {
	if h.vouchers == nil {
		return nil, errVouchersUnwired
	}
	in, err := voucherUploadFrom(req.Msg)
	if err != nil {
		return nil, connectErr(err)
	}
	v, replayed, err := h.vouchers.Upload(ctx, in)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.CreatePaymentVoucherResponse{Voucher: toProtoPaymentVoucher(v), Replayed: replayed}), nil
}

// ListPaymentVouchers reads vouchers within the request's scope.
func (h *FinanceHandler) ListPaymentVouchers(ctx context.Context, req *connect.Request[financev1.ListPaymentVouchersRequest]) (*connect.Response[financev1.ListPaymentVouchersResponse], error) {
	if h.vouchers == nil {
		return nil, errVouchersUnwired
	}
	m := req.Msg
	vs, err := h.vouchers.List(ctx, voucherScopeFrom(m.GetScope()), m.GetStatus(), int(m.GetLimit()))
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*financev1.PaymentVoucher, 0, len(vs))
	for _, v := range vs {
		out = append(out, toProtoPaymentVoucher(v))
	}
	return connect.NewResponse(&financev1.ListPaymentVouchersResponse{Vouchers: out}), nil
}

// GetPaymentVoucherFile returns a voucher's bytes and type, within scope.
func (h *FinanceHandler) GetPaymentVoucherFile(ctx context.Context, req *connect.Request[financev1.GetPaymentVoucherFileRequest]) (*connect.Response[financev1.GetPaymentVoucherFileResponse], error) {
	if h.vouchers == nil {
		return nil, errVouchersUnwired
	}
	f, err := h.vouchers.File(ctx, req.Msg.GetId(), voucherScopeFrom(req.Msg.GetScope()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.GetPaymentVoucherFileResponse{Data: f.Data(), ContentType: f.ContentType()}), nil
}

// RejectPaymentVoucher refuses a pending voucher.
func (h *FinanceHandler) RejectPaymentVoucher(ctx context.Context, req *connect.Request[financev1.RejectPaymentVoucherRequest]) (*connect.Response[financev1.RejectPaymentVoucherResponse], error) {
	if h.vouchers == nil {
		return nil, errVouchersUnwired
	}
	m := req.Msg
	v, replayed, err := h.vouchers.Reject(ctx, m.GetId(), m.GetReason(), actorOf(m.GetActorId(), m.GetActorName()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.RejectPaymentVoucherResponse{Voucher: toProtoPaymentVoucher(v), Replayed: replayed}), nil
}

// voucherUploadFrom is the voucher the gateway forwarded, as the use case
// takes it. A claimed day that is not YYYY-MM-DD is InvalidArgument.
func voucherUploadFrom(m *financev1.CreatePaymentVoucherRequest) (usecase.VoucherUpload, error) {
	paidOn, err := parseOptionalDay(m.GetClaimedPaidOn())
	if err != nil {
		return usecase.VoucherUpload{}, err
	}
	return usecase.VoucherUpload{
		StudentID:          m.GetStudentId(),
		Data:               m.GetData(),
		ClaimedAmountCents: m.GetClaimedAmountCents(),
		ClaimedPaidOn:      paidOn,
		By:                 actorOf(m.GetUploaderId(), m.GetUploaderName()),
	}, nil
}

// voucherScopeFrom is the wire scope; an absent one is no scope at all, which
// the use case refuses.
func voucherScopeFrom(s *financev1.VoucherScope) usecase.VoucherScope {
	return usecase.VoucherScope{StudentIDs: s.GetStudentIds(), School: s.GetSchoolQueue()}
}

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
