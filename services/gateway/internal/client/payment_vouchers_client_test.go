package client

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

// ADR 0040, both directions of the wire.
func TestPaymentVoucherCrossesTheWire(t *testing.T) {
	at := timestamppb.New(time.Date(2026, 10, 21, 15, 0, 0, 0, time.UTC))
	got := toPaymentVoucher(&financev1.PaymentVoucher{
		Id: "v-1", StudentId: "st-1", Status: "applied", ClaimedAmountCents: 320000, ClaimedPaidOn: "2026-10-20",
		FileContentType: "image/png", FileSizeBytes: 4096, UploadedBy: "g-1", UploadedByName: "Rosa",
		DepositId: "dep-1", DepositFolio: "P-2026-000042", DecidedBy: "u-1", DecidedByName: "Caja", DecidedAt: at,
		RejectionReason: "", CreatedAt: at,
	})
	want := PaymentVoucher{
		ID: "v-1", StudentID: "st-1", Status: "applied", ClaimedAmountCents: 320000, ClaimedPaidOn: "2026-10-20",
		FileContentType: "image/png", FileSizeBytes: 4096, UploadedByName: "Rosa",
		DepositID: "dep-1", DepositFolio: "P-2026-000042", DecidedByName: "Caja",
		DecidedAt: "2026-10-21T15:00:00Z", CreatedAt: "2026-10-21T15:00:00Z",
	}
	if got != want {
		t.Errorf("voucher = %+v\nwant      %+v", got, want)
	}
	if v := toPaymentVoucher(&financev1.PaymentVoucher{Id: "v-2", Status: "rejected", RejectionReason: "ilegible"}); v.RejectionReason != "ilegible" || v.DecidedAt != "" {
		t.Errorf("rejected = %+v", v)
	}
}

func TestPaymentVoucherRequestsCarryEveryField(t *testing.T) {
	c := toCreatePaymentVoucherRequest(VoucherUpload{
		StudentID: "st-1", Data: []byte("%PDF-"), ClaimedAmountCents: 100, ClaimedPaidOn: "2026-10-20",
		By: Receiver{UserID: "g-1", Name: "Rosa"},
	})
	if c.GetStudentId() != "st-1" || string(c.GetData()) != "%PDF-" || c.GetClaimedAmountCents() != 100 ||
		c.GetClaimedPaidOn() != "2026-10-20" || c.GetUploaderId() != "g-1" || c.GetUploaderName() != "Rosa" {
		t.Errorf("create = %+v", c)
	}
	s := toProtoVoucherScope(VoucherScope{StudentIDs: []string{"st-1"}, SchoolQueue: true})
	if len(s.GetStudentIds()) != 1 || !s.GetSchoolQueue() {
		t.Errorf("scope = %+v", s)
	}
	d := toRegisterDepositRequest(DepositRequest{
		StudentID: "st-1", AmountCents: 1000, Method: "transfer",
		Vouchers: VoucherChoice{VoucherID: "v-1", Declined: []string{"v-2"}},
	})
	if d.GetVoucherId() != "v-1" || len(d.GetDeclinedVoucherIds()) != 1 || d.GetDeclinedVoucherIds()[0] != "v-2" || d.GetStudentId() != "st-1" {
		t.Errorf("deposit = %+v", d)
	}
	res := toDepositResult(&financev1.RegisterDepositResponse{LinkedVoucher: &financev1.PaymentVoucher{Id: "v-1"}})
	if res.LinkedVoucher == nil || res.LinkedVoucher.ID != "v-1" {
		t.Errorf("linked = %+v", res.LinkedVoucher)
	}
	if toDepositResult(&financev1.RegisterDepositResponse{}).LinkedVoucher != nil {
		t.Error("no voucher stays nil")
	}
}
