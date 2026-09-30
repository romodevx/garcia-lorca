package grpcadapter_test

import (
	"context"
	"strings"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	iamv1 "github.com/garcia-lorca/gen/go/garcialorca/iam/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/adapters/bcrypt"
	grpcadapter "github.com/garcia-lorca/services/iam/internal/adapters/grpc"
	"github.com/garcia-lorca/services/iam/internal/adapters/jwt"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/usecase"
)

// --- fake repos (only what the handler exercises) --------------------------

type fakeUsers struct {
	user    domain.User
	hash    string
	updated bool

	// admin-surface recording / control.
	created   *createdUser             // last CreateUser call
	createErr error                    // forced CreateUser error
	listOut   []domain.User            // ListUsers return
	statusSet map[string]domain.Status // SetStatus calls
	statusErr error                    // forced SetStatus error
	rolesSet  map[string][]string      // SetUserRoles calls
}

type createdUser struct {
	user domain.User
	hash string
}

func (f *fakeUsers) FindAuthByEmail(_ context.Context, _ string, email domain.Email) (domain.User, string, error) {
	if email == f.user.Email() {
		return f.user, f.hash, nil
	}
	return domain.User{}, "", glerrors.ErrNotFound
}
func (f *fakeUsers) FindByID(_ context.Context, id string) (domain.User, error) {
	if id == f.user.ID() {
		return f.user, nil
	}
	return domain.User{}, glerrors.ErrNotFound
}
func (f *fakeUsers) FindAuthByID(_ context.Context, id string) (domain.User, string, error) {
	if id == f.user.ID() {
		return f.user, f.hash, nil
	}
	return domain.User{}, "", glerrors.ErrNotFound
}
func (f *fakeUsers) TouchPasswordChangedAt(context.Context, string) error { return nil }

func (f *fakeUsers) UpdatePassword(_ context.Context, _, _ string) error {
	f.updated = true
	return nil
}
func (f *fakeUsers) AddRole(context.Context, string, string) error    { return nil }
func (f *fakeUsers) RemoveRole(context.Context, string, string) error { return nil }

func (f *fakeUsers) CreateUser(_ context.Context, user domain.User, passwordHash string) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = &createdUser{user: user, hash: passwordHash}
	// A repository can find what it just stored. Recording the call without
	// storing the row made this double lie about exactly that.
	f.user = user
	return nil
}
func (f *fakeUsers) ListUsers(context.Context) ([]domain.User, error) { return f.listOut, nil }
func (f *fakeUsers) SetStatus(_ context.Context, userID string, status domain.Status) error {
	if f.statusErr != nil {
		return f.statusErr
	}
	if f.statusSet == nil {
		f.statusSet = map[string]domain.Status{}
	}
	f.statusSet[userID] = status
	// APPLY it, do not merely record it. A double that remembers the call but
	// keeps handing back the old row lies about the one thing a repository
	// promises: what you write, you then read back. That lie made a test pass
	// while ResendInvitation happily re-invited an account that had just been
	// activated.
	if userID == f.user.ID() {
		f.user = rebuildWithStatus(f.user, status)
	}
	return nil
}

// rebuildWithStatus is the only way to change a domain User's status from
// outside the domain: it is immutable by construction, which is the point.
func rebuildWithStatus(u domain.User, status domain.Status) domain.User {
	rebuilt, err := domain.NewUser(u.ID(), "t1", u.Email(), u.DisplayName(), status, u.MustChangePassword(), nil)
	if err != nil {
		return u
	}
	return rebuilt
}
func (f *fakeUsers) SetUserRoles(_ context.Context, userID string, roleIDs []string) error {
	if f.rolesSet == nil {
		f.rolesSet = map[string][]string{}
	}
	f.rolesSet[userID] = roleIDs
	return nil
}

func (f *fakeUsers) SoftDelete(context.Context, string, string, string) error { return nil }
func (f *fakeUsers) Restore(context.Context, string, string, []string) error  { return nil }

type fakeRoles struct {
	known string

	created   []domain.Role       // CreateRole calls
	createErr error               // forced CreateRole error
	listOut   []domain.Role       // ListRoles return
	permsSet  map[string][]string // SetPermissions calls
}

func (f *fakeRoles) FindByID(_ context.Context, id string) (domain.Role, error) {
	if id == f.known {
		r, _ := domain.NewRole(id, "R", []string{"students:read"})
		return r, nil
	}
	return domain.Role{}, glerrors.ErrNotFound
}
func (f *fakeRoles) CreateRole(_ context.Context, role domain.Role) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, role)
	return nil
}
func (f *fakeRoles) ListRoles(context.Context) ([]domain.Role, error) { return f.listOut, nil }
func (f *fakeRoles) SetPermissions(_ context.Context, roleID string, permissions []string) error {
	if f.permsSet == nil {
		f.permsSet = map[string][]string{}
	}
	f.permsSet[roleID] = permissions
	return nil
}

func newHandler(t *testing.T) *grpcadapter.IAMHandler {
	t.Helper()
	h, _ := newHandlerWithUsers(t, false)
	return h
}

// newHandlerWithUsers builds a handler over a single seeded user (password
// "test-owner-pw") and returns the fake repo so tests can assert side effects. When
// mustChange is true the user still holds a temporary password.
func newHandlerWithUsers(t *testing.T, mustChange bool) (*grpcadapter.IAMHandler, *fakeUsers) {
	t.Helper()
	hasher := bcrypt.NewHasher(4)
	hash, _ := hasher.Hash("test-owner-pw")
	email, _ := domain.NewEmail("ana@garcialorca.mx")
	role, _ := domain.NewRole("r1", "Docente", []string{"students:read", "grades:create"})
	user, _ := domain.NewUser("u1", "t1", email, "Ana", domain.StatusActive, mustChange, []domain.Role{role})

	users := &fakeUsers{user: user, hash: hash}
	tokens := jwt.NewHMAC([]byte(strings.Repeat("gl-key-", 6)), "garcia-lorca", time.Hour, time.Now)
	roles := &fakeRoles{known: "r1"}
	auth := usecase.NewAuthService(users, roles, hasher, tokens, tokens, nil, nil, "t1", 15*time.Minute, time.Now)
	admin := usecase.NewAdminService(users, roles, hasher, nil, nil, nil, "", "t1", func() string { return "generated-id" }, nil)
	return grpcadapter.NewIAMHandler(auth, admin, usecase.NewHealthService("iam")), users
}

// --- tests -----------------------------------------------------------------

func TestHealth(t *testing.T) {
	res, err := newHandler(t).Health(context.Background(), connect.NewRequest(&iamv1.HealthRequest{}))
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if res.Msg.GetStatus() != "SERVING" {
		t.Errorf("status = %q", res.Msg.GetStatus())
	}
}

func TestLoginThenValidateAndCheckPermission(t *testing.T) {
	h := newHandler(t)
	ctx := context.Background()

	login, err := h.Login(ctx, connect.NewRequest(&iamv1.LoginRequest{Email: "ana@garcialorca.mx", Password: "test-owner-pw"}))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if login.Msg.GetAccessToken() == "" || login.Msg.GetRefreshToken() == "" {
		t.Fatal("expected tokens")
	}
	if login.Msg.GetExpiresIn() != 900 {
		t.Errorf("expires_in = %d, want 900", login.Msg.GetExpiresIn())
	}
	if login.Msg.GetUser().GetId() != "u1" || login.Msg.GetUser().GetStatus() != iamv1.UserStatus_USER_STATUS_ACTIVE {
		t.Errorf("user mapping: %+v", login.Msg.GetUser())
	}

	access := login.Msg.GetAccessToken()

	val, err := h.ValidateToken(ctx, connect.NewRequest(&iamv1.ValidateTokenRequest{AccessToken: access}))
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if val.Msg.GetClaims().GetSubject() != "u1" || len(val.Msg.GetClaims().GetPermissions()) != 2 {
		t.Errorf("claims: %+v", val.Msg.GetClaims())
	}

	ok, err := h.CheckPermission(ctx, connect.NewRequest(&iamv1.CheckPermissionRequest{AccessToken: access, Permission: "students:read"}))
	if err != nil || !ok.Msg.GetAllowed() {
		t.Errorf("students:read should be allowed: %v %v", ok.Msg.GetAllowed(), err)
	}
	deny, _ := h.CheckPermission(ctx, connect.NewRequest(&iamv1.CheckPermissionRequest{AccessToken: access, Permission: "students:delete"}))
	if deny.Msg.GetAllowed() {
		t.Error("students:delete should be denied")
	}
}

func TestLoginResponseCarriesMustChangePassword(t *testing.T) {
	h, _ := newHandlerWithUsers(t, true)
	res, err := h.Login(context.Background(), connect.NewRequest(&iamv1.LoginRequest{Email: "ana@garcialorca.mx", Password: "test-owner-pw"}))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !res.Msg.GetMustChangePassword() {
		t.Error("LoginResponse.MustChangePassword = false, want true")
	}
	if !res.Msg.GetUser().GetMustChangePassword() {
		t.Error("User.MustChangePassword = false, want true")
	}
}

// loginAccessToken logs the seeded user in and returns a valid access token so
// ChangePassword (which now derives the subject from the token) can be exercised.
func loginAccessToken(t *testing.T, h *grpcadapter.IAMHandler) string {
	t.Helper()
	login, err := h.Login(context.Background(), connect.NewRequest(&iamv1.LoginRequest{Email: "ana@garcialorca.mx", Password: "test-owner-pw"}))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	return login.Msg.GetAccessToken()
}

func TestChangePasswordClearsFlagAndPersists(t *testing.T) {
	h, users := newHandlerWithUsers(t, true)
	_, err := h.ChangePassword(context.Background(), connect.NewRequest(&iamv1.ChangePasswordRequest{
		AccessToken:     loginAccessToken(t, h),
		CurrentPassword: "test-owner-pw",
		NewPassword:     "test-new-pw-123",
	}))
	if err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if !users.updated {
		t.Error("expected UpdatePassword to be called")
	}
}

func TestChangePasswordWrongCurrentIsUnauthenticated(t *testing.T) {
	h, _ := newHandlerWithUsers(t, true)
	_, err := h.ChangePassword(context.Background(), connect.NewRequest(&iamv1.ChangePasswordRequest{
		AccessToken:     loginAccessToken(t, h),
		CurrentPassword: "wrong",
		NewPassword:     "test-new-pw-123",
	}))
	assertCode(t, err, connect.CodeUnauthenticated)
}

func TestChangePasswordInvalidTokenIsUnauthenticated(t *testing.T) {
	h, _ := newHandlerWithUsers(t, true)
	_, err := h.ChangePassword(context.Background(), connect.NewRequest(&iamv1.ChangePasswordRequest{
		AccessToken:     "garbage",
		CurrentPassword: "test-owner-pw",
		NewPassword:     "test-new-pw-123",
	}))
	assertCode(t, err, connect.CodeUnauthenticated)
}

func TestLoginWrongPasswordIsUnauthenticated(t *testing.T) {
	h := newHandler(t)
	_, err := h.Login(context.Background(), connect.NewRequest(&iamv1.LoginRequest{Email: "ana@garcialorca.mx", Password: "wrong"}))
	assertCode(t, err, connect.CodeUnauthenticated)
}

func TestLoginMalformedEmailIsInvalidArgument(t *testing.T) {
	h := newHandler(t)
	_, err := h.Login(context.Background(), connect.NewRequest(&iamv1.LoginRequest{Email: "nope", Password: "test-owner-pw"}))
	assertCode(t, err, connect.CodeInvalidArgument)
}

func TestValidateGarbageTokenIsUnauthenticated(t *testing.T) {
	h := newHandler(t)
	_, err := h.ValidateToken(context.Background(), connect.NewRequest(&iamv1.ValidateTokenRequest{AccessToken: "garbage"}))
	assertCode(t, err, connect.CodeUnauthenticated)
}

func TestAssignRoleUnknownIsNotFound(t *testing.T) {
	h := newHandler(t)
	_, err := h.AssignRole(context.Background(), connect.NewRequest(&iamv1.AssignRoleRequest{UserId: "u1", RoleId: "nope"}))
	assertCode(t, err, connect.CodeNotFound)
}

// handlerOver builds a handler over caller-supplied fakes so admin-surface tests
// can control repository behavior (forced conflicts, seeded listings).
func handlerOver(t *testing.T, users *fakeUsers, roles *fakeRoles) *grpcadapter.IAMHandler {
	t.Helper()
	hasher := bcrypt.NewHasher(4)
	tokens := jwt.NewHMAC([]byte(strings.Repeat("gl-key-", 6)), "garcia-lorca", time.Hour, time.Now)
	auth := usecase.NewAuthService(users, roles, hasher, tokens, tokens, nil, nil, "t1", 15*time.Minute, time.Now)
	admin := usecase.NewAdminService(users, roles, hasher, nil, nil, nil, "", "t1", func() string { return "generated-id" }, nil)
	return grpcadapter.NewIAMHandler(auth, admin, usecase.NewHealthService("iam"))
}

func TestCreateUserHashesTempPasswordAndAssignsRoles(t *testing.T) {
	users := &fakeUsers{}
	roles := &fakeRoles{known: "r1"}
	h := handlerOver(t, users, roles)

	res, err := h.CreateUser(context.Background(), connect.NewRequest(&iamv1.CreateUserRequest{
		Email:        "nueva@garcialorca.mx",
		DisplayName:  "Nueva Docente",
		TempPassword: "init-pw-123",
		RoleIds:      []string{"r1"},
	}))
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	got := res.Msg.GetUser()
	if got.GetId() != "generated-id" || got.GetEmail() != "nueva@garcialorca.mx" {
		t.Errorf("user mapping: %+v", got)
	}
	if !got.GetMustChangePassword() {
		t.Error("created user must_change_password = false, want true")
	}
	if len(got.GetRoleIds()) != 1 || got.GetRoleIds()[0] != "r1" {
		t.Errorf("role_ids = %v, want [r1]", got.GetRoleIds())
	}
	if users.created == nil {
		t.Fatal("CreateUser did not persist a user")
	}
	if users.created.hash == "init-pw-123" || users.created.hash == "" {
		t.Errorf("temp password must be hashed, never stored verbatim (hash=%q)", users.created.hash)
	}
	if got := users.rolesSet["generated-id"]; len(got) != 1 || got[0] != "r1" {
		t.Errorf("SetUserRoles recorded %v, want [r1]", got)
	}
}

func TestCreateUserDuplicateEmailIsAlreadyExists(t *testing.T) {
	users := &fakeUsers{createErr: glerrors.New(glerrors.KindConflict, "email already exists")}
	h := handlerOver(t, users, &fakeRoles{})
	_, err := h.CreateUser(context.Background(), connect.NewRequest(&iamv1.CreateUserRequest{
		Email: "taken@garcialorca.mx", DisplayName: "Dup", TempPassword: "init-pw-123",
	}))
	assertCode(t, err, connect.CodeAlreadyExists)
}

func TestCreateUserUnknownRoleIsInvalidArgument(t *testing.T) {
	h := handlerOver(t, &fakeUsers{}, &fakeRoles{known: "r1"})
	_, err := h.CreateUser(context.Background(), connect.NewRequest(&iamv1.CreateUserRequest{
		Email: "a@garcialorca.mx", DisplayName: "A", TempPassword: "init-pw-123", RoleIds: []string{"ghost"},
	}))
	assertCode(t, err, connect.CodeInvalidArgument)
}

func TestListUsersProjectsRoleIds(t *testing.T) {
	email, _ := domain.NewEmail("ana@garcialorca.mx")
	role, _ := domain.NewRole("r1", "Docente", []string{"students:read"})
	u, _ := domain.NewUser("u1", "t1", email, "Ana", domain.StatusActive, false, []domain.Role{role})
	h := handlerOver(t, &fakeUsers{listOut: []domain.User{u}}, &fakeRoles{})

	res, err := h.ListUsers(context.Background(), connect.NewRequest(&iamv1.ListUsersRequest{}))
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(res.Msg.GetUsers()) != 1 {
		t.Fatalf("ListUsers returned %d users, want 1", len(res.Msg.GetUsers()))
	}
	if ids := res.Msg.GetUsers()[0].GetRoleIds(); len(ids) != 1 || ids[0] != "r1" {
		t.Errorf("role_ids = %v, want [r1]", ids)
	}
}

func TestSetUserStatusSuspends(t *testing.T) {
	// The seeded handler's fake user has id "u1"; GetUser after the update reads it.
	h, users := newHandlerWithUsers(t, false)
	res, err := h.SetUserStatus(context.Background(), connect.NewRequest(&iamv1.SetUserStatusRequest{
		UserId: "u1", Status: iamv1.UserStatus_USER_STATUS_SUSPENDED,
	}))
	if err != nil {
		t.Fatalf("SetUserStatus: %v", err)
	}
	if got := users.statusSet["u1"]; got != domain.StatusSuspended {
		t.Errorf("status for u1 = %v, want StatusSuspended", got)
	}
	if res.Msg.GetUser().GetId() != "u1" {
		t.Errorf("response user = %+v, want id u1", res.Msg.GetUser())
	}
}

func TestSetUserStatusUnspecifiedIsInvalidArgument(t *testing.T) {
	h, _ := newHandlerWithUsers(t, false)
	_, err := h.SetUserStatus(context.Background(), connect.NewRequest(&iamv1.SetUserStatusRequest{
		UserId: "u1", Status: iamv1.UserStatus_USER_STATUS_UNSPECIFIED,
	}))
	assertCode(t, err, connect.CodeInvalidArgument)
}

func TestSetUserStatusUnknownUserIsNotFound(t *testing.T) {
	users := &fakeUsers{statusErr: glerrors.ErrNotFound}
	h := handlerOver(t, users, &fakeRoles{})
	_, err := h.SetUserStatus(context.Background(), connect.NewRequest(&iamv1.SetUserStatusRequest{
		UserId: "ghost", Status: iamv1.UserStatus_USER_STATUS_ACTIVE,
	}))
	assertCode(t, err, connect.CodeNotFound)
}

func TestCreateRoleBadPermissionIsInvalidArgument(t *testing.T) {
	roles := &fakeRoles{}
	h := handlerOver(t, &fakeUsers{}, roles)
	_, err := h.CreateRole(context.Background(), connect.NewRequest(&iamv1.CreateRoleRequest{
		Name: "R", Permissions: []string{"Not-A-Permission"},
	}))
	assertCode(t, err, connect.CodeInvalidArgument)
	if len(roles.created) != 0 {
		t.Error("role must not persist on a bad permission")
	}
}

func TestCreateRoleDuplicateNameIsAlreadyExists(t *testing.T) {
	roles := &fakeRoles{createErr: glerrors.New(glerrors.KindConflict, "role name already exists")}
	h := handlerOver(t, &fakeUsers{}, roles)
	_, err := h.CreateRole(context.Background(), connect.NewRequest(&iamv1.CreateRoleRequest{
		Name: "Coordinador", Permissions: []string{"users:read"},
	}))
	assertCode(t, err, connect.CodeAlreadyExists)
}

func TestGetRoleUnknownIsNotFound(t *testing.T) {
	h := handlerOver(t, &fakeUsers{}, &fakeRoles{known: "r1"})
	_, err := h.GetRole(context.Background(), connect.NewRequest(&iamv1.GetRoleRequest{Id: "ghost"}))
	assertCode(t, err, connect.CodeNotFound)
}

func TestSetRolePermissionsReplacesAndReturnsRole(t *testing.T) {
	roles := &fakeRoles{known: "r1"}
	h := handlerOver(t, &fakeUsers{}, roles)
	res, err := h.SetRolePermissions(context.Background(), connect.NewRequest(&iamv1.SetRolePermissionsRequest{
		RoleId: "r1", Permissions: []string{"users:read", "users:write"},
	}))
	if err != nil {
		t.Fatalf("SetRolePermissions: %v", err)
	}
	if got := roles.permsSet["r1"]; len(got) != 2 {
		t.Errorf("SetPermissions recorded %v, want 2 entries", got)
	}
	if res.Msg.GetRole().GetId() != "r1" {
		t.Errorf("response role = %+v, want id r1", res.Msg.GetRole())
	}
}

func TestSetRolePermissionsBadPermissionIsInvalidArgument(t *testing.T) {
	h := handlerOver(t, &fakeUsers{}, &fakeRoles{known: "r1"})
	_, err := h.SetRolePermissions(context.Background(), connect.NewRequest(&iamv1.SetRolePermissionsRequest{
		RoleId: "r1", Permissions: []string{"bad perm"},
	}))
	assertCode(t, err, connect.CodeInvalidArgument)
}

func assertCode(t *testing.T, err error, want connect.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %v, got nil", want)
	}
	if got := connect.CodeOf(err); got != want {
		t.Errorf("code = %v, want %v (err=%v)", got, want, err)
	}
}
