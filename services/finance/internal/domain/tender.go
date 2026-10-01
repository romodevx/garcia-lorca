package domain

import (
	"fmt"
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// MaxTenders is how many methods one payment may combine (T8). The counter
// offers four; a fifth is a typing accident, not a family paying.
const MaxTenders = 4

// Tender is one method a payment came in by, and how much of it (T8).
//
// A family may settle part of a bill with a credit note the school issued by
// hand and the rest in cash or by card. The school does not automate credit
// balances: the cashier types the credit note's amount, and it travels as a
// tender like any other. What sets it apart is IsMoney — nothing reaches the
// drawer, so cash-ups and income reports keep it on a line of its own.
//
// A Tender is a value; it has no identity of its own outside the payment it
// belongs to.
type Tender struct {
	method    string
	amount    Cents
	reference string
	cardType  string
}

// NewTender builds a validated tender. reference is free text (a credit note's
// folio, a voucher, a transfer id); cardType is debit or credit, and only on a
// card.
func NewTender(method string, amountCents int64, reference, cardType string) (Tender, error) {
	if err := validateMethod(method); err != nil {
		return Tender{}, err
	}
	if amountCents <= 0 {
		return Tender{}, glerrors.New(glerrors.KindInvalid, "cada forma de pago necesita un monto mayor a cero")
	}
	amount, err := NewCents(amountCents)
	if err != nil {
		return Tender{}, err
	}
	if err := validateCardType(method, cardType); err != nil {
		return Tender{}, err
	}
	return Tender{method: method, amount: amount, reference: strings.TrimSpace(reference), cardType: cardType}, nil
}

// validateCardType: debit or credit, and only on a card. Empty is the ordinary
// state of everything else, and of card payments from before it was asked.
func validateCardType(method, cardType string) error {
	if cardType == "" {
		return nil
	}
	if cardType != CardDebit && cardType != CardCredit {
		return glerrors.New(glerrors.KindInvalid, "card type must be debit or credit")
	}
	if method != MethodCard {
		return glerrors.New(glerrors.KindInvalid, "only a card payment has a card type")
	}
	return nil
}

// Method is how this part arrived.
func (t Tender) Method() string { return t.method }

// Amount is how much of the payment came this way.
func (t Tender) Amount() Cents { return t.amount }

// Reference identifies this part: a credit note's folio, a voucher.
func (t Tender) Reference() string { return t.reference }

// CardType is debit or credit on a card, empty otherwise.
func (t Tender) CardType() string { return t.cardType }

// IsMoney says whether this part reached the drawer or the bank. A credit note
// did not: it settles a debt without bringing money in.
func (t Tender) IsMoney() bool { return t.method != MethodCreditNote }

// ValidateTenders is the rule the school approved for a combined payment: one
// to MaxTenders methods that add up EXACTLY to the amount being paid.
func ValidateTenders(tenders []Tender, amountCents int64) error {
	if len(tenders) == 0 {
		return glerrors.New(glerrors.KindInvalid, "falta la forma de pago")
	}
	if len(tenders) > MaxTenders {
		return glerrors.New(glerrors.KindInvalid, "un pago admite como máximo cuatro formas de pago")
	}
	var sum int64
	for _, t := range tenders {
		sum += t.Amount().Int64()
	}
	if sum != amountCents {
		return glerrors.New(glerrors.KindInvalid, "la suma de las formas de pago debe ser igual al monto a pagar")
	}
	return nil
}

// TenderKey is the idempotency key of the i-th row of a combined payment: the
// caller's key on the first, and "#2", "#3"… after it. An empty key stays empty
// — the caller opted out of deduplication. The use case mints these and the
// repository looks the set up by them, so it is said once, here.
func TenderKey(key string, i int) string {
	if key == "" || i == 0 {
		return key
	}
	return fmt.Sprintf("%s#%d", key, i+1)
}

// TenderedAllocation is a slice of an allocation paid by one tender.
type TenderedAllocation struct {
	Allocation
	Tender Tender
}

// SplitByTender cuts a deposit's allocations wherever one tender ends and the
// next begins: credit notes first, then money, each in the order given.
//
// It is what keeps the payments table honest about method: every row carries
// one method, so "how much of September was paid with a credit note" is a sum,
// not an apportionment. Allocations are already in the school's order (oldest
// debt first), and the tenders are consumed credit notes first, then in the
// order the cashier listed them — deterministic, so a replay splits the same
// way. Money beyond the allocations (the family's credit) has no row, as
// before, and is money rather than a credit note whenever the note fits.
func SplitByTender(allocations []Allocation, tenders []Tender) []TenderedAllocation {
	out := make([]TenderedAllocation, 0, len(allocations)+len(tenders))
	cursor := &tenderCursor{tenders: creditNotesFirst(tenders)}
	for _, a := range allocations {
		for need := a.AmountCents; need > 0; {
			tender, took, ok := cursor.take(need)
			if !ok {
				break
			}
			slice := a
			slice.AmountCents = took
			out = append(out, TenderedAllocation{Allocation: slice, Tender: tender})
			need -= took
		}
	}
	return out
}

// creditNotesFirst orders the tenders so credit notes are spent before money,
// each group keeping the cashier's order. What a deposit leaves unallocated
// (the family's credit) is then money they handed over, never a credit note,
// and the cash-up and the income report agree on what the note settled.
func creditNotesFirst(tenders []Tender) []Tender {
	out := make([]Tender, 0, len(tenders))
	for _, t := range tenders {
		if !t.IsMoney() {
			out = append(out, t)
		}
	}
	for _, t := range tenders {
		if t.IsMoney() {
			out = append(out, t)
		}
	}
	return out
}

// tenderCursor walks the tenders, remembering how much of the current one is
// still unspent.
type tenderCursor struct {
	tenders []Tender
	next    int
	left    int64
}

// take spends up to want from the current tender, moving to the next one when
// it runs out. ok is false once every tender is spent.
func (c *tenderCursor) take(want int64) (Tender, int64, bool) {
	for c.left == 0 {
		if c.next == len(c.tenders) {
			return Tender{}, 0, false
		}
		c.left = c.tenders[c.next].Amount().Int64()
		c.next++
	}
	took := min(want, c.left)
	c.left -= took
	return c.tenders[c.next-1], took, true
}
