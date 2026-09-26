package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// AdminService is the iam admin surface the edge needs (consumer-defined, so the
// handler is testable with a fake). The Connect AuthClient implements it. Role
// editing is expressed as AssignRole/RevokeRole because iam exposes no bulk
// setter for an existing user's roles — the handler diffs the desired set
// against the current one (from GetUser) and applies the delta.
type AdminService interface {
	CreateUser(ctx context.Context, email, displayName, tempPassword string, roleIDs []string) (client.User, error)
	InviteUser(ctx context.Context, email, displayName string, roleIDs []string) (client.User, client.Invitation, error)
	ResendInvitation(ctx context.Context, userID string) (client.Invitation, error)
	GetUser(ctx context.Context, id string) (client.User, error)
	ListUsers(ctx context.Context) ([]client.User, error)
	SetUserStatus(ctx context.Context, userID, status string) (client.User, error)
	AssignRole(ctx context.Context, userID, roleID string) error
	RevokeRole(ctx context.Context, userID, roleID string) error
	CreateRole(ctx context.Context, name string, perms []string) (client.Role, error)
	ListRoles(ctx context.Context) ([]client.Role, error)
	SetRolePermissions(ctx context.Context, roleID string, perms []string) (client.Role, error)
}

// TeacherLinker is the config surface the teacher↔user link needs. The Connect
// ConfigClient implements it. The config service only stores the id; the gateway
// is responsible for validating (via iam) that the user exists first.
type TeacherLinker interface {
	SetTeacherUser(ctx context.Context, teacherID, userID string) (client.Teacher, error)
}

// AdminHandler exposes the front-facing (HTTP/JSON) user/role admin endpoints,
// proxying to iam, plus the gateway-orchestrated teacher↔user link (iam
// validate + config store). Every user/role route must run behind the module
// guard (module_iam) + RequireAuth + rotation-enforcement, with users:* on the
// user routes and roles:* on the role routes; the link route sits under
// module_config + config:write (it mutates a config teacher).
type AdminHandler struct {
	svc    AdminService
	linker TeacherLinker
}

// NewAdminHandler wires the handler to the iam admin service and the config
// teacher linker.
func NewAdminHandler(svc AdminService, linker TeacherLinker) *AdminHandler {
	return &AdminHandler{svc: svc, linker: linker}
}

// --- caller-subset authorization guard ------------------------------------
//
// Delegation rule: an admin may only grant, or assign via a role, permissions it
// already holds itself. holding roles:write/users:write lets you administer the
// permission model, NOT hand yourself powers your own account was never given.
// Without this, such an admin could mint or assign a role carrying finance:read
// (or any owner-only read) and escalate — the very outcome iam's owner guard is
// the last-resort backstop for. The caller's effective permission set only
// exists here at the edge (from the verified token's principal), so the check
// lives at the edge; iam remains the defense-in-depth backstop.

// callerHeld is the authenticated caller's effective permission set. These routes
// always run behind RequireAuth, so the principal is present; a missing principal
// yields the empty set, which fails every subset check closed.
func callerHeld(ctx context.Context) map[string]struct{} {
	p, _ := middleware.PrincipalFrom(ctx)
	held := make(map[string]struct{}, len(p.Permissions))
	for _, perm := range p.Permissions {
		held[perm] = struct{}{}
	}
	return held
}

// firstUnheld returns the first permission in want that held does not contain
// (and true), or "" and false when held is a superset of want.
func firstUnheld(held map[string]struct{}, want []string) (string, bool) {
	for _, perm := range want {
		if _, ok := held[perm]; !ok {
			return perm, true
		}
	}
	return "", false
}

// callerCanAssign reports whether the caller may assign every role in ids —
// i.e. holds every permission those roles carry. Role permissions come from a
// single ListRoles; an unknown id contributes nothing (iam validates existence).
// Returns the offending permission when the caller falls short.
func (h *AdminHandler) callerCanAssign(ctx context.Context, ids []string) (offending string, ok bool, err error) {
	if len(ids) == 0 {
		return "", true, nil
	}
	roles, err := h.svc.ListRoles(ctx)
	if err != nil {
		return "", false, err
	}
	if perm, missing := firstUnheldInRoles(callerHeld(ctx), roles, ids); missing {
		return perm, false, nil
	}
	return "", true, nil
}

// firstUnheldInRoles is the subset check over a set of role ids, against an
// already-fetched role catalog: the first permission any of those roles carries
// that held lacks. An id missing from the catalog contributes nothing.
func firstUnheldInRoles(held map[string]struct{}, roles []client.Role, ids []string) (string, bool) {
	permsByRole := make(map[string][]string, len(roles))
	for _, role := range roles {
		permsByRole[role.ID] = role.Permissions
	}
	for _, id := range ids {
		if perm, missing := firstUnheld(held, permsByRole[id]); missing {
			return perm, true
		}
	}
	return "", false
}

func forbidGrant(w http.ResponseWriter, perm string) {
	http.Error(w, "forbidden: cannot grant a permission you do not hold: "+perm, http.StatusForbidden)
}

func forbidAssign(w http.ResponseWriter, perm string) {
	http.Error(w, "forbidden: cannot assign a role granting a permission you do not hold: "+perm, http.StatusForbidden)
}

func forbidReach(w http.ResponseWriter, perm string) {
	http.Error(w, "forbidden: cannot issue access to an account holding a permission you do not hold: "+perm, http.StatusForbidden)
}

// --- wire shapes ----------------------------------------------------------

type userResponse struct {
	ID                 string   `json:"id"`
	Email              string   `json:"email"`
	DisplayName        string   `json:"displayName"`
	Status             string   `json:"status"`
	MustChangePassword bool     `json:"mustChangePassword"`
	RoleIDs            []string `json:"roleIds"`
}

func toUserResponse(u client.User) userResponse {
	// Never emit a null roleIds array: a user with no roles must serialize as []
	// so JSON clients can iterate it (a null crashed the web users panel, which
	// does user.roleIds.length / new Set(user.roleIds)).
	roleIDs := u.RoleIDs
	if roleIDs == nil {
		roleIDs = []string{}
	}
	return userResponse{
		ID:                 u.ID,
		Email:              u.Email,
		DisplayName:        u.DisplayName,
		Status:             u.Status,
		MustChangePassword: u.MustChangePassword,
		RoleIDs:            roleIDs,
	}
}

type roleResponse struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

func toRoleResponse(r client.Role) roleResponse {
	// Never emit a null permissions array: a role with no permissions must
	// serialize to [] so JSON clients can always iterate it (a null crashed the
	// web roles panel, which does role.permissions.length).
	perms := r.Permissions
	if perms == nil {
		perms = []string{}
	}
	return roleResponse{ID: r.ID, Name: r.Name, Permissions: perms}
}

// --- users ----------------------------------------------------------------

type createUserRequest struct {
	Email        string   `json:"email"`
	DisplayName  string   `json:"displayName"`
	TempPassword string   `json:"tempPassword"`
	RoleIDs      []string `json:"roleIds"`
}

// inviteUserRequest carries no password field, and that is the design: the
// account's owner chooses the first one through the emailed link.
type inviteUserRequest struct {
	Email       string   `json:"email"`
	DisplayName string   `json:"displayName"`
	RoleIDs     []string `json:"roleIds"`
}

// CreateUser handles POST /api/v1/users: provisions an identity with a temporary
// password (the user must rotate it at first login) and the given roles.
func (h *AdminHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	// Subset guard: the caller may only seed the new user with roles whose
	// permissions it holds — else it could create an owner-powered account (with
	// a temp password it chose) and log in as it.
	if perm, ok, err := h.callerCanAssign(r.Context(), req.RoleIDs); err != nil {
		writeConnectError(w, err)
		return
	} else if !ok {
		forbidAssign(w, perm)
		return
	}
	res, err := h.svc.CreateUser(r.Context(), req.Email, req.DisplayName, req.TempPassword, req.RoleIDs)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toUserResponse(res))
}

// InviteUser handles POST /api/v1/users/invite: create the account with no
// password and let the provider mail its owner the link that sets one.
//
// The subset guard matters MORE here than on CreateUser. An unchecked invite is
// a direct route to owner: send the invitation for an owner-powered account to
// an address you control, open the link, pick the password. So the caller may
// only invite into roles whose permissions it already holds, and a refusal must
// happen BEFORE iam is asked, so no account exists for a rejected request.
func (h *AdminHandler) InviteUser(w http.ResponseWriter, r *http.Request) {
	var req inviteUserRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if perm, ok, err := h.callerCanAssign(r.Context(), req.RoleIDs); err != nil {
		writeConnectError(w, err)
		return
	} else if !ok {
		forbidAssign(w, perm)
		return
	}
	res, inv, err := h.svc.InviteUser(r.Context(), req.Email, req.DisplayName, req.RoleIDs)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		userResponse
		Invitation invitationResponse `json:"invitation"`
	}{toUserResponse(res), toInvitationResponse(inv)})
}

// ResendInvitation handles POST /api/v1/users/{id}/invite: mint a fresh link
// for somebody who has not walked through the door yet.
//
// THE SUBSET GUARD APPLIES HERE, and the comment that used to sit in its place
// explained why it did not: "the link goes to the address the user row already
// carries, so the only person who can use it is that user." That was true while
// the route answered 204 and the message was the only way to reach the link. It
// stopped being true when the response started carrying the URL.
//
// What that opened: this endpoint does not GRANT a permission, it hands over an
// account — so it walked straight around the fence that keeps a `users:write`
// holder inside its own permission subset everywhere else. A delegated
// administrator could pull the access link of a principal whose powers it was
// never given, redeem it anonymously with a password of its choosing, and
// arrive as that person.
//
// So the same rule as assigning a role, for the same reason: you may only reach
// an account whose permissions you already hold.
func (h *AdminHandler) ResendInvitation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	target, err := h.svc.GetUser(r.Context(), id)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if perm, ok, err := h.callerCanAssign(r.Context(), target.RoleIDs); err != nil {
		writeConnectError(w, err)
		return
	} else if !ok {
		forbidReach(w, perm)
		return
	}

	inv, err := h.svc.ResendInvitation(r.Context(), id)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	// 200 with a body, not 204. The caller needs to know whether the message
	// actually left — and, when it did not, what link to hand over instead.
	// Answering "no content" was the shape that let an undelivered invitation
	// look exactly like a delivered one.
	writeJSON(w, http.StatusOK, toInvitationResponse(inv))
}

// invitationResponse is what the admin screen renders.
//
// `sent` and `sendError` exist so the director is never told "invitación
// enviada" about a message that was dropped, which is the bug this whole change
// is here to fix. `url` rides along only as the fallback for handing access
// over by hand; it grants nothing the administrator does not already hold over
// that account.
type invitationResponse struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expiresAt"`
	Sent      bool   `json:"sent"`
	SendError string `json:"sendError,omitempty"`
	Via       string `json:"via,omitempty"`
}

func toInvitationResponse(inv client.Invitation) invitationResponse {
	out := invitationResponse{Sent: inv.Sent, SendError: inv.SendError, Via: inv.Via}
	// The link comes back ONLY when nothing was delivered. That is the case it
	// exists for — an administrator who cannot mail somebody still needs a way
	// to hand access over. Once the message has gone out it buys nothing, and
	// every extra copy of a token is another place it can leak from.
	if !inv.Sent {
		out.URL = inv.URL
	}
	if !inv.ExpiresAt.IsZero() {
		out.ExpiresAt = inv.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return out
}

// ListUsers handles GET /api/v1/users.
func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.ListUsers(r.Context())
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]userResponse, 0, len(res))
	for _, u := range res {
		out = append(out, toUserResponse(u))
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

// GetUser handles GET /api/v1/users/{id}.
func (h *AdminHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.GetUser(r.Context(), r.PathValue("id"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(res))
}

type setUserStatusRequest struct {
	Status string `json:"status"`
}

// SetUserStatus handles POST /api/v1/users/{id}/status: suspend or reactivate.
func (h *AdminHandler) SetUserStatus(w http.ResponseWriter, r *http.Request) {
	var req setUserStatusRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	res, err := h.svc.SetUserStatus(r.Context(), r.PathValue("id"), req.Status)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(res))
}

type setUserRolesRequest struct {
	RoleIDs []string `json:"roleIds"`
}

// SetUserRoles handles POST /api/v1/users/{id}/roles. iam has no bulk setter for
// an existing user's roles, so the handler reads the current set (GetUser),
// diffs it against the requested set, and applies the delta: AssignRole for each
// added id, RevokeRole for each removed id. A missing user (GetUser NotFound)
// maps to 404 before any mutation.
func (h *AdminHandler) SetUserRoles(w http.ResponseWriter, r *http.Request) {
	var req setUserRolesRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	userID := r.PathValue("id")
	current, err := h.svc.GetUser(r.Context(), userID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	toAdd, toRemove := diffRoleSets(current.RoleIDs, req.RoleIDs)
	// Subset guard: only the newly ADDED roles need checking (revoking a role you
	// couldn't grant is de-escalation, always allowed). Checked before any
	// mutation so a rejected escalation leaves the user's roles untouched.
	if perm, ok, err := h.callerCanAssign(r.Context(), toAdd); err != nil {
		writeConnectError(w, err)
		return
	} else if !ok {
		forbidAssign(w, perm)
		return
	}
	for _, roleID := range toAdd {
		if err := h.svc.AssignRole(r.Context(), userID, roleID); err != nil {
			writeConnectError(w, err)
			return
		}
	}
	for _, roleID := range toRemove {
		if err := h.svc.RevokeRole(r.Context(), userID, roleID); err != nil {
			writeConnectError(w, err)
			return
		}
	}
	res, err := h.svc.GetUser(r.Context(), userID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(res))
}

// diffRoleSets returns the ids to add (in want, not in have) and to remove (in
// have, not in want), so an edit becomes the minimal set of AssignRole/RevokeRole
// calls.
func diffRoleSets(have, want []string) (toAdd, toRemove []string) {
	haveSet := make(map[string]struct{}, len(have))
	for _, id := range have {
		haveSet[id] = struct{}{}
	}
	wantSet := make(map[string]struct{}, len(want))
	for _, id := range want {
		wantSet[id] = struct{}{}
		if _, ok := haveSet[id]; !ok {
			toAdd = append(toAdd, id)
		}
	}
	for _, id := range have {
		if _, ok := wantSet[id]; !ok {
			toRemove = append(toRemove, id)
		}
	}
	return toAdd, toRemove
}

// --- roles ----------------------------------------------------------------

type createRoleRequest struct {
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

// CreateRole handles POST /api/v1/roles.
func (h *AdminHandler) CreateRole(w http.ResponseWriter, r *http.Request) {
	var req createRoleRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	// Subset guard: a role may only be built from permissions the caller holds.
	if perm, unheld := firstUnheld(callerHeld(r.Context()), req.Permissions); unheld {
		forbidGrant(w, perm)
		return
	}
	res, err := h.svc.CreateRole(r.Context(), req.Name, req.Permissions)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toRoleResponse(res))
}

// ListRoles handles GET /api/v1/roles.
func (h *AdminHandler) ListRoles(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.ListRoles(r.Context())
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]roleResponse, 0, len(res))
	for _, role := range res {
		out = append(out, toRoleResponse(role))
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": out})
}

type setRolePermissionsRequest struct {
	Permissions []string `json:"permissions"`
}

// SetRolePermissions handles POST /api/v1/roles/{id}/permissions.
func (h *AdminHandler) SetRolePermissions(w http.ResponseWriter, r *http.Request) {
	var req setRolePermissionsRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	// Subset guard: a role's new permission set may only contain permissions the
	// caller holds.
	if perm, unheld := firstUnheld(callerHeld(r.Context()), req.Permissions); unheld {
		forbidGrant(w, perm)
		return
	}
	res, err := h.svc.SetRolePermissions(r.Context(), r.PathValue("id"), req.Permissions)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toRoleResponse(res))
}

// --- teacher ↔ user link --------------------------------------------------

type linkTeacherUserRequest struct {
	UserID string `json:"userId"`
}

// TeacherUserLinkRoute is where LinkTeacherUser is served. It is shared with the
// composition root so the tests pin the path a client must call.
const TeacherUserLinkRoute = "POST /api/v1/teachers/{id}/user"

// LinkTeacherUser handles POST /api/v1/teachers/{id}/user. It is the
// gateway-orchestrated cross-service link: for a non-empty userId it FIRST
// validates the iam user exists (GetUser) — a missing user maps to 404 and no
// config write happens — THEN calls config SetTeacherUser to store the link. An
// empty userId unlinks and skips the validation.
func (h *AdminHandler) LinkTeacherUser(w http.ResponseWriter, r *http.Request) {
	var req linkTeacherUserRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.UserID != "" {
		if _, err := h.svc.GetUser(r.Context(), req.UserID); err != nil {
			writeConnectError(w, err)
			return
		}
	}
	res, err := h.linker.SetTeacherUser(r.Context(), r.PathValue("id"), req.UserID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toTeacherResponse(res))
}
