package domain

import (
	"strings"
	"time"

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
	}, nil
}

// Close closes an open period at `at` on behalf of actor and bumps the
// version. A closed period is refused with ALREADY_CLOSED.
func (p PeriodClosure) Close(actor Actor, at time.Time) (PeriodClosure, error) {
	if p.status == PeriodClosed {
		return PeriodClosure{}, Conflict(CodeAlreadyClosed, msgAlreadyClosed)
	}
	p.status = PeriodClosed
	p.version++
	p.closedAt, p.closedBy, p.closedByName = at, actor.ID, actor.Name
	return p, nil
}

// Reopen opens a closed period at `at`, keeping its version. An open period is
// refused with ALREADY_OPEN.
func (p PeriodClosure) Reopen(at time.Time) (PeriodClosure, error) {
	if p.status == PeriodOpen {
		return PeriodClosure{}, Conflict(CodeAlreadyOpen, msgAlreadyOpen)
	}
	p.status = PeriodOpen
	p.reopenedAt = at
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
