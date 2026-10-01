package domain

import (
	"cmp"
	"slices"
	"time"
)

// MovementKind is what a line of the account statement records (T7 · A4).
//
// A closed vocabulary shared by every reader — the office's drawer, the parents'
// app and the PDF — so a kind added here (a scholarship in T18, a credit note in
// T8) reaches all three the same day.
type MovementKind string

// The kinds a statement line can be.
const (
	// MovementCharge is a debt the school issued: a month, an inscription, a fee.
	MovementCharge MovementKind = "charge"
	// MovementSurcharge is a late fee: written down as its own charge, or
	// accrued by the daily rule up to the statement's day and not yet written.
	MovementSurcharge MovementKind = "surcharge"
	// MovementDiscount is money taken off a charge (pronto pago, level
	// discount, price at payment date, or one the office typed).
	MovementDiscount MovementKind = "discount"
	// MovementScholarship is a scholarship's part of a charge. Reserved for
	// T18: nothing issues it yet, every reader already renders it.
	MovementScholarship MovementKind = "scholarship"
	// MovementPayment is money the family handed over.
	MovementPayment MovementKind = "payment"
	// MovementCreditNote is a credit note used as payment. Reserved for T8.
	MovementCreditNote MovementKind = "credit_note"
)

// Debit reports whether the movement raises what the family owes.
func (k MovementKind) Debit() bool {
	return k == MovementCharge || k == MovementSurcharge
}

// money reports whether the movement is money received rather than an
// adjustment to a charge.
func (k MovementKind) money() bool {
	return k == MovementPayment || k == MovementCreditNote
}

// rank orders the kinds that share a charge and a day: the charge, then what
// was added to it, then what was taken off.
func (k MovementKind) rank() int {
	switch k {
	case MovementCharge:
		return 0
	case MovementSurcharge:
		return 1
	case MovementDiscount:
		return 2
	case MovementScholarship:
		return 3
	case MovementPayment:
		return 4
	default:
		return 5
	}
}

// Movement is one line of the statement. AmountCents is always positive; the
// kind says which way it moves the balance.
type Movement struct {
	Kind        MovementKind
	Date        time.Time
	Description string
	AmountCents int64
	// ChargeID names the charge a debt or an adjustment belongs to.
	ChargeID string
	// DepositID names the money a payment line is (empty for a payment made
	// straight against a charge, outside any deposit).
	DepositID string
	Method    string
	CardType  string
	Reference string
	Folio     string
	// Accrued marks a surcharge the daily rule has earned up to the
	// statement's day and nobody has written down yet. It counts: it is what
	// the counter would collect.
	Accrued bool
	// NotYetDue marks a charge that has not opened (T5). Shown to the office,
	// never counted in the balance.
	NotYetDue bool
	// Voided marks a cancelled deposit: shown, struck through, never counted.
	Voided     bool
	VoidReason string
	// BalanceCents is the running balance after this line, set by
	// BuildStatement. Negative is money the school holds for the family.
	BalanceCents int64
}

// counts reports whether the line moves the balance.
func (m Movement) counts() bool { return !m.Voided && !m.NotYetDue }

// StatementTotals are the statement's footing. BalanceCents = ChargedCents −
// DiscountCents − PaidCents; negative is credit in the family's favour.
type StatementTotals struct {
	// ChargedCents is every charge and surcharge in play.
	ChargedCents int64
	// DiscountCents is every discount and scholarship in play.
	DiscountCents int64
	// PaidCents is every live payment and credit note.
	PaidCents    int64
	BalanceCents int64
	// NotYetDueCents is what the charges that have not opened will ask for,
	// net of their discounts. Not part of the balance.
	NotYetDueCents int64
}

// BuildStatement orders the movements and works out the running balance and
// the totals. The input is not modified.
//
// Order: by date; on a day, each charge with its own adjustments right after
// it, then the money received. Ties fall back to the ids so the same ledger is
// always printed the same way.
func BuildStatement(in []Movement) ([]Movement, StatementTotals) {
	out := slices.Clone(in)
	slices.SortFunc(out, compareMovements)

	var totals StatementTotals
	var balance int64
	for i := range out {
		m := &out[i]
		if m.counts() {
			balance += signed(*m)
			addTo(&totals, *m)
		} else if m.NotYetDue {
			totals.NotYetDueCents += signed(*m)
		}
		m.BalanceCents = balance
	}
	totals.BalanceCents = balance
	return out, totals
}

func compareMovements(a, b Movement) int {
	return cmp.Or(
		a.Date.Compare(b.Date),
		cmp.Compare(moneyGroup(a.Kind), moneyGroup(b.Kind)),
		cmp.Compare(a.ChargeID, b.ChargeID),
		cmp.Compare(a.Kind.rank(), b.Kind.rank()),
		cmp.Compare(a.DepositID, b.DepositID),
	)
}

func moneyGroup(k MovementKind) int {
	if k.money() {
		return 1
	}
	return 0
}

func signed(m Movement) int64 {
	if m.Kind.Debit() {
		return m.AmountCents
	}
	return -m.AmountCents
}

func addTo(t *StatementTotals, m Movement) {
	switch {
	case m.Kind.Debit():
		t.ChargedCents += m.AmountCents
	case m.Kind.money():
		t.PaidCents += m.AmountCents
	default:
		t.DiscountCents += m.AmountCents
	}
}
