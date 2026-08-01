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
func (f *fakeUsers) UpdatePassword(_ context.Context, _, _ string) error {
	f.updated = true
	return nil
}
func (f *fakeUsers) AddRole(context.Context, string, string) error    { return nil }
func (f *fakeUsers) RemoveRole(context.Context, string, string) error { return nil }

type fakeRoles struct{ known string }

func (f *fakeRoles) FindByID(_ context.Context, id string) (domain.Role, error) {
	if id == f.known {
		r, _ := domain.NewRole(id, "R", []string{"students:read"})
		return r, nil
	}
	return domain.Role{}, glerrors.ErrNotFound
}

func newHandler(t *testing.T) *grpcadapter.IAMHandler {
	t.Helper()
	h, _ := newHandlerWithUsers(t, false)
	return h
}

// newHandlerWithUsers builds a handler over a single seeded user (password
// "secret") and returns the fake repo so tests can assert side effects. When
// mustChange is true the user still holds a temporary password.
func newHandlerWithUsers(t *testing.T, mustChange bool) (*grpcadapter.IAMHandler, *fakeUsers) {
	t.Helper()
	hasher := bcrypt.NewHasher(4)
	hash, _ := hasher.Hash("secret")
	email, _ := domain.NewEmail("ana@garcialorca.mx")
	role, _ := domain.NewRole("r1", "Docente", []string{"students:read", "grades:create"})
	user, _ := domain.NewUser("u1", "t1", email, "Ana", domain.StatusActive, mustChange, []domain.Role{role})

	users := &fakeUsers{user: user, hash: hash}
	tokens := jwt.NewHMAC([]byte(strings.Repeat("gl-key-", 6)), "garcia-lorca", time.Hour, time.Now)
	auth := usecase.NewAuthService(users, &fakeRoles{known: "r1"}, hasher, tokens, tokens, "t1", 15*time.Minute, time.Now)
	return grpcadapter.NewIAMHandler(auth, usecase.NewHealthService("iam")), users
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

	login, err := h.Login(ctx, connect.NewRequest(&iamv1.LoginRequest{Email: "ana@garcialorca.mx", Password: "secret"}))
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
	res, err := h.Login(context.Background(), connect.NewRequest(&iamv1.LoginRequest{Email: "ana@garcialorca.mx", Password: "secret"}))
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

func TestChangePasswordClearsFlagAndPersists(t *testing.T) {
	h, users := newHandlerWithUsers(t, true)
	_, err := h.ChangePassword(context.Background(), connect.NewRequest(&iamv1.ChangePasswordRequest{
		UserId:          "u1",
		CurrentPassword: "secret",
		NewPassword:     "NuevaClave#2026",
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
		UserId:          "u1",
		CurrentPassword: "wrong",
		NewPassword:     "NuevaClave#2026",
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
	_, err := h.Login(context.Background(), connect.NewRequest(&iamv1.LoginRequest{Email: "nope", Password: "secret"}))
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

func assertCode(t *testing.T, err error, want connect.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %v, got nil", want)
	}
	if got := connect.CodeOf(err); got != want {
		t.Errorf("code = %v, want %v (err=%v)", got, want, err)
	}
}
