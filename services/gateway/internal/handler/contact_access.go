package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// ContactAccountIssuer is the iam surface this action needs: which accounts
// already exist, and how to invite a guardian. Consumer-defined, so the handler
// is testable without iam.
//
// InviteGuardian takes no role: iam picks the guardian role itself, and it is
// the ONLY road into that role — InviteUser refuses it (ISSUE-CC-115), so the
// staff screen cannot mint a family login that no pupil is linked to.
type ContactAccountIssuer interface {
	ListUsers(ctx context.Context) ([]client.User, error)
	InviteGuardian(ctx context.Context, email, displayName string) (client.User, client.Invitation, error)
}

// ContactGranter is the students surface: the pupil's contact list, and
// attaching an account to one of them.
type ContactGranter interface {
	ListGuardiansForStudent(ctx context.Context, studentID string) ([]client.GuardianLink, error)
	GrantContactAccess(ctx context.Context, contactID, studentID, guardianUserID string) (client.GuardianLink, error)
}

// ContactAccessHandler gives a family the app.
//
// WHY THIS EXISTS AT ALL. It is the ONE road to a family login. Creating the
// account by hand with the `guardian` role is refused by iam on every manual
// road — InviteUser, CreateUser, AssignRole (ISSUE-CC-115) — because a guardian
// account means nothing without the link to a pupil that this handler makes
// right after. iam's InviteGuardian picks the role itself; the edge never names
// it. (The owner holds `family:read` so the anti-escalation guard lets it
// delegate the role at all, ADR 0012.)
//
// WHY IT COMPOSES RATHER THAN PROXIES. "Dar acceso a la app" is one action for
// the school and two services underneath: iam owns accounts, students owns who a
// pupil's contacts are. The edge is where this repo already composes across
// services (see family.go and announcements.go), and it is the only place that
// holds both clients.
//
// AUTHORIZATION. The route runs behind the students module guard plus
// `guardians:write` — the same permission as linking a guardian by hand, because
// it decides the same thing: who may see a child.
type ContactAccessHandler struct {
	iam      ContactAccountIssuer
	students ContactGranter
}

// NewContactAccessHandler wires the edge to iam and students.
func NewContactAccessHandler(iam ContactAccountIssuer, students ContactGranter) *ContactAccessHandler {
	return &ContactAccessHandler{iam: iam, students: students}
}

type contactAccessResponse struct {
	Contact guardianLinkResponse `json:"contact"`
	// Invitation is nil when the address already had an account: the mother of
	// two children signs in once for both, and there was nothing to send.
	Invitation *invitationResponse `json:"invitation"`
}

// Invite handles POST /api/v1/students/{studentId}/guardians/{contactId}/invite.
//
// ORDER OF OPERATIONS. Every refusal that can be decided from the contact alone
// happens BEFORE iam is asked, so a rejected request leaves no account behind.
// After that the order is forced: the grant needs a user id, so the account must
// exist first.
//
// THE WINDOW THAT LEAVES. If the grant fails after the invitation succeeded, an
// iam account exists that no contact points at. It is not orphaned for long:
// this endpoint reuses an account by address, so pressing the button again
// attaches the one already made instead of minting a second. A retry heals it,
// which is why the account is matched on email rather than created blindly.
func (h *ContactAccessHandler) Invite(w http.ResponseWriter, r *http.Request) {
	studentID := r.PathValue("studentId")
	contactID := r.PathValue("contactId")

	contact, err := h.findContact(r.Context(), studentID, contactID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if contact == nil {
		// Not found, deliberately, for a contact of another pupil as much as for
		// one that does not exist: a 403 would confirm it is there.
		http.Error(w, "contact not found", http.StatusNotFound)
		return
	}
	if contact.GuardianUserID != "" {
		http.Error(w, "this contact can already open the app", http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(contact.Email)
	if email == "" {
		http.Error(w, "this contact has no email to send the invitation to", http.StatusBadRequest)
		return
	}

	// A deployment missing migrations/iam/0010_guardians.sql has no guardian
	// role; iam refuses that as a server fault (5xx), before writing anything.
	user, invitation, err := h.accountFor(r.Context(), email, contact.FullName)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	granted, err := h.students.GrantContactAccess(r.Context(), contactID, studentID, user.ID)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, contactAccessResponse{
		Contact:    toGuardianLinkResponse(granted),
		Invitation: invitation,
	})
}

// findContact returns the pupil's contact with this id, or nil when the pupil
// does not have one. Reading the pupil's OWN list is what makes the ownership
// check unavoidable: there is no path here that reaches a contact by id alone.
func (h *ContactAccessHandler) findContact(ctx context.Context, studentID, contactID string) (*client.GuardianLink, error) {
	links, err := h.students.ListGuardiansForStudent(ctx, studentID)
	if err != nil {
		return nil, err
	}
	for i := range links {
		if links[i].ID == contactID {
			return &links[i], nil
		}
	}
	return nil, nil
}

// accountFor returns the iam account for this address, inviting one if there is
// none. An existing account is REUSED rather than duplicated: a parent with two
// children at the school has one login, and a second account for the same
// address would be one they never open and the school cannot tell apart.
//
// The returned invitation is nil in that case — nothing was sent.
func (h *ContactAccessHandler) accountFor(ctx context.Context, email, displayName string) (client.User, *invitationResponse, error) {
	if existing, found, err := userByEmail(ctx, h.iam, email); err != nil || found {
		return existing, nil, err
	}

	user, invitation, err := h.iam.InviteGuardian(ctx, email, displayName)
	if err != nil {
		return client.User{}, nil, err
	}
	res := toInvitationResponse(invitation)
	return user, &res, nil
}
