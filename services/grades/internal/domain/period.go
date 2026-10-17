package domain

import (
	"strings"
	"time"
	"unicode/utf8"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// PeriodStatus is whether a period (a config term: one level of one cycle)
// still accepts grade changes.
type PeriodStatus string

// The two states of a period.
const (
	PeriodOpen   PeriodStatus = "open"
	PeriodClosed PeriodStatus = "closed"
)

// Sentences the period lifecycle refuses with.
const (
	msgAlreadyClosed = "El periodo ya está cerrado"
	msgAlreadyOpen   = "El periodo ya está abierto"
)

// Actor is the authenticated person closing a period: their id for the audit
// and their name for the snapshot.
type Actor struct {
	ID   string
	Name string
}

// PeriodClosure is the closure state of one term (T7). version counts how many
// times it has been closed: it is the version of the latest report cards, so a
// period closed, reopened and closed again issues version 2. Reopening keeps
// the version — the issued report cards stay published until the next close
// supersedes them.
type PeriodClosure struct {
	termID       string
	cycleID      string
	levelID      string
	status       PeriodStatus
	version      int
	closedAt     time.Time
	closedBy     string
	closedByName string
	reopenedAt   time.Time
	// reopenedBy / reopenedByName / reopenReason are who last reopened the
	// period and why (CC-308): the audit of the latest reopen.
	reopenedBy     string
	reopenedByName string
	reopenReason   string
	// months are the term's months when it was last closed: while closed, the
	// term locks them even if config later moves a month out of the term.
	months []Month
}

// PeriodClosureFields are the stored parts of a PeriodClosure.
type PeriodClosureFields struct {
	TermID       string
	CycleID      string
	LevelID      string
	Status       PeriodStatus
	Version      int
	ClosedAt     time.Time
	ClosedBy     string
	ClosedByName string
	ReopenedAt   time.Time
	// ReopenedBy, ReopenedByName and ReopenReason: the latest reopen's audit.
	ReopenedBy     string
	ReopenedByName string
	ReopenReason   string
	Months         []Month
}

// NewOpenPeriod is a period that was never closed: open, version 0.
func NewOpenPeriod(termID, cycleID, levelID string) PeriodClosure {
	return PeriodClosure{termID: termID, cycleID: cycleID, levelID: levelID, status: PeriodOpen}
}

// RestorePeriodClosure rebuilds a stored closure, refusing a state the table
// cannot hold (KindInvalid).
func RestorePeriodClosure(f PeriodClosureFields) (PeriodClosure, error) {
	if strings.TrimSpace(f.TermID) == "" {
		return PeriodClosure{}, glerrors.New(glerrors.KindInvalid, "term id is required")
	}
	if f.Status != PeriodOpen && f.Status != PeriodClosed {
		return PeriodClosure{}, glerrors.New(glerrors.KindInvalid, "period status must be open or closed")
	}
	if f.Version < 0 {
		return PeriodClosure{}, glerrors.New(glerrors.KindInvalid, "period version cannot be negative")
	}
	return PeriodClosure{
		termID: f.TermID, cycleID: f.CycleID, levelID: f.LevelID, status: f.Status, version: f.Version,
		closedAt: f.ClosedAt, closedBy: f.ClosedBy, closedByName: f.ClosedByName, reopenedAt: f.ReopenedAt,
		reopenedBy: f.ReopenedBy, reopenedByName: f.ReopenedByName, reopenReason: f.ReopenReason,
		months: append([]Month(nil), f.Months...),
	}, nil
}

// Close closes an open period at `at` on behalf of actor, bumps the version
// and snapshots the months the term covers now (copied). A closed period is
// refused with ALREADY_CLOSED.
func (p PeriodClosure) Close(actor Actor, at time.Time, months []Month) (PeriodClosure, error) {
	if p.status == PeriodClosed {
		return PeriodClosure{}, Conflict(CodeAlreadyClosed, msgAlreadyClosed)
	}
	p.status = PeriodClosed
	p.version++
	p.closedAt, p.closedBy, p.closedByName = at, actor.ID, actor.Name
	p.months = append([]Month(nil), months...)
	return p, nil
}

// MaxReopenReasonLength bounds, in characters, why a period was reopened (the
// column carries the same CHECK).
const MaxReopenReasonLength = 500

// msgReopenReason is the refusal of a reopen without a usable reason.
const msgReopenReason = "Escribe el motivo para reabrir el periodo (máximo 500 caracteres)"

// ParseReopenReason is why a period is reopened, trimmed: 1..
// MaxReopenReasonLength characters, KindInvalid otherwise (CC-308).
func ParseReopenReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || utf8.RuneCountInString(reason) > MaxReopenReasonLength {
		return "", glerrors.New(glerrors.KindInvalid, msgReopenReason)
	}
	return reason, nil
}

// Reopen opens a closed period at `at` on behalf of actor, keeping its version,
// and records who reopened it and why (CC-308; see ParseReopenReason). An
// open period is refused with ALREADY_OPEN.
func (p PeriodClosure) Reopen(actor Actor, reason string, at time.Time) (PeriodClosure, error) {
	reason, err := ParseReopenReason(reason)
	if err != nil {
		return PeriodClosure{}, err
	}
	if p.status == PeriodOpen {
		return PeriodClosure{}, Conflict(CodeAlreadyOpen, msgAlreadyOpen)
	}
	p.status = PeriodOpen
	p.reopenedAt = at
	p.reopenedBy, p.reopenedByName, p.reopenReason = actor.ID, actor.Name, reason
	return p, nil
}

// Accessors.
func (p PeriodClosure) TermID() string       { return p.termID }
func (p PeriodClosure) CycleID() string      { return p.cycleID }
func (p PeriodClosure) LevelID() string      { return p.levelID }
func (p PeriodClosure) Status() PeriodStatus { return p.status }
func (p PeriodClosure) IsClosed() bool       { return p.status == PeriodClosed }
func (p PeriodClosure) Version() int         { return p.version }
func (p PeriodClosure) ClosedAt() time.Time  { return p.closedAt }
func (p PeriodClosure) ClosedBy() string     { return p.closedBy }
func (p PeriodClosure) ClosedByName() string { return p.closedByName }
func (p PeriodClosure) ReopenedAt() time.Time {
	return p.reopenedAt
}

// ReopenedBy is the iam user who last reopened the period ("" if never).
func (p PeriodClosure) ReopenedBy() string { return p.reopenedBy }

// ReopenedByName is that user's name as it read when they reopened it.
func (p PeriodClosure) ReopenedByName() string { return p.reopenedByName }

// ReopenReason is why the period was last reopened.
func (p PeriodClosure) ReopenReason() string { return p.reopenReason }

// Months is the snapshot of the term's months taken by the last close (a copy).
func (p PeriodClosure) Months() []Month { return append([]Month(nil), p.months...) }
