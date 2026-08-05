package domain

import (
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Term is a period (periodo) within a cycle for a given level, e.g. "T1". It is
// ordered within its (cycle, level) by `ordinal` (1-based) and carries a date
// range.
type Term struct {
	id       string
	tenantID string
	cycleID  string
	levelID  string
	name     string
	ordinal  int
	startsOn time.Time
	endsOn   time.Time
}

// NewTerm constructs a term. `id`, `tenantID`, `cycleID`, `levelID`, and `name`
// are required, `ordinal` must be at least 1, and `endsOn` must not be before
// `startsOn`.
func NewTerm(id, tenantID, cycleID, levelID, name string, ordinal int, startsOn, endsOn time.Time) (Term, error) {
	if strings.TrimSpace(id) == "" {
		return Term{}, glerrors.New(glerrors.KindInvalid, "term id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return Term{}, glerrors.New(glerrors.KindInvalid, "term tenant id is required")
	}
	if strings.TrimSpace(cycleID) == "" {
		return Term{}, glerrors.New(glerrors.KindInvalid, "term cycle id is required")
	}
	if strings.TrimSpace(levelID) == "" {
		return Term{}, glerrors.New(glerrors.KindInvalid, "term level id is required")
	}
	if strings.TrimSpace(name) == "" {
		return Term{}, glerrors.New(glerrors.KindInvalid, "term name is required")
	}
	if ordinal < 1 {
		return Term{}, glerrors.New(glerrors.KindInvalid, "term ordinal must be at least 1")
	}
	if endsOn.Before(startsOn) {
		return Term{}, glerrors.New(glerrors.KindInvalid, "term ends_on must not be before starts_on")
	}
	return Term{
		id:       id,
		tenantID: tenantID,
		cycleID:  cycleID,
		levelID:  levelID,
		name:     name,
		ordinal:  ordinal,
		startsOn: startsOn,
		endsOn:   endsOn,
	}, nil
}

// ID returns the stable term identifier.
func (t Term) ID() string { return t.id }

// TenantID returns the tenant the term belongs to.
func (t Term) TenantID() string { return t.tenantID }

// CycleID returns the parent cycle identifier.
func (t Term) CycleID() string { return t.cycleID }

// LevelID returns the level the term applies to.
func (t Term) LevelID() string { return t.levelID }

// Name returns the human-readable term name (e.g. "T1").
func (t Term) Name() string { return t.name }

// Ordinal returns the 1-based ordering of the term within its (cycle, level).
func (t Term) Ordinal() int { return t.ordinal }

// StartsOn returns the first day of the term.
func (t Term) StartsOn() time.Time { return t.startsOn }

// EndsOn returns the last day of the term.
func (t Term) EndsOn() time.Time { return t.endsOn }
