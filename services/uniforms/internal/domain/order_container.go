package domain

import (
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// OrderSource says where an order came from, and it is load-bearing rather than
// decorative: an anonymous sale is only legitimate at the counter.
type OrderSource string

const (
	SourceFamily  OrderSource = "family"
	SourceCounter OrderSource = "counter"
	SourceStaff   OrderSource = "staff"
)

// IsValid reports whether s is a source this system issues.
func (s OrderSource) IsValid() bool {
	return s == SourceFamily || s == SourceCounter || s == SourceStaff
}

// OrderStudent is who the order is for, captured at the moment it is placed.
//
// These are SNAPSHOTS on purpose. The panel filters by level and searches by
// name on every page load, and resolving that against the students service per
// row would make the busiest screen of the busiest fortnight depend on another
// service being awake. A pupil moved to another level afterwards keeps the old
// label on old orders, which is the correct reading of history anyway.
//
// The zero value is the anonymous counter sale: an uncle, an alumnus, someone
// who is not enrolled.
type OrderStudent struct {
	ID         string
	FullName   string
	Level      Level
	LevelName  string // the level's name as the school wrote it, at order time
	GradeGroup string
	CycleID    string
}

// Order is the container: what a family asked for.
//
// It has NO status of its own. Its label is derived from its lines and
// deliveries on every read (see DeriveOrderLabel), because a stored copy would
// be a second truth to keep in step with the first — and the first is what the
// stock ledger and the money actually follow.
type Order struct {
	id              string
	tenantID        string
	folio           string
	student         OrderStudent
	source          OrderSource
	createdByUserID string
	createdAt       time.Time
	closedAt        time.Time
}

const maxGradeGroupLen = 40

// NewOrder opens an order.
func NewOrder(id, tenantID, folio string, student OrderStudent, source OrderSource, createdByUserID string, at time.Time) (Order, error) {
	if strings.TrimSpace(id) == "" {
		return Order{}, glerrors.New(glerrors.KindInvalid, "order id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return Order{}, glerrors.New(glerrors.KindInvalid, "tenant id is required")
	}
	if !LooksLikeFolio(folio) {
		return Order{}, glerrors.New(glerrors.KindInvalid, "order folio is malformed")
	}
	if !source.IsValid() {
		return Order{}, glerrors.New(glerrors.KindInvalid, "unknown order source")
	}
	if at.IsZero() {
		return Order{}, glerrors.New(glerrors.KindInvalid, "the time the order was placed is required")
	}

	anonymous := strings.TrimSpace(student.ID) == ""
	if anonymous {
		// Only the counter can sell to someone who is not a pupil. A family-app
		// order that reached here without a child would create an obligation
		// nobody owns.
		if source != SourceCounter {
			return Order{}, glerrors.New(glerrors.KindInvalid, "only a counter sale may have no student")
		}
		if strings.TrimSpace(student.CycleID) != "" {
			return Order{}, glerrors.New(glerrors.KindInvalid, "a sale with no student cannot claim a cycle")
		}
		student = OrderStudent{}
	} else {
		// A charge has to belong to a school year, so a named pupil brings one.
		if strings.TrimSpace(student.CycleID) == "" {
			return Order{}, glerrors.New(glerrors.KindInvalid, "an order for a student needs the cycle it belongs to")
		}
		name := strings.TrimSpace(student.FullName)
		if name == "" || len(name) > 200 {
			return Order{}, glerrors.New(glerrors.KindInvalid, "the student's name is required")
		}
		group := strings.TrimSpace(student.GradeGroup)
		if len(group) > maxGradeGroupLen {
			return Order{}, glerrors.New(glerrors.KindInvalid, "grade and group is too long")
		}
		student.FullName = name
		student.LevelName = strings.TrimSpace(student.LevelName)
		student.GradeGroup = group
	}

	return Order{
		id: id, tenantID: tenantID, folio: strings.ToUpper(strings.TrimSpace(folio)),
		student: student, source: source, createdByUserID: createdByUserID, createdAt: at,
	}, nil
}

// IsAnonymous reports the counter sale to someone who is not a pupil. Such a sale
// still moves stock and still takes money — it is charged to the school's own
// counter account so the till reconciles — but it lands on nobody's statement.
func (o Order) IsAnonymous() bool { return o.student.ID == "" }

// Close stamps the order as finished.
func (o Order) Close(at time.Time) Order {
	o.closedAt = at
	return o
}

// Accessors.
func (o Order) ID() string                { return o.id }
func (o Order) TenantID() string          { return o.tenantID }
func (o Order) Folio() string             { return o.folio }
func (o Order) StudentID() string         { return o.student.ID }
func (o Order) StudentName() string       { return o.student.FullName }
func (o Order) StudentLevel() Level       { return o.student.Level }
func (o Order) StudentLevelName() string  { return o.student.LevelName }
func (o Order) StudentGradeGroup() string { return o.student.GradeGroup }
func (o Order) CycleID() string           { return o.student.CycleID }
func (o Order) Source() OrderSource       { return o.source }
func (o Order) CreatedByUserID() string   { return o.createdByUserID }
func (o Order) CreatedAt() time.Time      { return o.createdAt }
func (o Order) ClosedAt() time.Time       { return o.closedAt }
