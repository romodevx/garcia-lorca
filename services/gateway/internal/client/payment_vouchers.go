package client

import (
	"context"

	connect "connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

// PaymentVoucher is a family's proof of a transfer for one pupil, as the
// office reads it (ADR 0040). Never money: the claimed figures only prefill.
type PaymentVoucher struct {
	ID                 string `json:"id"`
	StudentID          string `json:"studentId"`
	Status             string `json:"status"`
	ClaimedAmountCents int64  `json:"claimedAmountCents,omitempty"`
	ClaimedPaidOn      string `json:"claimedPaidOn,omitempty"`
	FileContentType    string `json:"fileContentType"`
	FileSizeBytes      int64  `json:"fileSizeBytes"`
	UploadedByName     string `json:"uploadedByName"`
	DepositID          string `json:"depositId,omitempty"`
	DepositFolio       string `json:"depositFolio,omitempty"`
	DecidedByName      string `json:"decidedByName,omitempty"`
	DecidedAt          string `json:"decidedAt,omitempty"`
	RejectionReason    string `json:"rejectionReason,omitempty"`
	CreatedAt          string `json:"createdAt"`
}

// VoucherChoice is what the counter says about vouchers on a deposit: this
// money IS VoucherID, or (by hand) it is NOT any of Declined.
type VoucherChoice struct {
	VoucherID string
	Declined  []string
}

// VoucherScope is whose vouchers a read may see: the family's children, or
// the school's queue. Finance refuses neither.
type VoucherScope struct {
	StudentIDs  []string
	SchoolQueue bool
}

// VoucherUpload is a voucher as the family sends it. ClaimedPaidOn is
// YYYY-MM-DD or empty; ClaimedAmountCents 0 is "did not say".
type VoucherUpload struct {
	StudentID          string
	Data               []byte
	ClaimedAmountCents int64
	ClaimedPaidOn      string
	By                 Receiver
}

// CreatePaymentVoucher stores a family's voucher; replayed says the same file
// was already sent for the pupil.
func (c *FinanceClient) CreatePaymentVoucher(ctx context.Context, in VoucherUpload) (PaymentVoucher, bool, error) {
	res, err := c.rpc.CreatePaymentVoucher(ctx, connect.NewRequest(toCreatePaymentVoucherRequest(in)))
	if err != nil {
		return PaymentVoucher{}, false, err
	}
	return toPaymentVoucher(res.Msg.GetVoucher()), res.Msg.GetReplayed(), nil
}

// ListPaymentVouchers reads vouchers within scope, oldest first.
func (c *FinanceClient) ListPaymentVouchers(ctx context.Context, scope VoucherScope, status string, limit int) ([]PaymentVoucher, error) {
	res, err := c.rpc.ListPaymentVouchers(ctx, connect.NewRequest(&financev1.ListPaymentVouchersRequest{
		Scope: toProtoVoucherScope(scope), Status: status, Limit: int32(limit), //nolint:gosec // bounded to 500 at the edge
	}))
	if err != nil {
		return nil, err
	}
	out := make([]PaymentVoucher, 0, len(res.Msg.GetVouchers()))
	for _, v := range res.Msg.GetVouchers() {
		out = append(out, toPaymentVoucher(v))
	}
	return out, nil
}

// GetPaymentVoucherFile returns a voucher's bytes and the type they proved to be.
func (c *FinanceClient) GetPaymentVoucherFile(ctx context.Context, id string, scope VoucherScope) ([]byte, string, error) {
	res, err := c.rpc.GetPaymentVoucherFile(ctx, connect.NewRequest(&financev1.GetPaymentVoucherFileRequest{
		Id: id, Scope: toProtoVoucherScope(scope),
	}))
	if err != nil {
		return nil, "", err
	}
	return res.Msg.GetData(), res.Msg.GetContentType(), nil
}

// RejectPaymentVoucher refuses a pending voucher; replayed says it already was.
func (c *FinanceClient) RejectPaymentVoucher(ctx context.Context, id, reason string, by Receiver) (PaymentVoucher, bool, error) {
	res, err := c.rpc.RejectPaymentVoucher(ctx, connect.NewRequest(&financev1.RejectPaymentVoucherRequest{
		Id: id, Reason: reason, ActorId: by.UserID, ActorName: by.Name,
	}))
	if err != nil {
		return PaymentVoucher{}, false, err
	}
	return toPaymentVoucher(res.Msg.GetVoucher()), res.Msg.GetReplayed(), nil
}

func toCreatePaymentVoucherRequest(in VoucherUpload) *financev1.CreatePaymentVoucherRequest {
	return &financev1.CreatePaymentVoucherRequest{
		StudentId: in.StudentID, Data: in.Data, ClaimedAmountCents: in.ClaimedAmountCents,
		ClaimedPaidOn: in.ClaimedPaidOn, UploaderId: in.By.UserID, UploaderName: in.By.Name,
	}
}

func toProtoVoucherScope(s VoucherScope) *financev1.VoucherScope {
	return &financev1.VoucherScope{StudentIds: s.StudentIDs, SchoolQueue: s.SchoolQueue}
}

func toPaymentVoucher(v *financev1.PaymentVoucher) PaymentVoucher {
	return PaymentVoucher{
		ID: v.GetId(), StudentID: v.GetStudentId(), Status: v.GetStatus(),
		ClaimedAmountCents: v.GetClaimedAmountCents(), ClaimedPaidOn: v.GetClaimedPaidOn(),
		FileContentType: v.GetFileContentType(), FileSizeBytes: v.GetFileSizeBytes(),
		UploadedByName: v.GetUploadedByName(), DepositID: v.GetDepositId(), DepositFolio: v.GetDepositFolio(),
		DecidedByName: v.GetDecidedByName(), DecidedAt: rfc3339(v.GetDecidedAt()),
		RejectionReason: v.GetRejectionReason(), CreatedAt: rfc3339(v.GetCreatedAt()),
	}
}
