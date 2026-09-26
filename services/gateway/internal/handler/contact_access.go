package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// guardianRoleName is the iam role that lets somebody open the family app. It
// carries exactly `family:read`; the per-child scoping is the guardian↔student
// link, not the permission (ADR 0004).
const guardianRoleName = "guardian"

// ContactAccountIssuer is the iam surface this action needs: which roles exist,
// which accounts already do, and how to invite a new one. Consumer-defined, so
// the handler is testable without iam.
type ContactAccountIssuer interface {
	ListRoles(ctx context.Context) ([]client.Role, error)
	ListUsers(ctx context.Context) ([]client.User, error)
	InviteUser(ctx context.Context, email, displayName string, roleIDs []string) (client.User, client.Invitation, error)
}

// ContactGranter is the students surface: the pupil's contact list, and
// attaching an account to one of them.
type ContactGranter interface {
	ListGuardiansForStudent(ctx context.Context, studentID string) ([]client.GuardianLink, error)
	GrantContactAccess(ctx context.Context, contactID, studentID, guardianUserID string) (client.GuardianLink, error)
}

// ContactAccessHandler gives a family the app.
//
// WHY THIS EXISTS AT ALL. Until now there was no way to do it. Creating the
// account by hand and assigning the `guardian` role is refused for everybody by
// the anti-escalation guard on CreateUser/InviteUser — a caller may only assign
// roles whose permissions it holds, and `family:read` was held by nobody. The
// fix was to give the owner that permission (ADR 0012), not to weaken the guard;
// this handler is what that unblocks.
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

	roleID, err := h.guardianRoleID(r.Context())
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if roleID == "" {
		// The deployment is missing migrations/iam/0010_guardians.sql. Inviting
		// into no role would create a login that can see nothing, so refuse —
		// and refuse as OUR fault, not the caller's.
		http.Error(w, "the guardian role is missing from this deployment", http.StatusInternalServerError)
		return
	}

	user, invitation, err := h.accountFor(r.Context(), email, contact.FullName, roleID)
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

// guardianRoleID resolves the guardian role by NAME rather than hard-coding the
// uuid the migration seeds: the id is an implementation detail of a migration,
// and a deployment that ever reseeds it would silently start inviting nobody
// into nothing.
func (h *ContactAccessHandler) guardianRoleID(ctx context.Context) (string, error) {
	roles, err := h.iam.ListRoles(ctx)
	if err != nil {
		return "", err
	}
	role, _ := findRoleByName(roles, guardianRoleName)
	return role.ID, nil
}

// accountFor returns the iam account for this address, inviting one if there is
// none. An existing account is REUSED rather than duplicated: a parent with two
// children at the school has one login, and a second account for the same
// address would be one they never open and the school cannot tell apart.
//
// The returned invitation is nil in that case — nothing was sent.
func (h *ContactAccessHandler) accountFor(ctx context.Context, email, displayName, roleID string) (client.User, *invitationResponse, error) {
	if existing, found, err := userByEmail(ctx, h.iam, email); err != nil || found {
		return existing, nil, err
	}

	user, invitation, err := h.iam.InviteUser(ctx, email, displayName, []string{roleID})
	if err != nil {
		return client.User{}, nil, err
	}
	res := toInvitationResponse(invitation)
	return user, &res, nil
}
