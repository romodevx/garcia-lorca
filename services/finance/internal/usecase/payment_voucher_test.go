package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// ADR 0040: the voucher a family sends, the office's queue and its refusal.
// 22:30 on the 20th in Morelia is already the 21st in UTC: "today" is the
// 20th, so a claimed 21st is the future.
var voucherNow = time.Date(2026, 10, 21, 4, 30, 0, 0, time.UTC)

const (
	vStudent  = "stu-v1"
	vSibling  = "stu-v2"
	vStranger = "stu-v3"
)

var (
	vGuardian = domain.Receiver{UserID: "guardian-1", Name: "Rosa Pérez"}
	vCashier  = domain.Receiver{UserID: "cashier-1", Name: "Encargada"}
	vPDF      = []byte("%PDF-1.7\ntransferencia")
)

type voucherSvcFixture struct {
	svc   *usecase.PaymentVoucherService
	repo  *fakeVoucherRepo
	store *fakeReceiptStore
	ids   int
}

func newVoucherSvcFixture(t *testing.T) *voucherSvcFixture {
	t.Helper()
	f := &voucherSvcFixture{repo: newFakeVoucherRepo(), store: newFakeReceiptStore()}
	students := &fakeStudents{byID: map[string]ports.StudentRef{
		vStudent: {ID: vStudent}, vSibling: {ID: vSibling}, vStranger: {ID: vStranger},
	}}
	f.svc = usecase.NewPaymentVoucherService(f.repo, students, f.store, func() string {
		f.ids++
		return fmt.Sprintf("b2000000-0000-0000-0000-%012d", f.ids)
	}, usecase.NewSchoolClock(func() time.Time { return voucherNow }, schoolZone))
	return f
}

func (f *voucherSvcFixture) upload(t *testing.T, student string, data []byte) domain.PaymentVoucher {
	t.Helper()
	v, _, err := f.svc.Upload(context.Background(), usecase.VoucherUpload{StudentID: student, Data: data, By: vGuardian})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	return v
}

func TestUploadPaymentVoucher(t *testing.T) {
	f := newVoucherSvcFixture(t)
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	v, replayed, err := f.svc.Upload(context.Background(), usecase.VoucherUpload{
		StudentID: vStudent, Data: vPDF, ClaimedAmountCents: 320000, ClaimedPaidOn: day, By: vGuardian,
	})
	if err != nil || replayed {
		t.Fatalf("Upload: replayed=%v err=%v", replayed, err)
	}
	amount, _ := v.ClaimedAmount()
	paidOn, _ := v.ClaimedPaidOn()
	if v.StudentID() != vStudent || v.Status() != domain.VoucherPending || amount.Int64() != 320000 ||
		!paidOn.Equal(day) || v.UploadedBy() != vGuardian || v.File().ContentType != "application/pdf" {
		t.Fatalf("voucher = %+v", v)
	}
	if !strings.HasPrefix(v.File().Key, "vouchers/"+v.ID()+"/") {
		t.Fatalf("key %q is not under vouchers/<id>/", v.File().Key)
	}
	if string(f.store.objects[v.File().Key]) != string(vPDF) {
		t.Fatal("the file went to the store under its key")
	}
	events, _ := f.repo.ListPaymentVoucherEvents(context.Background(), v.ID())
	if len(events) != 1 || events[0].Kind != domain.VoucherEventUploaded || events[0].By != vGuardian || !events[0].At.Equal(voucherNow) {
		t.Fatalf("events = %+v", events)
	}
}

// The same file again (a retry, a double tap) is the same voucher, and costs
// no second upload.
func TestUploadSameFileReplays(t *testing.T) {
	f := newVoucherSvcFixture(t)
	first := f.upload(t, vStudent, vPDF)
	objects := len(f.store.objects)
	again, replayed, err := f.svc.Upload(context.Background(), usecase.VoucherUpload{StudentID: vStudent, Data: vPDF, By: vGuardian})
	if err != nil || !replayed || again.ID() != first.ID() {
		t.Fatalf("replayed=%v id=%s err=%v", replayed, again.ID(), err)
	}
	if len(f.store.objects) != objects {
		t.Fatal("a replay must not upload the file again")
	}
}

func TestUploadRefusals(t *testing.T) {
	tomorrow := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC) // Morelia is still on the 20th
	tests := []struct {
		name string
		in   usecase.VoucherUpload
		kind glerrors.Kind
	}{
		{"not a jpeg, png or pdf", usecase.VoucherUpload{StudentID: vStudent, Data: []byte("<html>"), By: vGuardian}, glerrors.KindInvalid},
		{"empty", usecase.VoucherUpload{StudentID: vStudent, By: vGuardian}, glerrors.KindInvalid},
		{"too large", usecase.VoucherUpload{StudentID: vStudent, Data: append(append([]byte{}, vPDF...), make([]byte, domain.MaxReceiptBytes)...), By: vGuardian}, glerrors.KindInvalid},
		{"paid tomorrow (school day)", usecase.VoucherUpload{StudentID: vStudent, Data: vPDF, ClaimedPaidOn: tomorrow, By: vGuardian}, glerrors.KindInvalid},
		{"negative amount", usecase.VoucherUpload{StudentID: vStudent, Data: vPDF, ClaimedAmountCents: -5, By: vGuardian}, glerrors.KindInvalid},
		{"unknown pupil", usecase.VoucherUpload{StudentID: "stu-nobody", Data: vPDF, By: vGuardian}, glerrors.KindNotFound},
		{"no uploader", usecase.VoucherUpload{StudentID: vStudent, Data: vPDF}, glerrors.KindInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newVoucherSvcFixture(t)
			_, _, err := f.svc.Upload(context.Background(), tc.in)
			if glerrors.KindOf(err) != tc.kind {
				t.Fatalf("kind = %v (%v), want %v", glerrors.KindOf(err), err, tc.kind)
			}
			if len(f.store.objects) != 0 {
				t.Fatal("nothing is uploaded for a refused voucher")
			}
		})
	}
}

// At the ceiling the fourth is refused, and its upload is deleted: no object
// is left that nothing references.
func TestUploadPastTheLimitDeletesTheFile(t *testing.T) {
	f := newVoucherSvcFixture(t)
	for i := range domain.MaxPendingVouchersPerStudent {
		f.upload(t, vStudent, []byte(fmt.Sprintf("%%PDF-1.7\n%d", i)))
	}
	_, _, err := f.svc.Upload(context.Background(), usecase.VoucherUpload{StudentID: vStudent, Data: []byte("%PDF-1.7\nfourth"), By: vGuardian})
	if !errors.Is(err, domain.ErrVoucherLimit) {
		t.Fatalf("err = %v", err)
	}
	if len(f.store.objects) != domain.MaxPendingVouchersPerStudent || len(f.store.deleted) != 1 {
		t.Fatalf("objects=%d deleted=%v", len(f.store.objects), f.store.deleted)
	}
	// A sibling has a ceiling of their own.
	f.upload(t, vSibling, []byte("%PDF-1.7\nfourth"))
}

func TestUploadWithoutAStore(t *testing.T) {
	svc := usecase.NewPaymentVoucherService(newFakeVoucherRepo(), &fakeStudents{byID: map[string]ports.StudentRef{vStudent: {ID: vStudent}}},
		nil, func() string { return "x" }, usecase.NewSchoolClock(func() time.Time { return voucherNow }, schoolZone))
	if _, _, err := svc.Upload(context.Background(), usecase.VoucherUpload{StudentID: vStudent, Data: vPDF, By: vGuardian}); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Fatalf("err = %v", err)
	}
}

// The scope is the service's to enforce (ADR 0010 condition 2): a family read
// names its children, and only the school's queue may read everybody's.
func TestListPaymentVouchersScope(t *testing.T) {
	f := newVoucherSvcFixture(t)
	mine := f.upload(t, vStudent, vPDF)
	f.upload(t, vStranger, []byte("%PDF-1.7\nstranger"))

	got, err := f.svc.List(context.Background(), usecase.VoucherScope{StudentIDs: []string{vStudent, vSibling}}, "", 0)
	if err != nil || len(got) != 1 || got[0].ID() != mine.ID() {
		t.Fatalf("family read = %+v, %v", got, err)
	}
	queue, err := f.svc.List(context.Background(), usecase.VoucherScope{School: true}, domain.VoucherPending, 0)
	if err != nil || len(queue) != 2 {
		t.Fatalf("school queue = %d, %v", len(queue), err)
	}
	for name, scope := range map[string]usecase.VoucherScope{
		"no scope at all":           {},
		"an empty list of children": {StudentIDs: []string{}},
	} {
		if _, err := f.svc.List(context.Background(), scope, "", 0); glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Fatalf("%s: want KindInvalid, got %v", name, err)
		}
	}
	if _, err := f.svc.List(context.Background(), usecase.VoucherScope{School: true}, "lost", 0); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("unknown status: %v", err)
	}
	if _, err := f.svc.List(context.Background(), usecase.VoucherScope{School: true}, "", ports.MaxVoucherListLimit+1); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("limit past the cap: %v", err)
	}
}

func TestPaymentVoucherFileScope(t *testing.T) {
	f := newVoucherSvcFixture(t)
	v := f.upload(t, vStudent, vPDF)

	file, err := f.svc.File(context.Background(), v.ID(), usecase.VoucherScope{School: true})
	if err != nil || file.ContentType() != "application/pdf" || string(file.Data()) != string(vPDF) {
		t.Fatalf("school read: %q %v", file.ContentType(), err)
	}
	if _, err := f.svc.File(context.Background(), v.ID(), usecase.VoucherScope{StudentIDs: []string{vStudent}}); err != nil {
		t.Fatalf("own child's file: %v", err)
	}
	if _, err := f.svc.File(context.Background(), v.ID(), usecase.VoucherScope{StudentIDs: []string{vSibling}}); !errors.Is(err, domain.ErrVoucherNotOwned) {
		t.Fatalf("another family's voucher: want ErrVoucherNotOwned, got %v", err)
	}
	if _, err := f.svc.File(context.Background(), v.ID(), usecase.VoucherScope{}); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("no scope: %v", err)
	}
	if _, err := f.svc.File(context.Background(), "b2000000-0000-0000-0000-999999999999", usecase.VoucherScope{School: true}); !errors.Is(err, domain.ErrVoucherNotOwned) {
		t.Fatalf("missing voucher: %v", err)
	}
}

func TestRejectPaymentVoucher(t *testing.T) {
	f := newVoucherSvcFixture(t)
	v := f.upload(t, vStudent, vPDF)

	rejected, replayed, err := f.svc.Reject(context.Background(), v.ID(), "  la transferencia no se ve ", vCashier)
	if err != nil || replayed || rejected.Status() != domain.VoucherRejected || rejected.RejectionReason() != "la transferencia no se ve" ||
		rejected.DecidedBy() != vCashier || !rejected.DecidedAt().Equal(voucherNow) {
		t.Fatalf("rejected = %+v replayed=%v err=%v", rejected, replayed, err)
	}
	// Repeating the refusal is idempotent: same answer, no second event.
	again, replayed, err := f.svc.Reject(context.Background(), v.ID(), "otro motivo", vCashier)
	if err != nil || !replayed || again.RejectionReason() != "la transferencia no se ve" {
		t.Fatalf("again = %+v replayed=%v err=%v", again, replayed, err)
	}
	if events, _ := f.repo.ListPaymentVoucherEvents(context.Background(), v.ID()); len(events) != 2 {
		t.Fatalf("history = %+v", events)
	}

	if _, _, err := f.svc.Reject(context.Background(), f.upload(t, vSibling, vPDF).ID(), "no", vCashier); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("a two-letter reason: %v", err)
	}
	applied, _ := f.upload(t, vStranger, vPDF).Apply("dep-1", vCashier, voucherNow)
	f.repo.put(applied)
	if _, _, err := f.svc.Reject(context.Background(), applied.ID(), "ya no", vCashier); !errors.Is(err, domain.ErrVoucherAlreadyApplied) {
		t.Fatalf("rejecting an applied voucher: %v", err)
	}
	if _, _, err := f.svc.Reject(context.Background(), "not-a-uuid", "motivo", vCashier); !errors.Is(err, domain.ErrVoucherNotOwned) {
		t.Fatalf("malformed id: %v", err)
	}
}
