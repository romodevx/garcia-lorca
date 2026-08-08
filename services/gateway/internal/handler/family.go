package handler

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// GuardianLinkReader resolves the students one guardian is linked to. It is the
// scoping primitive of the family surface (consumer-defined; the Connect
// StudentsClient implements it).
type GuardianLinkReader interface {
	ListStudentsForGuardian(ctx context.Context, guardianUserID string) ([]client.GuardianLink, error)
}

// NotificationReader reads one student's notification inbox (consumer-defined;
// the Connect CommsClient implements it).
type NotificationReader interface {
	ListNotifications(ctx context.Context, studentID string, unreadOnly bool) ([]client.Notification, error)
}

// FamilyHandler is the SELF-SCOPED family surface: a guardian reading their own
// children's news. It owns no service of its own — it composes the students
// guardian links with the comms inbox.
//
// The scoping is BY CONSTRUCTION, not by a check: the guardian id comes from the
// authenticated principal, so the set of readable students is whatever the
// caller is linked to and nothing else. No request field (body, query or path)
// can widen it. This is what makes /family/* safe for a parent-facing app, in
// contrast to the admin /comms/notifications?studentId=… route, which reads any
// student by id and therefore stays owner-only.
type FamilyHandler struct {
	links GuardianLinkReader
	inbox NotificationReader
}

// NewFamilyHandler wires the handler to the guardian-link reader (students) and
// the notification reader (comms).
func NewFamilyHandler(links GuardianLinkReader, inbox NotificationReader) *FamilyHandler {
	return &FamilyHandler{links: links, inbox: inbox}
}

// Notifications handles GET /api/v1/family/notifications?unreadOnly=true — every
// notification of every child the CALLER is linked to, tagged with the child it
// belongs to, newest first. A guardian with no linked children gets an empty list.
//
// It must run behind RequireAuth (which injects the principal) plus the module
// guards of BOTH modules it composes (comms and students) and
// RequirePermission("family:read"). One child's inbox failing fails the whole
// read: a partial list would read as "your child has no news", which is worse
// than an error.
//
// The fan-out is sequential on purpose — a guardian has a handful of children, so
// the round-trips are few and the child order stays stable and reproducible. That
// fan-out is child-major, so the aggregate is re-sorted before it is written: the
// family app shows one chronological stream, not one block per child.
func (h *FamilyHandler) Notifications(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	links, err := h.links.ListStudentsForGuardian(r.Context(), p.UserID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	unreadOnly := r.URL.Query().Get("unreadOnly") == unreadOnlyTrue
	dated := make([]datedNotification, 0, len(links))
	seen := make(map[string]bool, len(links))
	for _, link := range links {
		if seen[link.StudentID] {
			continue // a duplicate link must not duplicate the child's news
		}
		seen[link.StudentID] = true
		notifications, err := h.inbox.ListNotifications(r.Context(), link.StudentID, unreadOnly)
		if err != nil {
			writeConnectError(w, err)
			return
		}
		for _, n := range notifications {
			res := toNotificationResponse(n)
			if res.StudentID == "" {
				res.StudentID = link.StudentID // always tagged, even if comms omits it
			}
			dated = append(dated, newDatedNotification(res))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": newestFirst(dated)})
}

// datedNotification pairs one rendered notification with its parsed creation
// instant, so the aggregate is sorted without re-parsing RFC3339 per comparison.
type datedNotification struct {
	res     notificationResponse
	created time.Time
}

// newDatedNotification parses CreatedAt once. An absent or unparseable instant
// yields the zero time, which sorts last — a notification with no usable date
// belongs at the bottom of the stream, not the top.
func newDatedNotification(res notificationResponse) datedNotification {
	created, err := time.Parse(time.RFC3339, res.CreatedAt)
	if err != nil {
		return datedNotification{res: res}
	}
	return datedNotification{res: res, created: created}
}

// newestFirst orders the aggregated inbox by creation instant, descending, and
// projects it back to the response shape. The sort is stable so notifications
// created at the same instant keep the fan-out's child order, making the response
// reproducible. The result is never nil, so the JSON carries [] rather than null.
func newestFirst(dated []datedNotification) []notificationResponse {
	sort.SliceStable(dated, func(i, j int) bool {
		return dated[i].created.After(dated[j].created)
	})
	out := make([]notificationResponse, 0, len(dated))
	for _, d := range dated {
		out = append(out, d.res)
	}
	return out
}
