package grpcadapter

import (
	"bytes"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/timestamppb"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

// ADR 0040 on the wire, both directions: what the counter sends about
// vouchers reaches the use case, and every field of a voucher reaches the
// screen (a field added to one side only has shipped twice in finance).
func TestRegisterDepositVoucherFieldsReachTheUseCase(t *testing.T) {
	in := depositInputFrom(&financev1.RegisterDepositRequest{
		StudentId: "st-1", AmountCents: 1000, Method: domain.MethodTransfer,
		VoucherId:          "v-1",
		DeclinedVoucherIds: []string{"v-2", "v-3"},
		ReceivedBy:         "u-1", ReceivedByName: "Caja",
		PaidAt: timestamppb.New(time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)),
	})
	assert.Equal(t, "v-1", in.VoucherID)
	assert.Equal(t, []string{"v-2", "v-3"}, in.DeclinedVoucherIDs)
	assert.Equal(t, "st-1", in.StudentID)
	assert.Equal(t, domain.Receiver{UserID: "u-1", Name: "Caja"}, in.Receiver)
}

func TestPaymentVoucherCrossesTheWire(t *testing.T) {
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	created := time.Date(2026, 10, 20, 18, 0, 0, 0, time.UTC)
	decided := time.Date(2026, 10, 21, 9, 30, 0, 0, time.UTC)
	file := domain.VoucherFile{Key: "vouchers/v-1/o", ContentType: "image/png", SizeBytes: 4096, SHA256: bytes.Repeat([]byte{1}, 32)}
	applied := domain.RehydratePaymentVoucher("v-1",
		domain.VoucherDraft{StudentID: "st-1", ClaimedAmountCents: 320000, ClaimedPaidOn: day}, file,
		domain.Receiver{UserID: "g-1", Name: "Rosa"},
		domain.VoucherState{
			Status: domain.VoucherApplied, DepositID: "dep-1", DepositFolio: "P-2026-000042",
			DecidedBy: domain.Receiver{UserID: "u-1", Name: "Caja"}, DecidedAt: decided, CreatedAt: created,
		})
	got := toProtoPaymentVoucher(applied)
	assert.Equal(t, &financev1.PaymentVoucher{
		Id: "v-1", StudentId: "st-1", Status: "applied",
		ClaimedAmountCents: 320000, ClaimedPaidOn: "2026-10-20",
		FileContentType: "image/png", FileSizeBytes: 4096,
		UploadedBy: "g-1", UploadedByName: "Rosa",
		DepositId: "dep-1", DepositFolio: "P-2026-000042",
		DecidedBy: "u-1", DecidedByName: "Caja", DecidedAt: timestamppb.New(decided),
		CreatedAt: timestamppb.New(created),
	}, got)

	rejected := domain.RehydratePaymentVoucher("v-2", domain.VoucherDraft{StudentID: "st-1"}, file,
		domain.Receiver{UserID: "g-1"}, domain.VoucherState{
			Status: domain.VoucherRejected, RejectionReason: "ilegible", DecidedAt: decided, CreatedAt: created,
		})
	wire := toProtoPaymentVoucher(rejected)
	assert.Equal(t, "ilegible", wire.GetRejectionReason())
	assert.Zero(t, wire.GetClaimedAmountCents(), "no claim is 0")
	assert.Empty(t, wire.GetClaimedPaidOn(), "no claimed day is empty")

	pending := domain.RehydratePaymentVoucher("v-3", domain.VoucherDraft{StudentID: "st-1"}, file,
		domain.Receiver{UserID: "g-1"}, domain.VoucherState{Status: domain.VoucherPending, CreatedAt: created})
	assert.Nil(t, toProtoPaymentVoucher(pending).GetDecidedAt(), "pending has no decision instant")
}

// Each voucher refusal carries its machine code to the gateway.
func TestVoucherRefusalsCarryTheirCode(t *testing.T) {
	cases := []struct {
		err  error
		code connect.Code
		meta string
	}{
		{domain.VoucherAlreadyApplied("P-2026-000001"), connect.CodeAlreadyExists, domain.CodeVoucherAlreadyApplied},
		{domain.ErrVoucherRejected, connect.CodeAlreadyExists, domain.CodeVoucherRejected},
		{domain.ErrVoucherChoiceRequired, connect.CodeAlreadyExists, domain.CodeVoucherChoiceRequired},
		{domain.ErrVoucherLimit, connect.CodeAlreadyExists, domain.CodeVoucherLimit},
		{domain.ErrVoucherNotOwned, connect.CodeNotFound, ""},
	}
	for _, c := range cases {
		ce := connectErr(c.err)
		assert.Equal(t, c.code, ce.Code(), c.err.Error())
		assert.Equal(t, c.meta, ce.Meta().Get(errorCodeMeta), c.err.Error())
	}
	assert.Contains(t, connectErr(domain.VoucherAlreadyApplied("P-2026-000001")).Message(), "P-2026-000001")
}
