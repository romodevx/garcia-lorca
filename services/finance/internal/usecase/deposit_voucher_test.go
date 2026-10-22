package usecase_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

const (
	voucherOne          = "b1000000-0000-0000-0000-000000000001"
	voucherTwo          = "b1000000-0000-0000-0000-000000000002"
	voucherOtherStudent = "stu-voucher-other"
)

var voucherUploader = domain.Receiver{UserID: "guardian-1", Name: "Rosa Pérez"}

// pendingVoucher stores a pending voucher for student in the fake.
func pendingVoucher(t *testing.T, repo *fakeVoucherRepo, id, student string) domain.PaymentVoucher {
	t.Helper()
	v, err := domain.NewPaymentVoucher(id, domain.VoucherDraft{StudentID: student, ClaimedAmountCents: 320000},
		domain.VoucherFile{Key: "vouchers/" + id + "/o", ContentType: "image/jpeg", SizeBytes: 10, SHA256: bytes.Repeat([]byte(id[:1]), 32)},
		voucherUploader, time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	repo.put(v)
	return v
}

func voucherFixture(t *testing.T) (*fixture, *fakeVoucherRepo) {
	t.Helper()
	f := depositFixture(t)
	vouchers := newFakeVoucherRepo()
	f.svc.WithPaymentVouchers(vouchers)
	return f, vouchers
}

func lastDeposit(t *testing.T, f *fixture) domain.Deposit {
	t.Helper()
	if len(f.repo.deposits) == 0 {
		t.Fatal("no deposit reached the repository")
	}
	return f.repo.deposits[len(f.repo.deposits)-1]
}

// A. Paying by voucher derives the idempotency key from the voucher, so the
// same voucher applied twice is the same payment; the method is a transfer
// unless the office says otherwise.
func TestRegisterDepositByVoucher(t *testing.T) {
	f, vouchers := voucherFixture(t)
	v := pendingVoucher(t, vouchers, voucherOne, testStudent)
	applied, _ := v.Apply("dep-x", domain.Receiver{}, f.clock)
	f.repo.linkedVoucher = &applied

	got, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 320000, PaidAt: f.clock, VoucherID: voucherOne,
	})
	if err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}
	d := lastDeposit(t, f)
	if d.IdempotencyKey() != "voucher:"+voucherOne || d.Method() != domain.MethodTransfer {
		t.Fatalf("key %q method %q", d.IdempotencyKey(), d.Method())
	}
	if c := d.VoucherChoice(); c.VoucherID != voucherOne || len(c.Declined) != 0 {
		t.Fatalf("choice = %+v", c)
	}
	if got.LinkedVoucher == nil || got.LinkedVoucher.ID() != voucherOne {
		t.Fatalf("result names its voucher: %+v", got.LinkedVoucher)
	}

	// The office may say it was cash after all; the key is still the voucher's.
	cash, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 1000, Method: domain.MethodCash, PaidAt: f.clock,
		VoucherID: voucherTwoFor(t, vouchers),
	})
	if err != nil || lastDeposit(t, f).Method() != domain.MethodCash {
		t.Fatalf("explicit method: %v %+v", err, cash)
	}
}

func voucherTwoFor(t *testing.T, vouchers *fakeVoucherRepo) string {
	t.Helper()
	return pendingVoucher(t, vouchers, voucherTwo, testStudent).ID()
}

// A released voucher's next application has a key of its own.
func TestRegisterDepositByReleasedVoucher(t *testing.T) {
	f, vouchers := voucherFixture(t)
	v := pendingVoucher(t, vouchers, voucherOne, testStudent)
	applied, _ := v.Apply("dep-x", domain.Receiver{}, f.clock)
	released, _ := applied.Release(f.clock)
	vouchers.put(released)
	if _, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 320000, PaidAt: f.clock, VoucherID: voucherOne,
	}); err != nil {
		t.Fatal(err)
	}
	if key := lastDeposit(t, f).IdempotencyKey(); key != "voucher:"+voucherOne+":1" {
		t.Fatalf("key = %q", key)
	}
}

func TestRegisterDepositByVoucherRefusals(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, vouchers *fakeVoucherRepo)
		in      usecase.DepositInput
		wantErr error
		kind    glerrors.Kind
	}{
		{
			name:  "another key than the voucher's",
			setup: func(t *testing.T, r *fakeVoucherRepo) { pendingVoucher(t, r, voucherOne, testStudent) },
			in:    usecase.DepositInput{VoucherID: voucherOne, IdempotencyKey: "attempt-1"},
			kind:  glerrors.KindInvalid,
		},
		{
			name:    "a voucher that does not exist",
			setup:   func(*testing.T, *fakeVoucherRepo) {},
			in:      usecase.DepositInput{VoucherID: voucherOne},
			wantErr: domain.ErrVoucherNotOwned,
		},
		{
			name:    "a malformed voucher id",
			setup:   func(*testing.T, *fakeVoucherRepo) {},
			in:      usecase.DepositInput{VoucherID: "../etc"},
			wantErr: domain.ErrVoucherNotOwned,
		},
		{
			name:    "another pupil's voucher",
			setup:   func(t *testing.T, r *fakeVoucherRepo) { pendingVoucher(t, r, voucherOne, voucherOtherStudent) },
			in:      usecase.DepositInput{VoucherID: voucherOne},
			wantErr: domain.ErrVoucherNotOwned,
		},
		{
			name: "a rejected voucher",
			setup: func(t *testing.T, r *fakeVoucherRepo) {
				v := pendingVoucher(t, r, voucherOne, testStudent)
				rejected, _ := v.Reject("ilegible", domain.Receiver{}, time.Now())
				r.put(rejected)
			},
			in:      usecase.DepositInput{VoucherID: voucherOne},
			wantErr: domain.ErrVoucherRejected,
		},
		{
			name:  "declined together with a chosen voucher",
			setup: func(t *testing.T, r *fakeVoucherRepo) { pendingVoucher(t, r, voucherOne, testStudent) },
			in:    usecase.DepositInput{VoucherID: voucherOne, DeclinedVoucherIDs: []string{voucherTwo}},
			kind:  glerrors.KindInvalid,
		},
		{
			name:  "a malformed declined id",
			setup: func(*testing.T, *fakeVoucherRepo) {},
			in:    usecase.DepositInput{DeclinedVoucherIDs: []string{"nope"}},
			kind:  glerrors.KindInvalid,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, vouchers := voucherFixture(t)
			tc.setup(t, vouchers)
			in := tc.in
			in.StudentID, in.AmountCents, in.PaidAt = testStudent, 320000, f.clock
			_, err := f.svc.RegisterDeposit(context.Background(), in)
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil && glerrors.KindOf(err) != tc.kind {
				t.Fatalf("kind = %v (%v), want %v", glerrors.KindOf(err), err, tc.kind)
			}
			if len(f.repo.deposits) != 0 {
				t.Fatal("a refused voucher must not reach the deposit transaction")
			}
		})
	}
}

// B. Registered by hand, the office's declined vouchers ride into the
// transaction, which decides the link under the pupil's locks.
func TestRegisterDepositByHandCarriesTheDeclinedVouchers(t *testing.T) {
	f, _ := voucherFixture(t)
	if _, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 320000, Method: domain.MethodCash, PaidAt: f.clock,
		IdempotencyKey: "attempt-9", DeclinedVoucherIDs: []string{voucherTwo},
	}); err != nil {
		t.Fatal(err)
	}
	d := lastDeposit(t, f)
	if c := d.VoucherChoice(); c.VoucherID != "" || len(c.Declined) != 1 || c.Declined[0] != voucherTwo {
		t.Fatalf("choice = %+v", c)
	}
	if d.IdempotencyKey() != "attempt-9" {
		t.Fatalf("a payment by hand keeps the counter's key: %q", d.IdempotencyKey())
	}
}

// A service with no voucher store cannot be paid by voucher.
func TestRegisterDepositByVoucherWithoutTheModule(t *testing.T) {
	f := depositFixture(t)
	_, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 320000, PaidAt: f.clock, VoucherID: voucherOne,
	})
	if glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Fatalf("err = %v", err)
	}
}
