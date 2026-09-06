package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// ownerCallerPerms is a broad, valid permission set — the seeded owner holds the
// whole catalog, so a caller carrying these passes the "grant/assign only what
// you hold" subset guard. Tests that exercise the guard's DENY path inject a
// deliberately narrower principal instead.
var ownerCallerPerms = []string{
	"students:read", "students:create", "students:list", "students:update",
	"grades:read", "grades:write", "users:read", "users:write",
	"roles:read", "roles:write", "finance:read", "finance:write",
	"attendance:read", "attendance:write", "comms:read", "comms:write",
	"config:read", "config:write", "guardians:read", "guardians:write",
}

func principalHolding(perms ...string) middleware.Principal {
	return middleware.Principal{UserID: "caller-1", TenantID: "tenant-1", Email: "caller@example.com", Permissions: perms}
}

// fakeAdmin is a hand-written double for the iam admin surface the handler
// needs. It records inputs and returns canned results/errors so each handler is
// exercised in isolation (no Connect round-trip).
type fakeAdmin struct {
	createIn struct {
		email, display, temp string
		roleIDs              []string
	}
	statusUserID, statusValue string
	assigned, revoked         []string // "userID:roleID" pairs
	getUser                   client.User
	roleName                  string
	rolePerms                 []string
	setPermsRole              string
	setPerms                  []string
	rolesList                 []client.Role // when set, ListRoles returns this

	err        error
	getUserErr error
}

func (f *fakeAdmin) CreateUser(_ context.Context, email, displayName, tempPassword string, roleIDs []string) (client.User, error) {
	f.createIn.email, f.createIn.display, f.createIn.temp, f.createIn.roleIDs = email, displayName, tempPassword, roleIDs
	if f.err != nil {
		return client.User{}, f.err
	}
	return client.User{ID: "usr-1", Email: email, DisplayName: displayName, Status: "active", MustChangePassword: true, RoleIDs: roleIDs}, nil
}

// InviteUser is not exercised through this double — admin_invite_test.go uses
// its own, which records the call. Failing loudly here means a test that reaches
// the invite path by accident says so instead of quietly passing.
func (f *fakeAdmin) InviteUser(_ context.Context, _, _ string, _ []string) (client.User, error) {
	return client.User{}, errors.New("fakeAdmin: InviteUser not wired for this test")
}

func (f *fakeAdmin) ResendInvitation(_ context.Context, _ string) error {
	return errors.New("fakeAdmin: ResendInvitation not wired for this test")
}

func (f *fakeAdmin) GetUser(_ context.Context, id string) (client.User, error) {
	if f.getUserErr != nil {
		return client.User{}, f.getUserErr
	}
	u := f.getUser
	u.ID = id
	return u, nil
}

func (f *fakeAdmin) ListUsers(_ context.Context) ([]client.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	return []client.User{{ID: "usr-1", Email: "prof@example.com", Status: "active", RoleIDs: []string{"rol-1"}}}, nil
}

func (f *fakeAdmin) SetUserStatus(_ context.Context, userID, status string) (client.User, error) {
	f.statusUserID, f.statusValue = userID, status
	if f.err != nil {
		return client.User{}, f.err
	}
	return client.User{ID: userID, Status: status}, nil
}

func (f *fakeAdmin) AssignRole(_ context.Context, userID, roleID string) error {
	f.assigned = append(f.assigned, userID+":"+roleID)
	return f.err
}

func (f *fakeAdmin) RevokeRole(_ context.Context, userID, roleID string) error {
	f.revoked = append(f.revoked, userID+":"+roleID)
	return f.err
}

func (f *fakeAdmin) CreateRole(_ context.Context, name string, perms []string) (client.Role, error) {
	f.roleName, f.rolePerms = name, perms
	if f.err != nil {
		return client.Role{}, f.err
	}
	return client.Role{ID: "rol-1", Name: name, Permissions: perms}, nil
}

func (f *fakeAdmin) ListRoles(_ context.Context) ([]client.Role, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.rolesList != nil {
		return f.rolesList, nil
	}
	return []client.Role{{ID: "rol-1", Name: "docente", Permissions: []string{"grades:read"}}}, nil
}

func (f *fakeAdmin) SetRolePermissions(_ context.Context, roleID string, perms []string) (client.Role, error) {
	f.setPermsRole, f.setPerms = roleID, perms
	if f.err != nil {
		return client.Role{}, f.err
	}
	return client.Role{ID: roleID, Name: "docente", Permissions: perms}, nil
}

// fakeLinker is a double for the config teacher-link surface.
type fakeLinker struct {
	teacherID, userID string
	err               error
}

func (f *fakeLinker) SetTeacherUser(_ context.Context, teacherID, userID string) (client.Teacher, error) {
	f.teacherID, f.userID = teacherID, userID
	if f.err != nil {
		return client.Teacher{}, f.err
	}
	return client.Teacher{ID: teacherID, UserID: userID}, nil
}

func newAdminHandler(svc handler.AdminService, linker handler.TeacherLinker) *handler.AdminHandler {
	return handler.NewAdminHandler(svc, linker)
}

// doReq drives a handler directly. pathValues are key,value pairs bound as
// ServeMux path params (e.g. "id","usr-9"), since a directly-invoked handler
// receives no mux-populated PathValue otherwise.
func doAdminReq(t *testing.T, h http.Handler, method, target, body string, pathValues ...string) *httptest.ResponseRecorder {
	t.Helper()
	// Default caller is an owner-equivalent principal (holds the whole catalog),
	// so the subset guard is transparent to every test that is not exercising it.
	return adminReqAs(t, h, principalHolding(ownerCallerPerms...), method, target, body, pathValues...)
}

// adminReqAs drives a handler with a specific caller principal in context, as
// RequireAuth would inject in production — needed to exercise the caller-subset
// authorization guard.
func adminReqAs(t *testing.T, h http.Handler, p middleware.Principal, method, target, body string, pathValues ...string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	r = r.WithContext(middleware.WithPrincipal(r.Context(), p))
	for i := 0; i+1 < len(pathValues); i += 2 {
		r.SetPathValue(pathValues[i], pathValues[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// --- users ----------------------------------------------------------------

func TestAdminCreateUser(t *testing.T) {
	f := &fakeAdmin{}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).CreateUser)
	w := doAdminReq(t, h, http.MethodPost, "/api/v1/users",
		`{"email":"prof@example.com","displayName":"Prof","tempPassword":"init-pw-123","roleIds":["rol-1"]}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
	if f.createIn.email != "prof@example.com" || f.createIn.temp != "init-pw-123" || len(f.createIn.roleIDs) != 1 {
		t.Errorf("captured input mismatch: %+v", f.createIn)
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["id"] != "usr-1" || out["mustChangePassword"] != true {
		t.Errorf("body = %v", out)
	}
}

func TestAdminCreateUserNilRoleIDsSerializesAsEmptyArray(t *testing.T) {
	f := &fakeAdmin{}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).CreateUser)
	// A user created with no roles: the service returns nil roleIDs, which MUST
	// serialize as [] (not null) — a null crashed the web users panel, which
	// does user.roleIds.length / new Set(user.roleIds).
	w := doAdminReq(t, h, http.MethodPost, "/api/v1/users",
		`{"email":"noroles@example.com","displayName":"No Roles","tempPassword":"init-pw-123"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
	if strings.Contains(w.Body.String(), `"roleIds":null`) {
		t.Errorf("roleIds serialized as null: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"roleIds":[]`) {
		t.Errorf("body = %s, want roleIds:[]", w.Body.String())
	}
}

func TestAdminCreateUserDuplicateEmailIs409(t *testing.T) {
	f := &fakeAdmin{err: connect.NewError(connect.CodeAlreadyExists, errString("dup"))}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).CreateUser)
	w := doAdminReq(t, h, http.MethodPost, "/api/v1/users",
		`{"email":"dup@example.com","displayName":"X","tempPassword":"init-pw-123"}`)
	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", w.Code)
	}
}

func TestAdminCreateUserBadBodyIs400(t *testing.T) {
	h := http.HandlerFunc(newAdminHandler(&fakeAdmin{}, &fakeLinker{}).CreateUser)
	w := doAdminReq(t, h, http.MethodPost, "/api/v1/users", `{not json`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAdminListUsers(t *testing.T) {
	h := http.HandlerFunc(newAdminHandler(&fakeAdmin{}, &fakeLinker{}).ListUsers)
	w := doAdminReq(t, h, http.MethodGet, "/api/v1/users", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var out struct {
		Users []map[string]any `json:"users"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Users) != 1 || out.Users[0]["id"] != "usr-1" {
		t.Errorf("body = %v", out)
	}
}

func TestAdminGetUser(t *testing.T) {
	f := &fakeAdmin{getUser: client.User{Email: "prof@example.com", Status: "active", RoleIDs: []string{"rol-1"}}}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).GetUser)
	w := doAdminReq(t, h, http.MethodGet, "/api/v1/users/usr-9", "", "id", "usr-9")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["id"] != "usr-9" {
		t.Errorf("body = %v", out)
	}
}

func TestAdminSetUserStatusSuspend(t *testing.T) {
	f := &fakeAdmin{}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).SetUserStatus)
	w := doAdminReq(t, h, http.MethodPost, "/api/v1/users/usr-1/status", `{"status":"suspended"}`, "id", "usr-1")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if f.statusUserID != "usr-1" || f.statusValue != "suspended" {
		t.Errorf("captured = %q/%q", f.statusUserID, f.statusValue)
	}
}

// SetUserRoles diffs the current role set (via GetUser) against the requested
// set: adds → AssignRole, removals → RevokeRole. Here current={rol-1,rol-2},
// requested={rol-2,rol-3} ⇒ assign rol-3, revoke rol-1.
func TestAdminSetUserRolesDiffs(t *testing.T) {
	f := &fakeAdmin{getUser: client.User{RoleIDs: []string{"rol-1", "rol-2"}}}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).SetUserRoles)
	w := doAdminReq(t, h, http.MethodPost, "/api/v1/users/usr-1/roles", `{"roleIds":["rol-2","rol-3"]}`, "id", "usr-1")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(f.assigned) != 1 || f.assigned[0] != "usr-1:rol-3" {
		t.Errorf("assigned = %v, want [usr-1:rol-3]", f.assigned)
	}
	if len(f.revoked) != 1 || f.revoked[0] != "usr-1:rol-1" {
		t.Errorf("revoked = %v, want [usr-1:rol-1]", f.revoked)
	}
}

func TestAdminSetUserRolesUnknownUserIs404(t *testing.T) {
	f := &fakeAdmin{getUserErr: connect.NewError(connect.CodeNotFound, errString("no user"))}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).SetUserRoles)
	w := doAdminReq(t, h, http.MethodPost, "/api/v1/users/ghost/roles", `{"roleIds":["rol-1"]}`, "id", "ghost")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// --- roles ----------------------------------------------------------------

func TestAdminCreateRole(t *testing.T) {
	f := &fakeAdmin{}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).CreateRole)
	w := doAdminReq(t, h, http.MethodPost, "/api/v1/roles",
		`{"name":"docente","permissions":["students:read","grades:read","grades:write"]}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
	if f.roleName != "docente" || len(f.rolePerms) != 3 {
		t.Errorf("captured = %q/%v", f.roleName, f.rolePerms)
	}
}

func TestAdminCreateRoleNilPermissionsSerializesAsEmptyArray(t *testing.T) {
	f := &fakeAdmin{}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).CreateRole)
	// A role with no permissions: the service returns a nil slice, which MUST
	// serialize as [] (not null) — a null crashed the web roles panel, which
	// reads role.permissions.length.
	w := doAdminReq(t, h, http.MethodPost, "/api/v1/roles", `{"name":"empty"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
	if strings.Contains(w.Body.String(), `"permissions":null`) {
		t.Errorf("permissions serialized as null: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"permissions":[]`) {
		t.Errorf("body = %s, want permissions:[]", w.Body.String())
	}
}

func TestAdminCreateRoleBadPermissionIs400(t *testing.T) {
	f := &fakeAdmin{err: connect.NewError(connect.CodeInvalidArgument, errString("bad perm"))}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).CreateRole)
	// Caller nominally "holds" the string so the subset guard passes and the
	// request reaches iam, which rejects it as a non-catalog permission (400).
	w := adminReqAs(t, h, principalHolding("bad"), http.MethodPost, "/api/v1/roles", `{"name":"x","permissions":["bad"]}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// --- caller-subset authorization guard ------------------------------------
// A delegated admin may only grant/assign permissions it already holds itself.
// Without this, a roles:write / users:write holder could mint or assign a role
// carrying finance:read etc. and escalate to permissions its own account lacks.

func TestAdminCreateRoleRejectsPermissionCallerLacks(t *testing.T) {
	f := &fakeAdmin{}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).CreateRole)
	// Caller holds roles:write + students:read but NOT finance:read.
	w := adminReqAs(t, h, principalHolding("roles:write", "students:read"),
		http.MethodPost, "/api/v1/roles", `{"name":"escalado","permissions":["students:read","finance:read"]}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	if f.roleName != "" || f.rolePerms != nil {
		t.Errorf("CreateRole must not be called on a rejected grant, captured %q/%v", f.roleName, f.rolePerms)
	}
}

func TestAdminSetRolePermissionsRejectsPermissionCallerLacks(t *testing.T) {
	f := &fakeAdmin{}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).SetRolePermissions)
	w := adminReqAs(t, h, principalHolding("roles:write", "grades:read"),
		http.MethodPost, "/api/v1/roles/rol-1/permissions", `{"permissions":["finance:read"]}`, "id", "rol-1")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	if f.setPermsRole != "" || f.setPerms != nil {
		t.Errorf("SetRolePermissions must not be called on a rejected grant, captured %q/%v", f.setPermsRole, f.setPerms)
	}
}

func TestAdminCreateRoleAllowsPermissionsWithinCallerSet(t *testing.T) {
	f := &fakeAdmin{}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).CreateRole)
	w := adminReqAs(t, h, principalHolding("roles:write", "students:read", "grades:read"),
		http.MethodPost, "/api/v1/roles", `{"name":"docente","permissions":["students:read","grades:read"]}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
	if len(f.rolePerms) != 2 {
		t.Errorf("CreateRole should have been called with 2 perms, got %v", f.rolePerms)
	}
}

func TestAdminSetUserRolesRejectsAssigningRoleWithUnheldPermission(t *testing.T) {
	f := &fakeAdmin{
		getUser:   client.User{RoleIDs: []string{}},
		rolesList: []client.Role{{ID: "rol-priv", Name: "coordinador", Permissions: []string{"finance:read"}}},
	}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).SetUserRoles)
	// Caller holds users:write but NOT finance:read → cannot assign rol-priv.
	w := adminReqAs(t, h, principalHolding("users:write"),
		http.MethodPost, "/api/v1/users/usr-1/roles", `{"roleIds":["rol-priv"]}`, "id", "usr-1")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	if len(f.assigned) != 0 {
		t.Errorf("no role must be assigned on a rejected escalation, got %v", f.assigned)
	}
}

func TestAdminSetUserRolesAllowsAssigningRoleWithinCallerSet(t *testing.T) {
	f := &fakeAdmin{
		getUser:   client.User{RoleIDs: []string{}},
		rolesList: []client.Role{{ID: "rol-priv", Name: "coordinador", Permissions: []string{"finance:read"}}},
	}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).SetUserRoles)
	// Caller DOES hold finance:read → may delegate it via rol-priv.
	w := adminReqAs(t, h, principalHolding("users:write", "finance:read"),
		http.MethodPost, "/api/v1/users/usr-1/roles", `{"roleIds":["rol-priv"]}`, "id", "usr-1")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(f.assigned) != 1 || f.assigned[0] != "usr-1:rol-priv" {
		t.Errorf("assigned = %v, want [usr-1:rol-priv]", f.assigned)
	}
}

func TestAdminCreateUserRejectsAssigningRoleWithUnheldPermission(t *testing.T) {
	f := &fakeAdmin{
		rolesList: []client.Role{{ID: "rol-priv", Name: "coordinador", Permissions: []string{"finance:read"}}},
	}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).CreateUser)
	w := adminReqAs(t, h, principalHolding("users:write"),
		http.MethodPost, "/api/v1/users",
		`{"email":"x@example.com","displayName":"X","tempPassword":"init-pw-123","roleIds":["rol-priv"]}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	if f.createIn.email != "" {
		t.Errorf("CreateUser must not be called on a rejected escalation, captured %+v", f.createIn)
	}
}

func TestAdminListRoles(t *testing.T) {
	h := http.HandlerFunc(newAdminHandler(&fakeAdmin{}, &fakeLinker{}).ListRoles)
	w := doAdminReq(t, h, http.MethodGet, "/api/v1/roles", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var out struct {
		Roles []map[string]any `json:"roles"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Roles) != 1 || out.Roles[0]["name"] != "docente" {
		t.Errorf("body = %v", out)
	}
}

func TestAdminSetRolePermissions(t *testing.T) {
	f := &fakeAdmin{}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).SetRolePermissions)
	w := doAdminReq(t, h, http.MethodPost, "/api/v1/roles/rol-1/permissions", `{"permissions":["grades:read"]}`, "id", "rol-1")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if f.setPermsRole != "rol-1" || len(f.setPerms) != 1 {
		t.Errorf("captured = %q/%v", f.setPermsRole, f.setPerms)
	}
}

// --- teacher ↔ user link --------------------------------------------------

// The link handler validates the user exists via iam GetUser, THEN calls config
// SetTeacherUser. Happy path: user exists ⇒ config is called and 200.
func TestAdminLinkTeacherUser(t *testing.T) {
	f := &fakeAdmin{getUser: client.User{Email: "prof@example.com", Status: "active"}}
	l := &fakeLinker{}
	h := http.HandlerFunc(newAdminHandler(f, l).LinkTeacherUser)
	w := doAdminReq(t, h, http.MethodPost, "/api/v1/config/teachers/tch-1/user", `{"userId":"usr-1"}`, "id", "tch-1")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if l.teacherID != "tch-1" || l.userID != "usr-1" {
		t.Errorf("linker captured = %q/%q", l.teacherID, l.userID)
	}
}

// A non-existent user must NOT reach config: GetUser NotFound ⇒ 404 and the
// linker is never called (this is the gateway-orchestrated cross-service check).
func TestAdminLinkTeacherUserUnknownUserIs404(t *testing.T) {
	f := &fakeAdmin{getUserErr: connect.NewError(connect.CodeNotFound, errString("no user"))}
	l := &fakeLinker{}
	h := http.HandlerFunc(newAdminHandler(f, l).LinkTeacherUser)
	w := doAdminReq(t, h, http.MethodPost, "/api/v1/config/teachers/tch-1/user", `{"userId":"ghost"}`, "id", "tch-1")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
	if l.teacherID != "" || l.userID != "" {
		t.Errorf("linker must not be called on invalid user, got %q/%q", l.teacherID, l.userID)
	}
}

// Unlink (empty userId) skips the GetUser validation and calls config directly.
func TestAdminLinkTeacherUserUnlinkSkipsValidation(t *testing.T) {
	f := &fakeAdmin{getUserErr: connect.NewError(connect.CodeNotFound, errString("should not be called"))}
	l := &fakeLinker{}
	h := http.HandlerFunc(newAdminHandler(f, l).LinkTeacherUser)
	w := doAdminReq(t, h, http.MethodPost, "/api/v1/config/teachers/tch-1/user", `{"userId":""}`, "id", "tch-1")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (unlink)", w.Code)
	}
	if l.teacherID != "tch-1" || l.userID != "" {
		t.Errorf("linker captured = %q/%q, want tch-1/empty", l.teacherID, l.userID)
	}
}
