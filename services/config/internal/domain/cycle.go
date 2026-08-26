package domain

import (
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// CycleStatus is the lifecycle state of an academic cycle (school year). A cycle
// is drafted, then opened for the year, then closed once the year ends.
//
//	draft ──activate──> active ──close──> closed
type CycleStatus uint8

const (
	// CycleStatusUnspecified is the zero value; not a valid status.
	CycleStatusUnspecified CycleStatus = iota
	// CycleStatusDraft: the cycle is being prepared and is not yet in use.
	CycleStatusDraft
	// CycleStatusActive: the cycle is the live school year.
	CycleStatusActive
	// CycleStatusClosed: the cycle has ended and is read-only.
	CycleStatusClosed
)

var cycleStatusNames = map[CycleStatus]string{
	CycleStatusDraft:  "draft",
	CycleStatusActive: "active",
	CycleStatusClosed: "closed",
}

// String returns the canonical status name ("" for unspecified).
func (s CycleStatus) String() string { return cycleStatusNames[s] }

// Valid reports whether the status is a real cycle status (not unspecified).
func (s CycleStatus) Valid() bool { return s >= CycleStatusDraft && s <= CycleStatusClosed }

// ParseCycleStatus resolves a status name (case-insensitive) to a CycleStatus.
func ParseCycleStatus(raw string) (CycleStatus, error) {
	key := strings.ToLower(strings.TrimSpace(raw))
	for status, name := range cycleStatusNames {
		if name == key {
			return status, nil
		}
	}
	return CycleStatusUnspecified, glerrors.New(glerrors.KindInvalid, "unknown cycle status: "+raw)
}

// Cycle is a per-cycle umbrella: a named school year ("2026-2027") with a
// lifecycle status and a date range. Terms and groups hang off a cycle.
type Cycle struct {
	id       string
	tenantID string
	name     string
	status   CycleStatus
	startsOn time.Time
	endsOn   time.Time
}

// NewCycle constructs a cycle. `id`, `tenantID`, and `name` are required,
// `status` must be a real status (not unspecified), and `endsOn` must not be
// before `startsOn`.
func NewCycle(id, tenantID, name string, status CycleStatus, startsOn, endsOn time.Time) (Cycle, error) {
	if strings.TrimSpace(id) == "" {
		return Cycle{}, glerrors.New(glerrors.KindInvalid, "cycle id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return Cycle{}, glerrors.New(glerrors.KindInvalid, "cycle tenant id is required")
	}
	if strings.TrimSpace(name) == "" {
		return Cycle{}, glerrors.New(glerrors.KindInvalid, "cycle name is required")
	}
	if !status.Valid() {
		return Cycle{}, glerrors.New(glerrors.KindInvalid, "cycle status is invalid")
	}
	if endsOn.Before(startsOn) {
		return Cycle{}, glerrors.New(glerrors.KindInvalid, "cycle ends_on must not be before starts_on")
	}
	return Cycle{
		id:       id,
		tenantID: tenantID,
		name:     name,
		status:   status,
		startsOn: startsOn,
		endsOn:   endsOn,
	}, nil
}

// ID returns the stable cycle identifier.
func (c Cycle) ID() string { return c.id }

// TenantID returns the tenant the cycle belongs to.
func (c Cycle) TenantID() string { return c.tenantID }

// Name returns the human-readable cycle name (e.g. "2026-2027").
func (c Cycle) Name() string { return c.name }

// Status returns the cycle lifecycle state.
func (c Cycle) Status() CycleStatus { return c.status }

// StartsOn returns the first day of the cycle.
func (c Cycle) StartsOn() time.Time { return c.startsOn }

// EndsOn returns the last day of the cycle.
func (c Cycle) EndsOn() time.Time { return c.endsOn }

// Activate makes the cycle the school's live year.
//
// A cycle was born in draft and nothing could move it, so every school year read
// "Borrador" forever — and the panel, which resolves "the active cycle" to decide
// which groups and prices to show, silently fell back to whichever cycle came
// first. That fallback is invisible and wrong the moment a school has two.
//
// Re-activating the live year is a no-op rather than an error: it is what a
// person does when they are not sure, and refusing would teach them nothing.
// A closed year cannot be re-opened — it is history a school has already
// reported on, and issuing charges or grades against it is not a thing to make
// easy.
func (c Cycle) Activate() (Cycle, error) {
	switch c.status {
	case CycleStatusActive:
		return c, nil
	case CycleStatusDraft:
		c.status = CycleStatusActive
		return c, nil
	default:
		return Cycle{}, glerrors.New(glerrors.KindConflict,
			"a closed school year cannot be reopened")
	}
}

// Close ends a live cycle, making it read-only.
//
// Only a live year can be closed: a draft has nothing to close, and saying so is
// better than accepting a click that means nothing.
func (c Cycle) Close() (Cycle, error) {
	switch c.status {
	case CycleStatusClosed:
		return c, nil
	case CycleStatusActive:
		c.status = CycleStatusClosed
		return c, nil
	default:
		return Cycle{}, glerrors.New(glerrors.KindConflict,
			"a school year that never opened cannot be closed")
	}
}
