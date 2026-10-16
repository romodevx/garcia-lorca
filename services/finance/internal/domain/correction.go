package domain

import (
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// MaxCorrectionReason bounds the reason a correction is recorded with: a
// sentence or two, never a document.
const MaxCorrectionReason = 500

// correctionLink is a deposit's place in a correction (T9).
//
// A registered payment that was keyed wrong ($500 for $1,000, the wrong day,
// cash for a transfer) is not edited in place: the original is SUPERSEDED — kept,
// with its folio, counting for nothing — and a new deposit carries the corrected
// figures and names the one it corrects. Both receipts can be printed again,
// each saying which way the link runs.
type correctionLink struct {
	correctsID        string
	correctsFolio     string
	supersededAt      time.Time
	supersededBy      string
	supersededByFolio string
}

// Supersede marks the deposit as replaced by its correction byID.
//
// A cancelled deposit cannot be corrected (there is no money left to correct),
// and a deposit is corrected once: a second correction corrects its successor,
// so the chain stays a line and every receipt names at most one replacement.
func (d Deposit) Supersede(byID string, at time.Time) (Deposit, error) {
	if d.Voided() {
		return Deposit{}, glerrors.New(glerrors.KindConflict, "un pago cancelado no se puede corregir")
	}
	if d.Superseded() {
		return Deposit{}, glerrors.New(glerrors.KindConflict, "este pago ya fue corregido; corrige el pago que lo sustituye")
	}
	byID = strings.TrimSpace(byID)
	if byID == "" || byID == d.id {
		return Deposit{}, glerrors.New(glerrors.KindInvalid, "a superseded deposit needs another deposit to replace it")
	}
	if at.IsZero() {
		return Deposit{}, glerrors.New(glerrors.KindInvalid, "correction date is required")
	}
	out := d
	out.link.supersededBy = byID
	out.link.supersededAt = at
	return out, nil
}

// RehydrateSupersession rebuilds a stored deposit's replacement. Only the
// persistence mapper calls it, as with Rehydrate.
func (d Deposit) RehydrateSupersession(at time.Time, byID, byFolio string) Deposit {
	d.link.supersededAt, d.link.supersededBy, d.link.supersededByFolio = at, byID, byFolio
	return d
}

// WithCorrects returns the deposit as the correction of another one.
func (d Deposit) WithCorrects(originalID, originalFolio string) Deposit {
	d.link.correctsID, d.link.correctsFolio = originalID, originalFolio
	return d
}

// Superseded reports whether a correction replaced this deposit.
func (d Deposit) Superseded() bool { return d.link.supersededBy != "" }

// SupersededAt is when it was corrected (zero if it was not).
func (d Deposit) SupersededAt() time.Time { return d.link.supersededAt }

// SupersededBy is the deposit that corrects this one.
func (d Deposit) SupersededBy() string { return d.link.supersededBy }

// SupersededByFolio is the receipt number of the deposit that corrects this one.
func (d Deposit) SupersededByFolio() string { return d.link.supersededByFolio }

// Corrects is the deposit this one corrects, or empty.
func (d Deposit) Corrects() string { return d.link.correctsID }

// CorrectsFolio is the receipt number of the deposit this one corrects.
func (d Deposit) CorrectsFolio() string { return d.link.correctsFolio }

// Counts reports whether the deposit is money in the ledger: neither
// cancelled nor replaced by a correction.
func (d Deposit) Counts() bool { return !d.Voided() && !d.Superseded() }

// TenderSnapshot is one method of a deposit as a correction recorded it.
type TenderSnapshot struct {
	Method      string
	AmountCents int64
	Reference   string
	CardType    string
}

// SnapshotLine is what a deposit put on one charge.
type SnapshotLine struct {
	ChargeID    string
	AmountCents int64
}

// DepositSnapshot is a deposit as it stood on one side of a correction: the
// figures the office can edit (amount, date, methods, what it paid) and the
// folio that names it.
type DepositSnapshot struct {
	DepositID   string
	Folio       string
	AmountCents int64
	PaidAt      time.Time
	// PaidOn is the school-local day the deposit was judged on.
	PaidOn  time.Time
	Tenders []TenderSnapshot
	Lines   []SnapshotLine
}

// SnapshotOf records a deposit and the payments it split into. A charge paid
// by two tenders is one line, in the order the charges were paid.
func SnapshotOf(d Deposit, allocations []Payment) DepositSnapshot {
	s := DepositSnapshot{DepositID: d.ID(), Folio: d.Folio(), AmountCents: d.Amount().Int64(), PaidAt: d.PaidAt(), PaidOn: d.PaidOn()}
	for _, t := range d.Tenders() {
		s.Tenders = append(s.Tenders, TenderSnapshot{
			Method: t.Method(), AmountCents: t.Amount().Int64(), Reference: t.Reference(), CardType: t.CardType(),
		})
	}
	at := map[string]int{}
	for _, p := range allocations {
		if i, seen := at[p.ChargeID()]; seen {
			s.Lines[i].AmountCents += p.Amount().Int64()
			continue
		}
		at[p.ChargeID()] = len(s.Lines)
		s.Lines = append(s.Lines, SnapshotLine{ChargeID: p.ChargeID(), AmountCents: p.Amount().Int64()})
	}
	return s
}

// DepositCorrection is the record of one correction (T9): what the payment
// was, what it is now, who changed it, when and why. Nothing is lost — the
// original deposit stays, superseded — and this is the answer to "who changed
// my receipt?".
type DepositCorrection struct {
	id          string
	tenantID    string
	studentID   string
	originalID  string
	correctedID string
	reason      string
	before      DepositSnapshot
	after       DepositSnapshot
	by          Receiver
	at          time.Time
}

// ValidateCorrectionReason is the one rule on the reason: present, and short.
// A correction without one is a changed receipt nobody can explain.
func ValidateCorrectionReason(reason string) error {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return glerrors.New(glerrors.KindInvalid, "el motivo de la corrección es obligatorio")
	}
	if len([]rune(trimmed)) > MaxCorrectionReason {
		return glerrors.New(glerrors.KindInvalid, "el motivo de la corrección admite como máximo 500 caracteres")
	}
	return nil
}

// NewDepositCorrection builds a validated correction record.
func NewDepositCorrection(id, tenantID, studentID, originalID, correctedID, reason string,
	before, after DepositSnapshot, by Receiver, at time.Time,
) (DepositCorrection, error) {
	if err := ValidateCorrectionReason(reason); err != nil {
		return DepositCorrection{}, err
	}
	for _, required := range []string{id, tenantID, studentID, originalID, correctedID} {
		if strings.TrimSpace(required) == "" {
			return DepositCorrection{}, glerrors.New(glerrors.KindInvalid, "a correction needs its ids")
		}
	}
	if at.IsZero() {
		return DepositCorrection{}, glerrors.New(glerrors.KindInvalid, "correction date is required")
	}
	return DepositCorrection{
		id: id, tenantID: tenantID, studentID: studentID,
		originalID: originalID, correctedID: correctedID, reason: strings.TrimSpace(reason),
		before: before, after: after, by: by, at: at,
	}, nil
}

// ID returns the correction's identifier.
func (c DepositCorrection) ID() string { return c.id }

// TenantID returns the owning tenant.
func (c DepositCorrection) TenantID() string { return c.tenantID }

// StudentID is the pupil whose ledger was corrected.
func (c DepositCorrection) StudentID() string { return c.studentID }

// OriginalID is the deposit that was superseded.
func (c DepositCorrection) OriginalID() string { return c.originalID }

// CorrectedID is the deposit that replaced it.
func (c DepositCorrection) CorrectedID() string { return c.correctedID }

// Reason is why the payment was corrected.
func (c DepositCorrection) Reason() string { return c.reason }

// Before is the payment as it was.
func (c DepositCorrection) Before() DepositSnapshot { return c.before }

// After is the payment as it is now.
func (c DepositCorrection) After() DepositSnapshot { return c.after }

// By is who corrected it, with the name as it read then.
func (c DepositCorrection) By() Receiver { return c.by }

// At is when it was corrected.
func (c DepositCorrection) At() time.Time { return c.at }
