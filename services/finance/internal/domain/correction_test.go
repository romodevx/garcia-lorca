package domain_test

import (
	"errors"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

// T9: a corrected deposit is kept, marked superseded by its correction.
func TestSupersedingKeepsTheDepositAndNamesItsCorrection(t *testing.T) {
	at := time.Date(2026, time.October, 11, 9, 0, 0, 0, time.UTC)

	got, err := aDeposit(t).Supersede("dep-2", at)
	if err != nil {
		t.Fatalf("Supersede: %v", err)
	}
	if !got.Superseded() || got.SupersededBy() != "dep-2" || !got.SupersededAt().Equal(at) {
		t.Errorf("superseded = %v by %q at %v", got.Superseded(), got.SupersededBy(), got.SupersededAt())
	}
	if got.Counts() {
		t.Error("a superseded deposit must count for nothing")
	}
	if got.Voided() {
		t.Error("superseded is its own state, not a cancellation")
	}
	if got.Amount().Int64() != 670000 {
		t.Error("superseding must not touch the money as it was recorded")
	}
}

func TestADepositCountsUntilVoidedOrSuperseded(t *testing.T) {
	d := aDeposit(t)
	if !d.Counts() {
		t.Fatal("a live deposit counts")
	}
	voided, err := d.Void("doble captura", time.Now())
	if err != nil {
		t.Fatalf("Void: %v", err)
	}
	if voided.Counts() {
		t.Error("a voided deposit counts for nothing")
	}
}

func TestSupersedeIsRefusedWhenItCannotHappen(t *testing.T) {
	at := time.Date(2026, time.October, 11, 0, 0, 0, 0, time.UTC)
	voided, err := aDeposit(t).Void("doble captura", at)
	if err != nil {
		t.Fatal(err)
	}
	superseded, err := aDeposit(t).Supersede("dep-2", at)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		d    domain.Deposit
		by   string
		at   time.Time
		kind glerrors.Kind
	}{
		{"a cancelled deposit", voided, "dep-2", at, glerrors.KindConflict},
		{"an already corrected deposit", superseded, "dep-3", at, glerrors.KindConflict},
		{"no successor", aDeposit(t), " ", at, glerrors.KindInvalid},
		{"its own successor", aDeposit(t), "dep-1", at, glerrors.KindInvalid},
		{"no date", aDeposit(t), "dep-2", time.Time{}, glerrors.KindInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.d.Supersede(tc.by, tc.at)
			if glerrors.KindOf(err) != tc.kind {
				t.Errorf("err = %v, want kind %v", err, tc.kind)
			}
		})
	}
}

// Voiding a deposit that was corrected would cancel money the correction
// already moved to its successor.
func TestASupersededDepositCannotBeVoided(t *testing.T) {
	superseded, err := aDeposit(t).Supersede("dep-2", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := superseded.Void("x", time.Now()); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("err = %v, want conflict", err)
	}
}

func TestACorrectionNamesTheDepositItCorrects(t *testing.T) {
	d := aDeposit(t).WithCorrects("dep-0", "P-2026-000123")
	if d.Corrects() != "dep-0" || d.CorrectsFolio() != "P-2026-000123" {
		t.Errorf("corrects = %q (%q)", d.Corrects(), d.CorrectsFolio())
	}
	if !d.Counts() {
		t.Error("the correction is live money")
	}
}

func aCorrection(t *testing.T, reason string) (domain.DepositCorrection, error) {
	t.Helper()
	before := domain.DepositSnapshot{AmountCents: 50000, PaidAt: day(2026, time.October, 10), Folio: "P-2026-000001"}
	after := domain.DepositSnapshot{AmountCents: 100000, PaidAt: day(2026, time.October, 10), Folio: "P-2026-000002"}
	return domain.NewDepositCorrection("cor-1", "t1", "st-1", "dep-1", "dep-2", reason,
		before, after, domain.Receiver{UserID: "u-1", Name: "Directora"}, time.Date(2026, time.October, 11, 9, 0, 0, 0, time.UTC))
}

func TestACorrectionKeepsBeforeAfterWhoWhenAndWhy(t *testing.T) {
	c, err := aCorrection(t, "  Se capturó $500 y fueron $1,000  ")
	if err != nil {
		t.Fatalf("NewDepositCorrection: %v", err)
	}
	if c.Reason() != "Se capturó $500 y fueron $1,000" {
		t.Errorf("reason = %q, want it trimmed", c.Reason())
	}
	if c.Before().AmountCents != 50000 || c.After().AmountCents != 100000 {
		t.Errorf("before/after = %d/%d", c.Before().AmountCents, c.After().AmountCents)
	}
	if c.By().Name != "Directora" || c.OriginalID() != "dep-1" || c.CorrectedID() != "dep-2" {
		t.Errorf("correction = %+v", c)
	}
}

func TestACorrectionNeedsAReason(t *testing.T) {
	for _, reason := range []string{"", "   "} {
		if _, err := aCorrection(t, reason); glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Errorf("reason %q: err = %v, want invalid", reason, err)
		}
	}
}

func TestACorrectionReasonIsBounded(t *testing.T) {
	long := make([]rune, domain.MaxCorrectionReason+1)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := aCorrection(t, string(long)); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("err = %v, want invalid", err)
	}
	if err := domain.ValidateCorrectionReason(string(long[:domain.MaxCorrectionReason])); err != nil {
		t.Errorf("a reason at the bound is fine: %v", err)
	}
}

func TestSnapshotOfADepositIsWhatItWasAndWhatItPaid(t *testing.T) {
	cash, _ := domain.NewTender(domain.MethodCash, 30000, "", "")
	note, _ := domain.NewTender(domain.MethodCreditNote, 20000, "NC-7", "")
	d, err := domain.NewDeposit("dep-1", "t1", "st-1", 50000, domain.MethodCash, "", day(2026, time.October, 10), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if d, err = d.WithTenders([]domain.Tender{cash, note}); err != nil {
		t.Fatal(err)
	}
	p1, _ := domain.NewPayment("p1", "t1", "ch-sep", 20000, domain.MethodCreditNote, "NC-7", d.PaidAt(), "")
	p2, _ := domain.NewPayment("p2", "t1", "ch-sep", 10000, domain.MethodCash, "", d.PaidAt(), "")
	p3, _ := domain.NewPayment("p3", "t1", "ch-oct", 20000, domain.MethodCash, "", d.PaidAt(), "")

	s := domain.SnapshotOf(d.WithFolio("P-2026-000009"), []domain.Payment{p1, p2, p3})

	if s.AmountCents != 50000 || s.Folio != "P-2026-000009" || !s.PaidAt.Equal(d.PaidAt()) {
		t.Errorf("snapshot = %+v", s)
	}
	if len(s.Tenders) != 2 || s.Tenders[1].Method != domain.MethodCreditNote || s.Tenders[1].Reference != "NC-7" {
		t.Errorf("tenders = %+v", s.Tenders)
	}
	// One line per charge, in the order paid: the receipt's concepts.
	if len(s.Lines) != 2 || s.Lines[0].ChargeID != "ch-sep" || s.Lines[0].AmountCents != 30000 || s.Lines[1].AmountCents != 20000 {
		t.Errorf("lines = %+v", s.Lines)
	}
}

func TestCorrectionReasonErrorIsTheDomainKind(t *testing.T) {
	err := domain.ValidateCorrectionReason("")
	var e *glerrors.Error
	if !errors.As(err, &e) {
		t.Fatalf("err = %T, want *errors.Error", err)
	}
}
