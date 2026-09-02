package domain

import (
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// maxEventTitleLen and maxEventDescriptionLen bound what a person types. A
// calendar entry is read at a glance; a paragraph in the title is a paragraph
// nobody reads.
const (
	maxEventTitleLen       = 120
	maxEventDescriptionLen = 500
)

// EventAudience is who an event concerns: one level, one group, or — with both
// empty — the whole school. Never both at once: an event addressed to a level
// AND a group has an audience nobody could state.
type EventAudience struct {
	LevelID string
	GroupID string
}

// SchoolEvent is something the school DOES: a festival, a parents' meeting, an
// exam week.
//
// It is deliberately not a NonWorkingPeriod, though both draw a block on the
// same calendar. A closed day is read by finance to move a due date and stop a
// surcharge; an event must never do either. A festival on the 10th does not
// mean nobody could pay on the 10th, and filing the two together would put the
// school's agenda one typo away from moving four hundred families' deadlines.
type SchoolEvent struct {
	id          string
	tenantID    string
	cycleID     string
	title       string
	description string
	from        time.Time
	to          time.Time
	audience    EventAudience
}

// NewSchoolEvent builds a validated event. Both ends are inclusive, and a
// one-day event has from == to.
func NewSchoolEvent(id, tenantID, cycleID, title, description string, from, to time.Time, audience EventAudience) (SchoolEvent, error) {
	if strings.TrimSpace(id) == "" {
		return SchoolEvent{}, glerrors.New(glerrors.KindInvalid, "event id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return SchoolEvent{}, glerrors.New(glerrors.KindInvalid, "tenant id is required")
	}
	if strings.TrimSpace(cycleID) == "" {
		return SchoolEvent{}, glerrors.New(glerrors.KindInvalid, "cycle id is required")
	}
	trimmedTitle := strings.TrimSpace(title)
	if trimmedTitle == "" || len(trimmedTitle) > maxEventTitleLen {
		// An untitled event is a coloured block nobody can read.
		return SchoolEvent{}, glerrors.New(glerrors.KindInvalid, "an event needs a title")
	}
	trimmedDesc := strings.TrimSpace(description)
	if len(trimmedDesc) > maxEventDescriptionLen {
		return SchoolEvent{}, glerrors.New(glerrors.KindInvalid, "event description is too long")
	}
	if strings.TrimSpace(audience.LevelID) != "" && strings.TrimSpace(audience.GroupID) != "" {
		return SchoolEvent{}, glerrors.New(glerrors.KindInvalid,
			"an event is for a level or for a group, not both")
	}

	f, t := midnight(from), midnight(to)
	if t.Before(f) {
		return SchoolEvent{}, glerrors.New(glerrors.KindInvalid, "an event cannot end before it starts")
	}

	return SchoolEvent{
		id: id, tenantID: tenantID, cycleID: cycleID,
		title: trimmedTitle, description: trimmedDesc, from: f, to: t,
		audience: EventAudience{
			LevelID: strings.TrimSpace(audience.LevelID),
			GroupID: strings.TrimSpace(audience.GroupID),
		},
	}, nil
}

// ID returns the event's identifier.
func (e SchoolEvent) ID() string { return e.id }

// TenantID returns the owning tenant.
func (e SchoolEvent) TenantID() string { return e.tenantID }

// CycleID is the school year the event belongs to.
func (e SchoolEvent) CycleID() string { return e.cycleID }

// Title is what a person reads on the calendar.
func (e SchoolEvent) Title() string { return e.title }

// Description is the detail behind it: where, at what time, what to bring.
func (e SchoolEvent) Description() string { return e.description }

// From is the first day, inclusive.
func (e SchoolEvent) From() time.Time { return e.from }

// To is the last day, inclusive.
func (e SchoolEvent) To() time.Time { return e.to }

// Audience is who the event concerns.
func (e SchoolEvent) Audience() EventAudience { return e.audience }

// ForWholeSchool reports whether the event concerns everybody.
func (e SchoolEvent) ForWholeSchool() bool {
	return e.audience.LevelID == "" && e.audience.GroupID == ""
}
