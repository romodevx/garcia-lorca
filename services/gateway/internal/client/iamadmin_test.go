package client_test

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"

	iamv1 "github.com/garcia-lorca/gen/go/garcialorca/iam/v1"
)

// The admin RPCs extend the shared stubIAM (defined in iamauth_test.go). Each
// returns canned data or a mapped Connect error so the AuthClient admin methods
// are exercised over a REAL Connect round-trip.

func (stubIAM) CreateUser(_ context.Context, req *connect.Request[iamv1.CreateUserRequest]) (*connect.Response[iamv1.CreateUserResponse], error) {
	if req.Msg.GetEmail() == "dup@example.com" {
		return nil, connect.NewError(connect.CodeAlreadyExists, errString("email exists"))
	}
	return connect.NewResponse(&iamv1.CreateUserResponse{User: &iamv1.User{
		Id:                 "usr-1",
		Email:              req.Msg.GetEmail(),
		DisplayName:        req.Msg.GetDisplayName(),
		Status:             iamv1.UserStatus_USER_STATUS_ACTIVE,
		MustChangePassword: true,
		RoleIds:            req.Msg.GetRoleIds(),
	}}), nil
}

func (stubIAM) GetUser(_ context.Context, req *connect.Request[iamv1.GetUserRequest]) (*connect.Response[iamv1.GetUserResponse], error) {
	if req.Msg.GetId() != "usr-1" {
		return nil, connect.NewError(connect.CodeNotFound, errString("user not found"))
	}
	return connect.NewResponse(&iamv1.GetUserResponse{User: &iamv1.User{
		Id:      "usr-1",
		Email:   "prof@example.com",
		Status:  iamv1.UserStatus_USER_STATUS_ACTIVE,
		RoleIds: []string{"rol-1"},
	}}), nil
}

func (stubIAM) ListUsers(_ context.Context, _ *connect.Request[iamv1.ListUsersRequest]) (*connect.Response[iamv1.ListUsersResponse], error) {
	return connect.NewResponse(&iamv1.ListUsersResponse{Users: []*iamv1.User{
		{Id: "usr-1", Email: "prof@example.com", Status: iamv1.UserStatus_USER_STATUS_ACTIVE, RoleIds: []string{"rol-1"}},
	}}), nil
}

func (stubIAM) SetUserStatus(_ context.Context, req *connect.Request[iamv1.SetUserStatusRequest]) (*connect.Response[iamv1.SetUserStatusResponse], error) {
	if req.Msg.GetAccessToken() != "caller-token" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errString("invalid access token"))
	}
	return connect.NewResponse(&iamv1.SetUserStatusResponse{User: &iamv1.User{
		Id:     req.Msg.GetUserId(),
		Status: req.Msg.GetStatus(),
	}}), nil
}

func (stubIAM) AssignRole(_ context.Context, _ *connect.Request[iamv1.AssignRoleRequest]) (*connect.Response[iamv1.AssignRoleResponse], error) {
	return connect.NewResponse(&iamv1.AssignRoleResponse{}), nil
}

func (stubIAM) RevokeRole(_ context.Context, _ *connect.Request[iamv1.RevokeRoleRequest]) (*connect.Response[iamv1.RevokeRoleResponse], error) {
	return connect.NewResponse(&iamv1.RevokeRoleResponse{}), nil
}

func (stubIAM) CreateRole(_ context.Context, req *connect.Request[iamv1.CreateRoleRequest]) (*connect.Response[iamv1.CreateRoleResponse], error) {
	if req.Msg.GetName() == "dup" {
		return nil, connect.NewError(connect.CodeAlreadyExists, errString("role exists"))
	}
	return connect.NewResponse(&iamv1.CreateRoleResponse{Role: &iamv1.Role{
		Id:          "rol-1",
		Name:        req.Msg.GetName(),
		Permissions: req.Msg.GetPermissions(),
	}}), nil
}

func (stubIAM) GetRole(_ context.Context, req *connect.Request[iamv1.GetRoleRequest]) (*connect.Response[iamv1.GetRoleResponse], error) {
	return connect.NewResponse(&iamv1.GetRoleResponse{Role: &iamv1.Role{
		Id: req.Msg.GetId(), Name: "docente", Permissions: []string{"grades:read"},
	}}), nil
}

func (stubIAM) ListRoles(_ context.Context, _ *connect.Request[iamv1.ListRolesRequest]) (*connect.Response[iamv1.ListRolesResponse], error) {
	return connect.NewResponse(&iamv1.ListRolesResponse{Roles: []*iamv1.Role{
		{Id: "rol-1", Name: "docente", Permissions: []string{"grades:read", "grades:write"}},
	}}), nil
}

func (stubIAM) SetRolePermissions(_ context.Context, req *connect.Request[iamv1.SetRolePermissionsRequest]) (*connect.Response[iamv1.SetRolePermissionsResponse], error) {
	if len(req.Msg.GetPermissions()) == 1 && req.Msg.GetPermissions()[0] == "bad" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errString("bad permission"))
	}
	return connect.NewResponse(&iamv1.SetRolePermissionsResponse{Role: &iamv1.Role{
		Id: req.Msg.GetRoleId(), Name: "docente", Permissions: req.Msg.GetPermissions(),
	}}), nil
}

func TestAuthClientCreateUser(t *testing.T) {
	c, stop := newStubServer(t)
	defer stop()

	u, err := c.CreateUser(context.Background(), "prof@example.com", "Prof", "init-pw-123", []string{"rol-1"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.ID != "usr-1" || u.Email != "prof@example.com" || !u.MustChangePassword {
		t.Errorf("user mismatch: %+v", u)
	}
	if u.Status != "active" {
		t.Errorf("status = %q, want active", u.Status)
	}
	if len(u.RoleIDs) != 1 || u.RoleIDs[0] != "rol-1" {
		t.Errorf("roleIDs mismatch: %v", u.RoleIDs)
	}
}

func TestAuthClientCreateUserDuplicateEmail(t *testing.T) {
	c, stop := newStubServer(t)
	defer stop()

	_, err := c.CreateUser(context.Background(), "dup@example.com", "Dup", "init-pw-123", nil)
	if connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Errorf("code = %v, want AlreadyExists", connect.CodeOf(err))
	}
}

func TestAuthClientGetUser(t *testing.T) {
	c, stop := newStubServer(t)
	defer stop()

	u, err := c.GetUser(context.Background(), "usr-1")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if u.ID != "usr-1" || len(u.RoleIDs) != 1 {
		t.Errorf("user mismatch: %+v", u)
	}
}

func TestAuthClientGetUserNotFound(t *testing.T) {
	c, stop := newStubServer(t)
	defer stop()

	if _, err := c.GetUser(context.Background(), "ghost"); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("code = %v, want NotFound", connect.CodeOf(err))
	}
}

func TestAuthClientListUsers(t *testing.T) {
	c, stop := newStubServer(t)
	defer stop()

	us, err := c.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(us) != 1 || us[0].ID != "usr-1" {
		t.Errorf("users mismatch: %+v", us)
	}
}

func TestAuthClientSetUserStatusSuspend(t *testing.T) {
	c, stop := newStubServer(t)
	defer stop()

	u, err := c.SetUserStatus(context.Background(), "caller-token", "usr-1", "suspended")
	if err != nil {
		t.Fatalf("SetUserStatus: %v", err)
	}
	if u.Status != "suspended" {
		t.Errorf("status = %q, want suspended", u.Status)
	}
	// The caller's token travels as given: iam reads who is asking from it.
	if _, err := c.SetUserStatus(context.Background(), "someone-else", "usr-1", "suspended"); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want Unauthenticated — the token must be forwarded as given", connect.CodeOf(err))
	}
}

func (stubIAM) InviteGuardian(_ context.Context, req *connect.Request[iamv1.InviteGuardianRequest]) (*connect.Response[iamv1.InviteGuardianResponse], error) {
	if req.Msg.GetEmail() == "existe@example.com" {
		// A live account: iam adds the role and sends nothing.
		return connect.NewResponse(&iamv1.InviteGuardianResponse{
			User: &iamv1.User{Id: "usr-old", Email: req.Msg.GetEmail(), Status: iamv1.UserStatus_USER_STATUS_ACTIVE},
		}), nil
	}
	return connect.NewResponse(&iamv1.InviteGuardianResponse{
		User:       &iamv1.User{Id: "usr-g", Email: req.Msg.GetEmail(), DisplayName: req.Msg.GetDisplayName(), Status: iamv1.UserStatus_USER_STATUS_INVITED},
		Invitation: &iamv1.Invitation{Url: "https://x/activar?t=abc", Sent: false},
	}), nil
}

func TestAuthClientInviteGuardian(t *testing.T) {
	c, stop := newStubServer(t)
	defer stop()

	u, inv, err := c.InviteGuardian(context.Background(), "mama@example.com", "Mamá")
	if err != nil {
		t.Fatalf("InviteGuardian: %v", err)
	}
	if u.ID != "usr-g" || u.Email != "mama@example.com" || u.Status != "invited" {
		t.Errorf("user = %+v", u)
	}
	if inv == nil || inv.URL != "https://x/activar?t=abc" || inv.Sent {
		t.Errorf("invitation = %+v", inv)
	}

	// An existing account comes back with no invitation at all — not an empty
	// one that would read as "a link was issued and not sent".
	old, none, err := c.InviteGuardian(context.Background(), "existe@example.com", "Papá")
	if err != nil {
		t.Fatalf("InviteGuardian (existing): %v", err)
	}
	if old.ID != "usr-old" || none != nil {
		t.Errorf("got (%q, %+v), want usr-old and no invitation", old.ID, none)
	}
}

func TestAuthClientRoleCRUD(t *testing.T) {
	c, stop := newStubServer(t)
	defer stop()

	r, err := c.CreateRole(context.Background(), "docente", []string{"grades:read", "grades:write"})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if r.ID != "rol-1" || len(r.Permissions) != 2 {
		t.Errorf("role mismatch: %+v", r)
	}
	rs, err := c.ListRoles(context.Background())
	if err != nil || len(rs) != 1 {
		t.Fatalf("ListRoles: %v (%+v)", err, rs)
	}
	edited, err := c.SetRolePermissions(context.Background(), "rol-1", []string{"grades:read"})
	if err != nil {
		t.Fatalf("SetRolePermissions: %v", err)
	}
	if len(edited.Permissions) != 1 {
		t.Errorf("edited perms = %v", edited.Permissions)
	}
}

func TestAuthClientCreateRoleDuplicate(t *testing.T) {
	c, stop := newStubServer(t)
	defer stop()

	if _, err := c.CreateRole(context.Background(), "dup", nil); connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Errorf("code = %v, want AlreadyExists", connect.CodeOf(err))
	}
}

func TestAuthClientSetRolePermissionsInvalid(t *testing.T) {
	c, stop := newStubServer(t)
	defer stop()

	if _, err := c.SetRolePermissions(context.Background(), "rol-1", []string{"bad"}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestAuthClientAssignRevokeRole(t *testing.T) {
	c, stop := newStubServer(t)
	defer stop()

	if err := c.AssignRole(context.Background(), "usr-1", "rol-1"); err != nil {
		t.Errorf("AssignRole: %v", err)
	}
	if err := c.RevokeRole(context.Background(), "usr-1", "rol-1"); err != nil {
		t.Errorf("RevokeRole: %v", err)
	}
}
