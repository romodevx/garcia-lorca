package handler

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// TeacherAccessRoute is where GrantAccess is served. Shared with main.go so the
// handler tests exercise the real pattern (and its {id} path value).
const TeacherAccessRoute = "POST /api/v1/teachers/{id}/access"

// maestroRoleName is the iam role a teacher signs in with (migrations/iam/0022).
// It carries exactly teaching:* and attendance_own:*; WHICH groups the teacher
// reaches is the principal→teacher→assignments rule, not the role.
const maestroRoleName = "maestro"

// userStatusInvited is the status iam names an account whose owner has not yet
// chosen a password.
const userStatusInvited = "invited"

// errTeacherHasNoEmail is the refusal a teacher with no address gets: there is
// nowhere to send the invitation, and the account is keyed by email.
const errTeacherHasNoEmail = "El docente no tiene correo"

// TeacherAccountIssuer is the iam surface "Dar acceso al sistema" needs.
// Consumer-defined, so the handler is testable without iam.
type TeacherAccountIssuer interface {
	ListRoles(ctx context.Context) ([]client.Role, error)
	ListUsers(ctx context.Context) ([]client.User, error)
	GetUser(ctx context.Context, id string) (client.User, error)
	InviteUser(ctx context.Context, email, displayName string, roleIDs []string) (client.User, client.Invitation, error)
	ResendInvitation(ctx context.Context, userID string) (client.Invitation, error)
	AssignRole(ctx context.Context, userID, roleID string) error
}

// TeacherAccessConfig is the config surface: the teacher, and its user link.
type TeacherAccessConfig interface {
	GetTeacher(ctx context.Context, id string) (client.Teacher, error)
	SetTeacherUser(ctx context.Context, teacherID, userID string) (client.Teacher, error)
}

// TeacherAccessHandler gives a teacher the system: one action for the school,
// iam (the account, in the maestro role) plus config (the teacher↔user link)
// underneath — the same composition ContactAccessHandler does for a family.
//
// AUTHORIZATION. The route runs behind module_teachers + auth + rotation +
// teachers:write AND users:write (it edits a teacher and mints an account). On
// top of that the caller-subset guard: the caller may only grant the maestro
// role if it holds every permission the role carries, and may only resend the
// link of an account whose roles it could have assigned (the reach guard of
// ResendInvitation — a resent link hands the account over).
type TeacherAccessHandler struct {
	iam    TeacherAccountIssuer
	config TeacherAccessConfig
}

// NewTeacherAccessHandler wires the edge to iam and config.
func NewTeacherAccessHandler(iam TeacherAccountIssuer, config TeacherAccessConfig) *TeacherAccessHandler {
	return &TeacherAccessHandler{iam: iam, config: config}
}

// teacherAccessResponse is the teacher after the action plus the account it
// signs in with. Invitation is nil when nothing was sent (the account is active
// or suspended); its url rides along only when the message was not delivered.
type teacherAccessResponse struct {
	Teacher    teacherResponse     `json:"teacher"`
	UserID     string              `json:"userId"`
	Email      string              `json:"email"`
	Status     string              `json:"status"`
	Invitation *invitationResponse `json:"invitation"`
}

// GrantAccess handles POST /api/v1/teachers/{id}/access.
//
// IDEMPOTENT. Pressing it again never mints a second account: a teacher already
// linked keeps its account, an address that already has one reuses it (and gets
// the maestro role if it lacked it), and an account still in "invited" gets a
// fresh link. Every refusal happens before the first write.
//
// THE WINDOW THAT LEAVES. If the link fails after the invitation succeeded, an
// iam account exists that no teacher points at. Pressing again heals it: the
// account is matched by address and linked.
func (h *TeacherAccessHandler) GrantAccess(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	roles, err := h.iam.ListRoles(ctx)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	maestro, ok := findRoleByName(roles, maestroRoleName)
	if !ok {
		// The deployment is missing migrations/iam/0022. Inviting into no role
		// would create a login that reaches nothing — our fault, not the caller's.
		http.Error(w, "the maestro role is missing from this deployment", http.StatusInternalServerError)
		return
	}
	held := callerHeld(ctx)
	if perm, missing := firstUnheld(held, maestro.Permissions); missing {
		forbidAssign(w, perm)
		return
	}

	teacher, err := h.config.GetTeacher(ctx, r.PathValue("id"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	email := strings.TrimSpace(teacher.Email)
	if teacher.UserID == "" && email == "" {
		http.Error(w, errTeacherHasNoEmail, http.StatusBadRequest)
		return
	}

	user, found, err := h.existingAccount(ctx, teacher, email)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	var invitation *invitationResponse
	if found {
		if user.Status == userStatusInvited {
			if perm, missing := firstUnheldInRoles(held, roles, user.RoleIDs); missing {
				forbidReach(w, perm)
				return
			}
		}
		invitation, err = h.refresh(ctx, user, maestro.ID)
	} else {
		user, invitation, err = h.invite(ctx, teacher, email, maestro.ID)
	}
	if err != nil {
		writeConnectError(w, err)
		return
	}

	if teacher.UserID == "" {
		if teacher, err = h.config.SetTeacherUser(ctx, teacher.ID, user.ID); err != nil {
			writeConnectError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, teacherAccessResponse{
		Teacher:    toTeacherResponse(teacher),
		UserID:     user.ID,
		Email:      user.Email,
		Status:     user.Status,
		Invitation: invitation,
	})
}

// existingAccount finds the account this teacher already has: the linked one,
// else the one registered under the teacher's address (case-insensitive — iam
// stores emails lowercased, a teacher record may not).
func (h *TeacherAccessHandler) existingAccount(ctx context.Context, teacher client.Teacher, email string) (client.User, bool, error) {
	if teacher.UserID != "" {
		user, err := h.iam.GetUser(ctx, teacher.UserID)
		if err != nil {
			return client.User{}, false, err
		}
		return user, true, nil
	}
	users, err := h.iam.ListUsers(ctx)
	if err != nil {
		return client.User{}, false, err
	}
	for _, u := range users {
		if strings.EqualFold(strings.TrimSpace(u.Email), email) {
			return u, true, nil
		}
	}
	return client.User{}, false, nil
}

// refresh brings an existing account up to date: the maestro role if it lacked
// it, and a fresh link if it never walked through the door. An active or
// suspended account gets no invitation (a suspension is lifted in Usuarios, not
// here).
func (h *TeacherAccessHandler) refresh(ctx context.Context, user client.User, maestroID string) (*invitationResponse, error) {
	if !slices.Contains(user.RoleIDs, maestroID) {
		if err := h.iam.AssignRole(ctx, user.ID, maestroID); err != nil {
			return nil, err
		}
	}
	if user.Status != userStatusInvited {
		return nil, nil
	}
	inv, err := h.iam.ResendInvitation(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	res := toInvitationResponse(inv)
	return &res, nil
}

// invite mints the teacher's account in the maestro role.
func (h *TeacherAccessHandler) invite(ctx context.Context, teacher client.Teacher, email, maestroID string) (client.User, *invitationResponse, error) {
	displayName := strings.TrimSpace(teacher.FirstName + " " + teacher.LastName)
	user, inv, err := h.iam.InviteUser(ctx, email, displayName, []string{maestroID})
	if err != nil {
		return client.User{}, nil, err
	}
	res := toInvitationResponse(inv)
	return user, &res, nil
}

// findRoleByName resolves a seeded role by NAME rather than its migration uuid,
// like the guardian role in contact_access.go.
func findRoleByName(roles []client.Role, name string) (client.Role, bool) {
	for _, role := range roles {
		if role.Name == name {
			return role, true
		}
	}
	return client.Role{}, false
}
