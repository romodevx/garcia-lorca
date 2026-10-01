package domain

import (
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// MaxCashCutNote bounds the note left on a closure and a reopen's reason.
const MaxCashCutNote = 500

// MethodTotals is an amount per way of moving money, and their sum (T10).
// Signed: a balance (income − expenses) may be negative.
type MethodTotals struct {
	CashCents     int64
	CardCents     int64
	TransferCents int64
	TotalCents    int64
}

// Add counts cents under method. Only money counts: a credit note settles a
// debt without bringing anything in (ADR 0025), and "mixed" is a deposit's
// summary, never a tender's own method — neither moves the figures.
func (m *MethodTotals) Add(method string, cents int64) {
	switch method {
	case MethodCash:
		m.CashCents += cents
	case MethodCard:
		m.CardCents += cents
	case MethodTransfer:
		m.TransferCents += cents
	default:
		return
	}
	m.TotalCents += cents
}

// Minus is m − o, method by method.
func (m MethodTotals) Minus(o MethodTotals) MethodTotals {
	return MethodTotals{
		CashCents:     m.CashCents - o.CashCents,
		CardCents:     m.CardCents - o.CardCents,
		TransferCents: m.TransferCents - o.TransferCents,
		TotalCents:    m.TotalCents - o.TotalCents,
	}
}

// CutFigures are a day's money as the cut adds it up: what came in, what went
// out, and what credit notes settled (apart: not money).
type CutFigures struct {
	Income          MethodTotals
	Expenses        MethodTotals
	CreditNoteCents int64
}

// ExpectedCashCents is the cash the drawer should hold at close: cash in less
// cash out. Card and transfer never touch the drawer.
func (f CutFigures) ExpectedCashCents() int64 {
	return f.Income.CashCents - f.Expenses.CashCents
}

// CashCutClosure is one closure of one day's cut: the figures as they stood,
// the cash counted, the difference, who closed it and when. A reopen stamps it;
// it is never deleted, so a day's history shows every closure it had.
type CashCutClosure struct {
	id           string
	day          time.Time
	figures      CutFigures
	counted      int64
	note         string
	closedBy     Receiver
	closedAt     time.Time
	reopenedAt   time.Time
	reopenedBy   Receiver
	reopenReason string
}

// CloseCashCut closes day with the cash counted in the drawer. The figures are
// a snapshot: what the cut said at the moment of closing. now bounds the day —
// a day that has not started cannot be closed.
func CloseCashCut(id string, day time.Time, figures CutFigures, countedCashCents int64, note string,
	by Receiver, at, now time.Time,
) (CashCutClosure, error) {
	if day.IsZero() {
		return CashCutClosure{}, glerrors.New(glerrors.KindInvalid, "el día del corte es obligatorio")
	}
	if CalendarDay(day).After(CalendarDay(now)) {
		return CashCutClosure{}, glerrors.New(glerrors.KindInvalid, "no se puede cerrar un día futuro")
	}
	if countedCashCents < 0 {
		return CashCutClosure{}, glerrors.New(glerrors.KindInvalid, "el efectivo contado no puede ser negativo")
	}
	if _, err := NewCents(countedCashCents); err != nil {
		return CashCutClosure{}, err
	}
	trimmed := strings.TrimSpace(note)
	if len([]rune(trimmed)) > MaxCashCutNote {
		return CashCutClosure{}, glerrors.New(glerrors.KindInvalid, "la nota admite como máximo 500 caracteres")
	}
	return CashCutClosure{
		id: id, day: CalendarDay(day), figures: figures, counted: countedCashCents,
		note: trimmed, closedBy: by, closedAt: at,
	}, nil
}

// Reopen stamps the closure reopened, with its reason (required). The day may
// then be closed again; this closure stays as history.
func (c CashCutClosure) Reopen(reason string, by Receiver, at time.Time) (CashCutClosure, error) {
	if c.Reopened() {
		return CashCutClosure{}, glerrors.New(glerrors.KindConflict, "el corte ya estaba reabierto")
	}
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return CashCutClosure{}, glerrors.New(glerrors.KindInvalid, "el motivo para reabrir el corte es obligatorio")
	}
	if len([]rune(trimmed)) > MaxCashCutNote {
		return CashCutClosure{}, glerrors.New(glerrors.KindInvalid, "el motivo admite como máximo 500 caracteres")
	}
	c.reopenedAt, c.reopenedBy, c.reopenReason = at, by, trimmed
	return c, nil
}

// RehydrateCashCutClosure rebuilds a stored closure (repository only).
func RehydrateCashCutClosure(id string, day time.Time, figures CutFigures, counted int64, note string,
	closedBy Receiver, closedAt, reopenedAt time.Time, reopenedBy Receiver, reopenReason string,
) CashCutClosure {
	return CashCutClosure{
		id: id, day: CalendarDay(day), figures: figures, counted: counted, note: note,
		closedBy: closedBy, closedAt: closedAt, reopenedAt: reopenedAt, reopenedBy: reopenedBy, reopenReason: reopenReason,
	}
}

// ID is the closure's identifier.
func (c CashCutClosure) ID() string { return c.id }

// Day is the day closed (midnight UTC).
func (c CashCutClosure) Day() time.Time { return c.day }

// Figures are the cut's figures at the moment of closing.
func (c CashCutClosure) Figures() CutFigures { return c.figures }

// ExpectedCashCents is the cash the drawer should have held then.
func (c CashCutClosure) ExpectedCashCents() int64 { return c.figures.ExpectedCashCents() }

// CountedCashCents is the cash that was counted.
func (c CashCutClosure) CountedCashCents() int64 { return c.counted }

// DifferenceCents is counted − expected: negative is money missing.
func (c CashCutClosure) DifferenceCents() int64 { return c.counted - c.ExpectedCashCents() }

// Note is what the person closing wrote.
func (c CashCutClosure) Note() string { return c.note }

// ClosedBy is who closed the day.
func (c CashCutClosure) ClosedBy() Receiver { return c.closedBy }

// ClosedAt is when.
func (c CashCutClosure) ClosedAt() time.Time { return c.closedAt }

// Reopened says whether the closure was undone.
func (c CashCutClosure) Reopened() bool { return !c.reopenedAt.IsZero() }

// ReopenedAt is when it was reopened; zero while live.
func (c CashCutClosure) ReopenedAt() time.Time { return c.reopenedAt }

// ReopenedBy is who reopened it.
func (c CashCutClosure) ReopenedBy() Receiver { return c.reopenedBy }

// ReopenReason is why.
func (c CashCutClosure) ReopenReason() string { return c.reopenReason }
