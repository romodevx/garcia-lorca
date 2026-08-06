package handler

import (
	"context"
	"net/http"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// AdminService is the iam admin surface the edge needs (consumer-defined, so the
// handler is testable with a fake). The Connect AuthClient implements it. Role
// editing is expressed as AssignRole/RevokeRole because iam exposes no bulk
// setter for an existing user's roles — the handler diffs the desired set
// against the current one (from GetUser) and applies the delta.
type AdminService interface {
	CreateUser(ctx context.Context, email, displayName, tempPassword string, roleIDs []string) (client.User, error)
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
	return userResponse{
		ID:                 u.ID,
		Email:              u.Email,
		DisplayName:        u.DisplayName,
		Status:             u.Status,
		MustChangePassword: u.MustChangePassword,
		RoleIDs:            u.RoleIDs,
	}
}

type roleResponse struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

func toRoleResponse(r client.Role) roleResponse {
	return roleResponse{ID: r.ID, Name: r.Name, Permissions: r.Permissions}
}

// --- users ----------------------------------------------------------------

type createUserRequest struct {
	Email        string   `json:"email"`
	DisplayName  string   `json:"displayName"`
	TempPassword string   `json:"tempPassword"`
	RoleIDs      []string `json:"roleIds"`
}

// CreateUser handles POST /api/v1/users: provisions an identity with a temporary
// password (the user must rotate it at first login) and the given roles.
func (h *AdminHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	res, err := h.svc.CreateUser(r.Context(), req.Email, req.DisplayName, req.TempPassword, req.RoleIDs)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toUserResponse(res))
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

// LinkTeacherUser handles POST /api/v1/config/teachers/{id}/user. It is the
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
