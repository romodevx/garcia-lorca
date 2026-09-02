package domain

import (
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// maxFeesPerPlan bounds how many extra fees one level may carry. It is not a
// business rule the school asked for; it is what stops a malformed payload from
// turning one billing run into four hundred pupils times an unbounded list.
const maxFeesPerPlan = 24

// Fee is money the school collects on top of the monthly tuition, spread across
// the months it chooses.
//
// It exists because a school's year and its calendar do not agree. "La cuota de
// julio" is charged for a month the year does not bill, so it has to be
// collected inside months that it does — half in September, half in October is
// the arrangement the school actually runs. Before this, saying that meant
// hand-writing a payment schedule of month:amount pairs, which priced the
// tuition and the fee as one number and left nobody able to explain either.
type Fee struct {
	// Name is what the family reads on the statement. It is the whole
	// explanation of why a month costs more than the tabulador says, so it is
	// required.
	Name string
	// AmountCents is the WHOLE fee, not the monthly share. The school states
	// what it collects; the split is arithmetic, and arithmetic is this type's
	// job rather than the director's.
	AmountCents int64
	// Months are the calendar months (1-12) the fee is divided between, in the
	// order the school chose them. One month is a fee that is not split.
	Months []int
}

// FeeInstallment is one share of a fee, placed on the calendar.
//
// Part and Of are carried rather than derived later because the family's
// statement says "Cuota de Julio (1 de 2)", and a row that cannot say which
// half it is turns two identical charges into a suspected double bill.
type FeeInstallment struct {
	Name        string
	Part        int
	Of          int
	DueOn       time.Time
	AmountCents int64
}

// validate reports what a school cannot mean by a fee.
func (f Fee) validate() error {
	if strings.TrimSpace(f.Name) == "" {
		return glerrors.New(glerrors.KindInvalid, "a fee needs a name, or nobody can say what it is for")
	}
	if f.AmountCents <= 0 {
		return glerrors.New(glerrors.KindInvalid, "a fee must be for some money")
	}
	if len(f.Months) == 0 {
		return glerrors.New(glerrors.KindInvalid, "choose the months the fee is collected in")
	}
	seen := make(map[int]bool, len(f.Months))
	for _, m := range f.Months {
		if m < 1 || m > monthsInYear {
			return glerrors.New(glerrors.KindInvalid, "a fee is collected in a month from 1 to 12")
		}
		if seen[m] {
			// Not "collect it twice": a slip. Accepting it silently would halve
			// each share and collect the fee across what looks like one month.
			return glerrors.New(glerrors.KindInvalid, "the same month is listed twice for one fee")
		}
		seen[m] = true
	}
	if f.AmountCents < int64(len(f.Months)) {
		// Split so thin that a share rounds to nothing. A charge for zero pesos
		// sits in a statement forever, already settled and explaining nothing.
		return glerrors.New(glerrors.KindInvalid, "the fee is too small to split between that many months")
	}
	return nil
}

// shares divides the fee into one amount per month, losing no cent.
//
// The remainder goes to the FIRST payment. A thousand pesos over three months is
// not three times 333.33 — that collects a cent less than the school asked for,
// on every pupil, every year. Somebody eventually reconciles the accounts and
// cannot find it.
func (f Fee) shares() []int64 {
	n := int64(len(f.Months))
	base := f.AmountCents / n
	out := make([]int64, len(f.Months))
	for i := range out {
		out[i] = base
	}
	out[0] += f.AmountCents - base*n
	return out
}
