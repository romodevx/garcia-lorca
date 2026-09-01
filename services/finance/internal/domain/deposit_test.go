package domain_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

func aDeposit(t *testing.T) domain.Deposit {
	t.Helper()
	d, err := domain.NewDeposit("dep-1", "t1", "st-1", 670000, domain.MethodCash, "", day(2026, time.October, 10), "key-1", "cal-abc")
	if err != nil {
		t.Fatalf("NewDeposit: %v", err)
	}
	return d
}

// The money as it arrived: one amount, one date, one receipt.
func TestNewDeposit(t *testing.T) {
	d := aDeposit(t)

	if d.Amount().Int64() != 670000 || d.StudentID() != "st-1" {
		t.Errorf("deposit wrong: %+v", d)
	}
	if d.Voided() {
		t.Error("a new deposit is not voided")
	}
	if d.CalendarVersion() != "cal-abc" {
		t.Errorf("calendar version = %q, want cal-abc", d.CalendarVersion())
	}
}

func TestDepositRejectsMoneyThatIsNotMoney(t *testing.T) {
	cases := map[string]int64{
		"zero":     0,
		"negative": -100,
	}
	for name, amount := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := domain.NewDeposit("dep-1", "t1", "st-1", amount, domain.MethodCash, "", day(2026, time.October, 10), "", "")
			if glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("amount %d: want KindInvalid, got %v", amount, err)
			}
		})
	}
}

// A cashier who keys the wrong amount has to be able to undo it, and a school
// that deletes money rows cannot explain its own ledger. Voiding is an entry.
func TestVoidingKeepsTheDepositAndItsReason(t *testing.T) {
	d := aDeposit(t)
	when := day(2026, time.October, 11)

	voided, err := d.Void("importe capturado mal", when)
	if err != nil {
		t.Fatalf("Void: %v", err)
	}
	if !voided.Voided() || voided.VoidReason() != "importe capturado mal" {
		t.Errorf("void did not take: %+v", voided)
	}
	if !voided.VoidedAt().Equal(when) {
		t.Errorf("voided at %v, want %v", voided.VoidedAt(), when)
	}
	if voided.Amount().Int64() != d.Amount().Int64() {
		t.Error("voiding changed the amount — the entry must record what happened, not erase it")
	}
	if d.Voided() {
		t.Error("Void mutated the original: a deposit is a value, and the ledger keeps both")
	}
}

// Without a reason, a voided deposit is an unexplained hole in the day's takings.
func TestVoidingNeedsAReason(t *testing.T) {
	d := aDeposit(t)

	if _, err := d.Void("   ", day(2026, time.October, 11)); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("blank reason: want KindInvalid, got %v", err)
	}
}

// Voiding twice is a second cashier acting on a stale screen. The first void
// stands; the second is refused rather than quietly overwriting who cancelled
// what, and when.
func TestADepositCannotBeVoidedTwice(t *testing.T) {
	d := aDeposit(t)
	voided, err := d.Void("error", day(2026, time.October, 11))
	if err != nil {
		t.Fatalf("Void: %v", err)
	}

	if _, err := voided.Void("otro error", day(2026, time.October, 12)); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("second void: want KindConflict, got %v", err)
	}
}

// The office reconciles against the terminal at the end of the month, and the
// terminal knows debit from credit. The ledger has to as well.
func TestADepositRecordsWhichKindOfCard(t *testing.T) {
	d, err := domain.NewDeposit("dep-1", "t1", "st-1", 320000, domain.MethodCard, "",
		day(2026, time.October, 10), "", "")
	if err != nil {
		t.Fatalf("NewDeposit: %v", err)
	}
	withCard, err := d.WithCardType(domain.CardDebit)
	if err != nil {
		t.Fatalf("WithCardType: %v", err)
	}
	if withCard.CardType() != domain.CardDebit {
		t.Errorf("card type = %q, want debit", withCard.CardType())
	}
}

// A card type on cash is a contradiction. Somebody would have to explain it to
// an auditor, and nobody could.
func TestCashHasNoCardType(t *testing.T) {
	d, err := domain.NewDeposit("dep-1", "t1", "st-1", 320000, domain.MethodCash, "",
		day(2026, time.October, 10), "", "")
	if err != nil {
		t.Fatalf("NewDeposit: %v", err)
	}
	if _, err := d.WithCardType(domain.CardCredit); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("card type on cash: want KindInvalid, got %v", err)
	}
}

func TestAnUnknownCardTypeIsRefused(t *testing.T) {
	d, err := domain.NewDeposit("dep-1", "t1", "st-1", 320000, domain.MethodCard, "",
		day(2026, time.October, 10), "", "")
	if err != nil {
		t.Fatalf("NewDeposit: %v", err)
	}
	if _, err := d.WithCardType("vale de despensa"); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("unknown card type: want KindInvalid, got %v", err)
	}
}
