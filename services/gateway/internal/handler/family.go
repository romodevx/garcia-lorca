package handler

import (
	"context"
	"net/http"

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
// belongs to. A guardian with no linked children gets an empty list.
//
// It must run behind RequireAuth (which injects the principal) plus the comms
// module guard and RequirePermission("family:read"). One child's inbox failing
// fails the whole read: a partial list would read as "your child has no news",
// which is worse than an error.
//
// The fan-out is sequential on purpose — a guardian has a handful of children, so
// the round-trips are few and the child order stays stable and reproducible.
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
	out := make([]notificationResponse, 0, len(links))
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
			out = append(out, res)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": out})
}
