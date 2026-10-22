package domain_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

var (
	voucherToday = time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	voucherAt    = time.Date(2026, 10, 21, 17, 30, 0, 0, time.UTC)
	guardian     = domain.Receiver{UserID: "guardian-1", Name: "Rosa Pérez"}
	cashier      = domain.Receiver{UserID: "cashier-1", Name: "Encargada"}
)

func voucherFile() domain.VoucherFile {
	return domain.VoucherFile{
		Key:         "vouchers/v-1/obj",
		ContentType: "image/jpeg",
		SizeBytes:   1024,
		SHA256:      bytes.Repeat([]byte{7}, 32),
	}
}

func newVoucher(t *testing.T) domain.PaymentVoucher {
	t.Helper()
	v, err := domain.NewPaymentVoucher("v-1", domain.VoucherDraft{
		StudentID:          "student-1",
		ClaimedAmountCents: 250_000,
		ClaimedPaidOn:      voucherToday,
	}, voucherFile(), guardian, voucherToday)
	if err != nil {
		t.Fatalf("NewPaymentVoucher: %v", err)
	}
	return v
}

func TestNewPaymentVoucher(t *testing.T) {
	tomorrow := voucherToday.AddDate(0, 0, 1)
	tests := []struct {
		name     string
		draft    domain.VoucherDraft
		file     domain.VoucherFile
		by       domain.Receiver
		wantKind glerrors.Kind
	}{
		{"claims nothing", domain.VoucherDraft{StudentID: "s"}, voucherFile(), guardian, glerrors.KindUnknown},
		{"claims amount and day", domain.VoucherDraft{StudentID: "s", ClaimedAmountCents: 1, ClaimedPaidOn: voucherToday}, voucherFile(), guardian, glerrors.KindUnknown},
		{"no student", domain.VoucherDraft{}, voucherFile(), guardian, glerrors.KindInvalid},
		{"negative amount", domain.VoucherDraft{StudentID: "s", ClaimedAmountCents: -1}, voucherFile(), guardian, glerrors.KindInvalid},
		{"amount past the cap", domain.VoucherDraft{StudentID: "s", ClaimedAmountCents: int64(domain.MaxCents) + 1}, voucherFile(), guardian, glerrors.KindInvalid},
		{"paid tomorrow", domain.VoucherDraft{StudentID: "s", ClaimedPaidOn: tomorrow}, voucherFile(), guardian, glerrors.KindInvalid},
		{"no uploader", domain.VoucherDraft{StudentID: "s"}, voucherFile(), domain.Receiver{}, glerrors.KindInvalid},
		{"no file key", domain.VoucherDraft{StudentID: "s"}, domain.VoucherFile{ContentType: "image/png", SizeBytes: 1, SHA256: make([]byte, 32)}, guardian, glerrors.KindInvalid},
		{"short digest", domain.VoucherDraft{StudentID: "s"}, domain.VoucherFile{Key: "k", ContentType: "image/png", SizeBytes: 1, SHA256: make([]byte, 4)}, guardian, glerrors.KindInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v, err := domain.NewPaymentVoucher("v-1", tc.draft, tc.file, tc.by, voucherToday)
			if tc.wantKind != glerrors.KindUnknown {
				if glerrors.KindOf(err) != tc.wantKind {
					t.Fatalf("kind = %v (%v), want %v", glerrors.KindOf(err), err, tc.wantKind)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected: %v", err)
			}
			if v.Status() != domain.VoucherPending || v.DepositID() != "" || v.Releases() != 0 {
				t.Fatalf("a new voucher is pending and unlinked: %+v", v)
			}
		})
	}
}

// The paid day is a calendar day: an instant late on the 21st is the 21st.
func TestNewPaymentVoucherKeepsTheCalendarDay(t *testing.T) {
	v, err := domain.NewPaymentVoucher("v-1", domain.VoucherDraft{StudentID: "s", ClaimedPaidOn: voucherAt},
		voucherFile(), guardian, voucherToday)
	if err != nil {
		t.Fatal(err)
	}
	day, ok := v.ClaimedPaidOn()
	if !ok || !day.Equal(voucherToday) {
		t.Fatalf("claimed day = %v, %v", day, ok)
	}
	if _, ok := v.ClaimedAmount(); ok {
		t.Fatal("no amount was claimed")
	}
}

func TestPaymentVoucherTransitions(t *testing.T) {
	pending := newVoucher(t)
	applied, err := pending.Apply("dep-1", cashier, voucherAt)
	if err != nil {
		t.Fatalf("pending → applied: %v", err)
	}
	rejected, err := pending.Reject("  no es de este colegio  ", cashier, voucherAt)
	if err != nil {
		t.Fatalf("pending → rejected: %v", err)
	}

	tests := []struct {
		name string
		run  func() (domain.PaymentVoucher, error)
		want error
	}{
		{"applied → applied", func() (domain.PaymentVoucher, error) { return applied.Apply("dep-2", cashier, voucherAt) }, domain.ErrVoucherAlreadyApplied},
		{"rejected → applied", func() (domain.PaymentVoucher, error) { return rejected.Apply("dep-2", cashier, voucherAt) }, domain.ErrVoucherRejected},
		{"applied → rejected", func() (domain.PaymentVoucher, error) { return applied.Reject("motivo", cashier, voucherAt) }, domain.ErrVoucherAlreadyApplied},
		{"rejected → rejected", func() (domain.PaymentVoucher, error) { return rejected.Reject("motivo", cashier, voucherAt) }, domain.ErrVoucherRejected},
		{"release pending", func() (domain.PaymentVoucher, error) { return pending.Release(voucherAt) }, domain.ErrVoucherNotApplied},
		{"release rejected", func() (domain.PaymentVoucher, error) { return rejected.Release(voucherAt) }, domain.ErrVoucherNotApplied},
		{"move pending", func() (domain.PaymentVoucher, error) { return pending.MoveTo("dep-2", voucherAt) }, domain.ErrVoucherNotApplied},
		{"move rejected", func() (domain.PaymentVoucher, error) { return rejected.MoveTo("dep-2", voucherAt) }, domain.ErrVoucherNotApplied},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.run(); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}

	if applied.Status() != domain.VoucherApplied || applied.DepositID() != "dep-1" ||
		applied.DecidedBy() != cashier || !applied.DecidedAt().Equal(voucherAt) {
		t.Fatalf("applied = %+v", applied)
	}
	if rejected.Status() != domain.VoucherRejected || rejected.RejectionReason() != "no es de este colegio" {
		t.Fatalf("rejected = %+v", rejected)
	}
	if pending.Status() != domain.VoucherPending {
		t.Fatal("transitions return copies; the original is untouched")
	}
}

func TestPaymentVoucherApplyNeedsADeposit(t *testing.T) {
	if _, err := newVoucher(t).Apply(" ", cashier, voucherAt); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("err = %v", err)
	}
}

func TestPaymentVoucherRejectReasonBounds(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		ok     bool
	}{
		{"three letters", "abc", true},
		{"three accented letters", "ñáé", true},
		{"300 characters", strings.Repeat("é", 300), true},
		{"empty", "", false},
		{"two letters", " ab ", false},
		{"301 characters", strings.Repeat("a", 301), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newVoucher(t).Reject(tc.reason, cashier, voucherAt)
			if tc.ok != (err == nil) {
				t.Fatalf("err = %v, want ok=%v", err, tc.ok)
			}
			if !tc.ok && glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Fatalf("kind = %v", glerrors.KindOf(err))
			}
		})
	}
}

// A void hands the voucher back to the queue: pending again, unlinked, and
// counted, so its next application gets an idempotency key of its own.
func TestPaymentVoucherRelease(t *testing.T) {
	applied, err := newVoucher(t).Apply("dep-1", cashier, voucherAt)
	if err != nil {
		t.Fatal(err)
	}
	released, err := applied.Release(voucherAt)
	if err != nil {
		t.Fatal(err)
	}
	if released.Status() != domain.VoucherPending || released.DepositID() != "" ||
		released.DecidedBy() != (domain.Receiver{}) || !released.DecidedAt().IsZero() {
		t.Fatalf("released = %+v", released)
	}
	if released.Releases() != 1 {
		t.Fatalf("releases = %d, want 1", released.Releases())
	}
	if released.IdempotencyKey() == applied.IdempotencyKey() {
		t.Fatal("a released voucher's next application must not replay the voided deposit")
	}
	again, err := released.Apply("dep-2", cashier, voucherAt)
	if err != nil || again.DepositID() != "dep-2" {
		t.Fatalf("re-apply after release: %v, %+v", err, again)
	}
}

// A correction moves the voucher onto the deposit that replaces the original,
// so it always points at a live one.
func TestPaymentVoucherMoveTo(t *testing.T) {
	applied, err := newVoucher(t).Apply("dep-1", cashier, voucherAt)
	if err != nil {
		t.Fatal(err)
	}
	moved, err := applied.MoveTo("dep-2", voucherAt)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Status() != domain.VoucherApplied || moved.DepositID() != "dep-2" || moved.DecidedBy() != cashier {
		t.Fatalf("moved = %+v", moved)
	}
	for _, bad := range []string{"", "dep-1"} {
		if _, err := applied.MoveTo(bad, voucherAt); glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Fatalf("MoveTo(%q) = %v", bad, err)
		}
	}
}

func TestVoucherIdempotencyKey(t *testing.T) {
	tests := []struct {
		releases int
		want     string
	}{
		{0, "voucher:v-1"},
		{1, "voucher:v-1:1"},
		{4, "voucher:v-1:4"},
	}
	for _, tc := range tests {
		if got := domain.VoucherIdempotencyKey("v-1", tc.releases); got != tc.want {
			t.Fatalf("VoucherIdempotencyKey(v-1, %d) = %q, want %q", tc.releases, got, tc.want)
		}
	}
	if got := newVoucher(t).IdempotencyKey(); got != "voucher:v-1" {
		t.Fatalf("IdempotencyKey = %q", got)
	}
}

func TestVoucherErrorsCarryTheirCodes(t *testing.T) {
	tests := []struct {
		err  error
		kind glerrors.Kind
		code string
	}{
		{domain.ErrVoucherAlreadyApplied, glerrors.KindConflict, domain.CodeVoucherAlreadyApplied},
		{domain.VoucherAlreadyApplied("P-2026-000123"), glerrors.KindConflict, domain.CodeVoucherAlreadyApplied},
		{domain.ErrVoucherRejected, glerrors.KindConflict, domain.CodeVoucherRejected},
		{domain.ErrVoucherChoiceRequired, glerrors.KindConflict, domain.CodeVoucherChoiceRequired},
		{domain.ErrVoucherLimit, glerrors.KindConflict, domain.CodeVoucherLimit},
		{domain.ErrVoucherNotOwned, glerrors.KindNotFound, ""},
	}
	for _, tc := range tests {
		if glerrors.KindOf(tc.err) != tc.kind {
			t.Fatalf("%v: kind = %v, want %v", tc.err, glerrors.KindOf(tc.err), tc.kind)
		}
		if got := domain.VoucherCodeOf(tc.err); got != tc.code {
			t.Fatalf("%v: code = %q, want %q", tc.err, got, tc.code)
		}
	}
	withFolio := domain.VoucherAlreadyApplied("P-2026-000123")
	if !errors.Is(withFolio, domain.ErrVoucherAlreadyApplied) || !strings.Contains(glerrors.SafeMessage(withFolio), "P-2026-000123") {
		t.Fatalf("the folio rides the message and the sentinel still matches: %q", glerrors.SafeMessage(withFolio))
	}
}

// The office's word on vouchers rides the deposit into its transaction. The
// declined list is copied: a caller reusing its slice cannot change a deposit
// already built.
func TestDepositVoucherChoice(t *testing.T) {
	d, err := domain.NewDeposit("dep-1", "tenant", "student-1", 1000, domain.MethodTransfer, "", voucherAt, "k", "cal")
	if err != nil {
		t.Fatal(err)
	}
	if c := d.VoucherChoice(); c.VoucherID != "" || len(c.Declined) != 0 || c.Any() {
		t.Fatalf("a plain deposit says nothing about vouchers: %+v", c)
	}
	declined := []string{"v-2", "v-3"}
	with := d.WithVoucherChoice(domain.VoucherChoice{Declined: declined})
	declined[0] = "changed"
	if got := with.VoucherChoice().Declined; len(got) != 2 || got[0] != "v-2" {
		t.Fatalf("declined = %v", got)
	}
	if !with.VoucherChoice().Any() || d.VoucherChoice().Any() {
		t.Fatal("WithVoucherChoice returns a copy")
	}
	if !d.WithVoucherChoice(domain.VoucherChoice{VoucherID: "v-1"}).VoucherChoice().Any() {
		t.Fatal("a chosen voucher is a choice")
	}
}
