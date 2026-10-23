package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// ContactAccountIssuer is the iam surface this action needs. Consumer-defined,
// so the handler is testable without iam.
//
// InviteGuardian takes no role: iam picks the guardian role itself, and it is
// the ONLY road into that role — InviteUser refuses it (ISSUE-CC-115), so the
// staff screen cannot mint a family login that no pupil is linked to. It is
// keyed on the address and ENSURES the role: an account that already exists
// (a teacher who is also a parent) gains it and the invitation comes back nil.
type ContactAccountIssuer interface {
	InviteGuardian(ctx context.Context, email, displayName string, confirmStaff bool) (client.User, *client.Invitation, error)
}

// ContactGranter is the students surface: the pupil's contact list, and
// attaching an account to one of them.
type ContactGranter interface {
	GetStudent(ctx context.Context, id string) (client.StudentResult, error)
	ListGuardiansForStudent(ctx context.Context, studentID string) ([]client.GuardianLink, error)
	GrantContactAccess(ctx context.Context, contactID, studentID, guardianUserID string) (client.GuardianLink, error)
	// GetFamily reads the pupil's family, for the two-account cap (ADR 0041).
	GetFamily(ctx context.Context, familyID string) (client.Family, error)
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

// inviteRequest is the optional body of an invitation. confirmStaffAccount is
// the director's answer when iam said the address belongs to a staff account
// (C2-003); no body means "not confirmed".
type inviteRequest struct {
	ConfirmStaffAccount bool `json:"confirmStaffAccount"`
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
	var req inviteRequest
	if err := decodeJSONBody(w, r, &req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	links, err := h.students.ListGuardiansForStudent(r.Context(), studentID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	contact := findContact(links, contactID)
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
	pupil, ok := h.placeable(w, r.Context(), studentID)
	if !ok {
		return
	}
	if !h.familyHasRoom(w, r.Context(), pupil, links, email) {
		return
	}

	// A deployment missing migrations/iam/0010_guardians.sql has no guardian
	// role; iam refuses that as a server fault (5xx), before writing anything.
	user, invitation, err := h.accountFor(r.Context(), email, contact.FullName, req.ConfirmStaffAccount)
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

// codeStudentDraft is students' code for a draft pupil (PLAN-import-avisos).
const codeStudentDraft = "STUDENT_DRAFT"

// placeable refuses a draft pupil (imported without a CURP) BEFORE iam is
// asked. iam does not know the pupils, and students refuses the grant only
// after iam has minted the account and mailed the link — so the edge, which
// composes both, is where the refusal has to be. It holds with or without
// module_import_review: a draft is a state of the data, not of the screen.
func (h *ContactAccessHandler) placeable(w http.ResponseWriter, ctx context.Context, studentID string) (client.StudentResult, bool) {
	pupil, err := h.students.GetStudent(ctx, studentID)
	if err != nil {
		writeConnectError(w, err)
		return client.StudentResult{}, false
	}
	if pupil.Draft || pupil.AdmissionStatus == admissionDraft {
		writeCodedError(w, http.StatusConflict, codeStudentDraft,
			"ese alumno es un borrador sin CURP: captura su CURP antes de invitar a su familia")
		return client.StudentResult{}, false
	}
	return pupil, true
}

// codeFamilyAccountLimit is students' code for a third account in a family.
const codeFamilyAccountLimit = "FAMILY_ACCOUNT_LIMIT"

// maxFamilyAccounts mirrors the students service's cap (ADR 0041).
const maxFamilyAccounts = 2

// familyHasRoom refuses a third account in the pupil's family BEFORE iam is
// asked. students refuses the grant too, but only after iam has minted the
// account and mailed the link. It holds whatever module_families says: the
// cap is a data rule, and students stays the authority.
//
// An address one of the pupil's account-bearing contacts already carries is
// one of the family's accounts (the sync links every account to every
// placeable sibling), so granting it is not a third one: that is left to
// students. A family gone between the two reads leaves nothing to cap.
func (h *ContactAccessHandler) familyHasRoom(w http.ResponseWriter, ctx context.Context, pupil client.StudentResult, links []client.GuardianLink, email string) bool {
	if pupil.FamilyID == "" || carriesAccountFor(links, email) {
		return true
	}
	family, err := h.students.GetFamily(ctx, pupil.FamilyID)
	if connect.CodeOf(err) == connect.CodeNotFound {
		return true
	}
	if err != nil {
		// Fail safe: when the cap cannot be checked, the invitation is NOT
		// sent. Asking iam anyway could mail a link for a grant students
		// would then refuse; a retry once students answers costs nothing.
		writeConnectError(w, err)
		return false
	}
	if family.AccountCount >= maxFamilyAccounts {
		writeCodedError(w, http.StatusConflict, codeFamilyAccountLimit,
			"esta familia ya tiene 2 cuentas de la app: quita el acceso de una antes de dar otro")
		return false
	}
	return true
}

// carriesAccountFor reports whether a contact with an account already carries
// this address.
func carriesAccountFor(links []client.GuardianLink, email string) bool {
	want := normalEmail(email)
	for _, l := range links {
		if l.GuardianUserID != "" && normalEmail(l.Email) == want {
			return true
		}
	}
	return false
}

// findContact returns the pupil's contact with this id, or nil when the pupil
// does not have one. Searching the pupil's OWN list is what makes the
// ownership check unavoidable: there is no path here that reaches a contact by
// id alone.
func findContact(links []client.GuardianLink, contactID string) *client.GuardianLink {
	for i := range links {
		if links[i].ID == contactID {
			return &links[i]
		}
	}
	return nil
}

// accountFor returns the iam account for this address holding the guardian
// role. iam does the matching, so an existing account is REUSED rather than
// duplicated — a parent with two children, or a teacher who is also a parent,
// has one login — and it is iam that adds the role to it. Reusing the account
// here, at the edge, used to skip that: the account was linked to the pupil
// without the role, and with the manual roads closed it could never open the
// app.
//
// The returned invitation is nil when the address already had an account:
// nothing was sent.
func (h *ContactAccessHandler) accountFor(ctx context.Context, email, displayName string, confirmStaff bool) (client.User, *invitationResponse, error) {
	user, invitation, err := h.iam.InviteGuardian(ctx, email, displayName, confirmStaff)
	if err != nil || invitation == nil {
		return user, nil, err
	}
	res := toInvitationResponse(*invitation)
	return user, &res, nil
}
