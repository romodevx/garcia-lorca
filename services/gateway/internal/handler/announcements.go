package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// The audiences the school can address. Anything else is a typo, and a typo
// that reached "everybody" would be an expensive one.
const (
	scopeGroup = "group"
	scopeLevel = "level"
	scopeCycle = "cycle"
	// scopeStudent is one pupil: the note that goes to one family and nobody
	// else. A school sends more of these than it sends circulars.
	scopeStudent = "student"
	// scopeFamily is every current pupil of the named families (ADR 0041), only
	// while module_families is on.
	scopeFamily = "family"
)

// maxAnnouncementFamilies bounds one family announcement: each family is one
// students read, and fifty is far more than the office picks by hand.
const maxAnnouncementFamilies = 50

// The admission statuses that take a pupil out of a family audience: one who
// left, or was never admitted, is not a family's child at this school any more.
const (
	admissionWithdrawn = "withdrawn"
	admissionRejected  = "rejected"
)

// Announcer sends one message to many families. The audience is a list of
// pupils: comms does not know what a group is, on purpose.
type Announcer interface {
	Announce(ctx context.Context, studentIDs []string, title, body, idempotencyKey string) (int, error)
}

// EnrollmentReader resolves who is enrolled where.
type EnrollmentReader interface {
	ListEnrollments(ctx context.Context, studentID, cycleID, groupID string) ([]client.Enrollment, error)
}

// GroupReader walks a level down to its groups: level → grades → groups.
type GroupReader interface {
	ListGradeLevels(ctx context.Context, levelID string) ([]client.GradeLevelResult, error)
	ListGroups(ctx context.Context, cycleID, gradeLevelID string) ([]client.GroupResult, error)
}

// AnnouncementHandler turns "tell 1° A" into a list of pupils and hands it to
// comms.
//
// The resolution lives here rather than in comms because the edge already
// speaks to config and students. Teaching the notification service what a level
// is would give it a second reason to change — and the shape of a school's
// catalogue is exactly the thing that changes.
type AnnouncementHandler struct {
	comms       Announcer
	enrollments EnrollmentReader
	groups      GroupReader
	families    familyAudience
}

// AnnouncementOption configures optional audiences of the announcement edge.
type AnnouncementOption func(*AnnouncementHandler)

// familyAudience resolves the "family" scope while enabled says
// module_families is on. The zero value is off.
type familyAudience struct {
	enabled func(context.Context) bool
	pupils  FamilyStudentsLister
}

func (f familyAudience) on(ctx context.Context) bool {
	return f.enabled != nil && f.pupils != nil && f.enabled(ctx)
}

// WithFamilyAudience lets an announcement address families (ADR 0041) while
// enabled reports module_families on. Off, the scope is refused with 400 and
// every other scope is unchanged.
func WithFamilyAudience(enabled func(context.Context) bool, pupils FamilyStudentsLister) AnnouncementOption {
	return func(h *AnnouncementHandler) { h.families = familyAudience{enabled: enabled, pupils: pupils} }
}

// NewAnnouncementHandler wires the edge to comms plus the two readers it needs
// to work out an audience.
func NewAnnouncementHandler(comms Announcer, enrollments EnrollmentReader, groups GroupReader, opts ...AnnouncementOption) *AnnouncementHandler {
	h := &AnnouncementHandler{comms: comms, enrollments: enrollments, groups: groups}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

type announceRequest struct {
	// Scope is student | group | level | cycle | family.
	Scope string `json:"scope"`
	// TargetID is the group, level or pupil. Unused for a whole-cycle
	// announcement, and ignored when TargetIDs carries a list.
	TargetID string `json:"targetId"`
	// TargetIDs addresses SEVERAL groups at once — "1° A y 1° B" is one message
	// the school writes once, not two it writes twice and sends at different
	// times.
	TargetIDs      []string `json:"targetIds"`
	CycleID        string   `json:"cycleId"`
	Title          string   `json:"title"`
	Body           string   `json:"body"`
	IdempotencyKey string   `json:"idempotencyKey"`
}

// Announce handles POST /api/v1/comms/announcements.
func (h *AnnouncementHandler) Announce(w http.ResponseWriter, r *http.Request) {
	var req announceRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.CycleID) == "" {
		http.Error(w, "cycleId is required", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Body) == "" {
		http.Error(w, "an announcement needs a title and a message", http.StatusBadRequest)
		return
	}

	audience, err := h.audienceOf(r.Context(), req)
	if err != nil {
		writeAudienceError(w, err)
		return
	}
	if len(audience) == 0 {
		// A group that matched nobody. Reporting success here would let the
		// office believe a message went out that never did.
		http.Error(w, "nadie coincide con ese destino", http.StatusBadRequest)
		return
	}

	created, err := h.comms.Announce(r.Context(), audience, req.Title, req.Body, req.IdempotencyKey)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"created":  created,
		"audience": len(audience),
	})
}

// writeAudienceError answers an audience the request got wrong with 400 and
// its sentence; a service that did not answer is a downstream failure.
func writeAudienceError(w http.ResponseWriter, err error) {
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		writeConnectError(w, err)
		return
	}
	http.Error(w, err.Error(), http.StatusBadRequest)
}

// audienceOf turns a scope into the pupils it names.
func (h *AnnouncementHandler) audienceOf(ctx context.Context, req announceRequest) ([]string, error) {
	switch req.Scope {
	case scopeStudent:
		// No lookup: the caller named the pupil. Sending to one family is the
		// most common announcement there is.
		return targets(req), nil
	case scopeGroup:
		return h.studentsOfGroups(ctx, req.CycleID, targets(req))
	case scopeCycle:
		return h.studentsOfGroups(ctx, req.CycleID, nil)
	case scopeLevel:
		var groupIDs []string
		for _, levelID := range targets(req) {
			ids, err := h.groupsOfLevel(ctx, req.CycleID, levelID)
			if err != nil {
				return nil, err
			}
			groupIDs = append(groupIDs, ids...)
		}
		return h.studentsOfGroups(ctx, req.CycleID, groupIDs)
	case scopeFamily:
		return h.studentsOfFamilies(ctx, targets(req))
	default:
		return nil, errUnknownScope
	}
}

// targets is what the request addressed, as a list. One id and a list of ids
// are the same question asked two ways, and the rest of this file should not
// have to care which was used.
func targets(req announceRequest) []string {
	if len(req.TargetIDs) > 0 {
		return req.TargetIDs
	}
	if strings.TrimSpace(req.TargetID) == "" {
		return nil
	}
	return []string{req.TargetID}
}

// groupsOfLevel walks level → grades → groups. A level is not enrolled in
// directly; its pupils sit in the groups underneath it.
func (h *AnnouncementHandler) groupsOfLevel(ctx context.Context, cycleID, levelID string) ([]string, error) {
	grades, err := h.groups.ListGradeLevels(ctx, levelID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, grade := range grades {
		groups, gerr := h.groups.ListGroups(ctx, cycleID, grade.ID)
		if gerr != nil {
			return nil, gerr
		}
		for _, g := range groups {
			ids = append(ids, g.ID)
		}
	}
	return ids, nil
}

// studentsOfGroups collects the ACTIVE pupils of the given groups, or of the
// whole cycle when groupIDs is nil.
//
// Only active ones. A pupil who left mid-year is not in the group any more, and
// sending them the junta is how a school looks like it does not know who its
// pupils are.
func (h *AnnouncementHandler) studentsOfGroups(ctx context.Context, cycleID string, groupIDs []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	collect := func(rows []client.Enrollment) {
		for _, e := range rows {
			if e.Status != "active" || seen[e.StudentID] {
				continue
			}
			seen[e.StudentID] = true
			out = append(out, e.StudentID)
		}
	}

	if groupIDs == nil {
		rows, err := h.enrollments.ListEnrollments(ctx, "", cycleID, "")
		if err != nil {
			return nil, err
		}
		collect(rows)
		return out, nil
	}
	for _, groupID := range groupIDs {
		rows, err := h.enrollments.ListEnrollments(ctx, "", cycleID, groupID)
		if err != nil {
			return nil, err
		}
		collect(rows)
	}
	return out, nil
}

// studentsOfFamilies collects the current pupils of the given families, each
// once, in the order the families were named and, inside one, in the family's
// own order. A withdrawn or rejected pupil is left out, exactly as a group
// leaves out the pupil who left mid-year.
func (h *AnnouncementHandler) studentsOfFamilies(ctx context.Context, rawIDs []string) ([]string, error) {
	if !h.families.on(ctx) {
		return nil, errAudience("families are not enabled")
	}
	familyIDs, err := familyTargets(rawIDs)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, familyID := range familyIDs {
		pupils, lerr := h.families.pupils.ListStudentsInFamily(ctx, familyID, "")
		if lerr != nil {
			return nil, lerr
		}
		for _, p := range pupils {
			if p.AdmissionStatus == admissionWithdrawn || p.AdmissionStatus == admissionRejected || seen[p.ID] {
				continue
			}
			seen[p.ID] = true
			out = append(out, p.ID)
		}
	}
	return out, nil
}

// familyTargets validates and canonicalises the family ids before any read:
// at least one, at most maxAnnouncementFamilies, each a uuid.
func familyTargets(rawIDs []string) ([]string, error) {
	if len(rawIDs) == 0 {
		return nil, errAudience("targetIds must name at least one family")
	}
	if len(rawIDs) > maxAnnouncementFamilies {
		return nil, errAudience(fmt.Sprintf("an announcement addresses at most %d families", maxAnnouncementFamilies))
	}
	ids := make([]string, 0, len(rawIDs))
	for _, raw := range rawIDs {
		id, ok := canonicalUUID(strings.TrimSpace(raw))
		if !ok {
			return nil, errAudience("targetIds must be family ids")
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// errAudience is a request whose audience cannot be resolved as written.
type errAudience string

func (e errAudience) Error() string { return string(e) }

// errUnknownScope is a typo at the call site. It matters that it is refused:
// a scope nobody recognises must never fall through to "everybody".
var errUnknownScope = &scopeError{}

type scopeError struct{}

func (*scopeError) Error() string { return "scope must be student, group, level, cycle or family" }
