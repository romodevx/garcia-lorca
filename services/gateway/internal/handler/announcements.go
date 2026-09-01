package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// The audiences the school can address. Anything else is a typo, and a typo
// that reached "everybody" would be an expensive one.
const (
	scopeGroup = "group"
	scopeLevel = "level"
	scopeCycle = "cycle"
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
}

// NewAnnouncementHandler wires the edge to comms plus the two readers it needs
// to work out an audience.
func NewAnnouncementHandler(comms Announcer, enrollments EnrollmentReader, groups GroupReader) *AnnouncementHandler {
	return &AnnouncementHandler{comms: comms, enrollments: enrollments, groups: groups}
}

type announceRequest struct {
	// Scope is group | level | cycle.
	Scope string `json:"scope"`
	// TargetID is the group or level. Unused for a whole-cycle announcement.
	TargetID       string `json:"targetId"`
	CycleID        string `json:"cycleId"`
	Title          string `json:"title"`
	Body           string `json:"body"`
	IdempotencyKey string `json:"idempotencyKey"`
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
		http.Error(w, err.Error(), http.StatusBadRequest)
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

// audienceOf turns a scope into the pupils it names.
func (h *AnnouncementHandler) audienceOf(ctx context.Context, req announceRequest) ([]string, error) {
	switch req.Scope {
	case scopeGroup:
		return h.studentsOfGroups(ctx, req.CycleID, []string{req.TargetID})
	case scopeCycle:
		return h.studentsOfGroups(ctx, req.CycleID, nil)
	case scopeLevel:
		groupIDs, err := h.groupsOfLevel(ctx, req.CycleID, req.TargetID)
		if err != nil {
			return nil, err
		}
		return h.studentsOfGroups(ctx, req.CycleID, groupIDs)
	default:
		return nil, errUnknownScope
	}
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

// errUnknownScope is a typo at the call site. It matters that it is refused:
// a scope nobody recognises must never fall through to "everybody".
var errUnknownScope = &scopeError{}

type scopeError struct{}

func (*scopeError) Error() string { return "scope must be group, level or cycle" }
