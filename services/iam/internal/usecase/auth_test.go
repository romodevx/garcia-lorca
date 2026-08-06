package usecase_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/usecase"
)

// --- fakes (implement the ports) ------------------------------------------

const testTenant = "t1"

type createdUser struct {
	user domain.User
	hash string
}

type fakeUsers struct {
	byEmail map[domain.Email]struct {
		user domain.User
		hash string
	}
	byID    map[string]domain.User
	added   [][2]string // (userID, roleID)
	removed [][2]string

	// admin-surface recording
	created   []createdUser            // CreateUser calls
	listOut   []domain.User            // ListUsers return
	statusSet map[string]domain.Status // SetStatus calls
	rolesSet  map[string][]string      // SetUserRoles calls
}

func (f *fakeUsers) FindAuthByEmail(_ context.Context, _ string, email domain.Email) (domain.User, string, error) {
	rec, ok := f.byEmail[email]
	if !ok {
		return domain.User{}, "", glerrors.ErrNotFound
	}
	return rec.user, rec.hash, nil
}

func (f *fakeUsers) FindByID(_ context.Context, userID string) (domain.User, error) {
	u, ok := f.byID[userID]
	if !ok {
		return domain.User{}, glerrors.ErrNotFound
	}
	return u, nil
}

func (f *fakeUsers) FindAuthByID(_ context.Context, userID string) (domain.User, string, error) {
	rec, ok := f.byEmail[f.byID[userID].Email()]
	if !ok {
		return domain.User{}, "", glerrors.ErrNotFound
	}
	return rec.user, rec.hash, nil
}

func (f *fakeUsers) UpdatePassword(_ context.Context, _, _ string) error { return nil }

func (f *fakeUsers) AddRole(_ context.Context, userID, roleID string) error {
	f.added = append(f.added, [2]string{userID, roleID})
	return nil
}

func (f *fakeUsers) RemoveRole(_ context.Context, userID, roleID string) error {
	f.removed = append(f.removed, [2]string{userID, roleID})
	return nil
}

func (f *fakeUsers) CreateUser(_ context.Context, user domain.User, passwordHash string) error {
	if _, dup := f.byEmail[user.Email()]; dup {
		return glerrors.New(glerrors.KindConflict, "email already exists")
	}
	f.created = append(f.created, createdUser{user: user, hash: passwordHash})
	return nil
}

func (f *fakeUsers) ListUsers(_ context.Context) ([]domain.User, error) {
	return f.listOut, nil
}

func (f *fakeUsers) SetStatus(_ context.Context, userID string, status domain.Status) error {
	if f.statusSet == nil {
		f.statusSet = map[string]domain.Status{}
	}
	f.statusSet[userID] = status
	return nil
}

func (f *fakeUsers) SetUserRoles(_ context.Context, userID string, roleIDs []string) error {
	if f.rolesSet == nil {
		f.rolesSet = map[string][]string{}
	}
	f.rolesSet[userID] = roleIDs
	return nil
}

type fakeRoles struct {
	byID map[string]domain.Role

	created  []domain.Role       // CreateRole calls
	listOut  []domain.Role       // ListRoles return
	permsSet map[string][]string // SetPermissions calls
}

func (f *fakeRoles) FindByID(_ context.Context, roleID string) (domain.Role, error) {
	r, ok := f.byID[roleID]
	if !ok {
		return domain.Role{}, glerrors.ErrNotFound
	}
	return r, nil
}

func (f *fakeRoles) CreateRole(_ context.Context, role domain.Role) error {
	for _, existing := range f.byID {
		if existing.Name() == role.Name() {
			return glerrors.New(glerrors.KindConflict, "role name already exists")
		}
	}
	for _, existing := range f.created {
		if existing.Name() == role.Name() {
			return glerrors.New(glerrors.KindConflict, "role name already exists")
		}
	}
	f.created = append(f.created, role)
	return nil
}

func (f *fakeRoles) ListRoles(_ context.Context) ([]domain.Role, error) {
	return f.listOut, nil
}

func (f *fakeRoles) SetPermissions(_ context.Context, roleID string, permissions []string) error {
	if f.permsSet == nil {
		f.permsSet = map[string][]string{}
	}
	f.permsSet[roleID] = permissions
	return nil
}

// fakeHasher treats the stored "hash" as the plaintext prefixed with "h:".
type fakeHasher struct{}

func (fakeHasher) Hash(plain string) (string, error) { return "h:" + plain, nil }
func (fakeHasher) Verify(hash, plain string) error {
	if hash == "h:"+plain {
		return nil
	}
	return glerrors.ErrUnauthorized
}

// fakeTokens is both issuer and validator; tokens are inspectable strings.
type fakeTokens struct {
	lastClaims domain.Claims
	refreshFor string
}

func (t *fakeTokens) IssueAccessToken(_ context.Context, claims domain.Claims) (string, error) {
	t.lastClaims = claims
	return "access:" + claims.Subject, nil
}
func (t *fakeTokens) IssueRefreshToken(_ context.Context, userID string) (string, error) {
	return "refresh:" + userID, nil
}
func (t *fakeTokens) ValidateAccessToken(_ context.Context, token string) (domain.Claims, error) {
	if token == "access:u1" {
		return domain.Claims{Subject: "u1", Permissions: []string{"students:read"}}, nil
	}
	return domain.Claims{}, glerrors.ErrUnauthorized
}
func (t *fakeTokens) ParseRefreshToken(_ context.Context, token string) (string, error) {
	if token == "refresh:u1" {
		return "u1", nil
	}
	return "", glerrors.ErrUnauthorized
}

// --- fixtures --------------------------------------------------------------

func fixtureService(t *testing.T, status domain.Status) (*usecase.AuthService, *fakeUsers) {
	t.Helper()
	email, _ := domain.NewEmail("ana@garcialorca.mx")
	role, _ := domain.NewRole("r1", "Docente", []string{"students:read"})
	user, _ := domain.NewUser("u1", testTenant, email, "Ana", status, false, []domain.Role{role})
	users := &fakeUsers{
		byEmail: map[domain.Email]struct {
			user domain.User
			hash string
		}{email: {user: user, hash: "h:secret"}},
		byID: map[string]domain.User{"u1": user},
	}
	roles := &fakeRoles{byID: map[string]domain.Role{"r1": role, "r2": role}}
	now := func() time.Time { return time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC) }
	svc := usecase.NewAuthService(users, roles, fakeHasher{}, &fakeTokens{}, &fakeTokens{}, testTenant, 15*time.Minute, now)
	return svc, users
}

// --- Authenticate ----------------------------------------------------------

func TestAuthenticateSuccess(t *testing.T) {
	svc, _ := fixtureService(t, domain.StatusActive)
	res, err := svc.Authenticate(context.Background(), "Ana@Garcialorca.MX", "secret")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if res.AccessToken != "access:u1" || res.RefreshToken != "refresh:u1" {
		t.Errorf("tokens = %q / %q", res.AccessToken, res.RefreshToken)
	}
	if res.ExpiresIn != 900 {
		t.Errorf("ExpiresIn = %d, want 900", res.ExpiresIn)
	}
	if res.User.ID() != "u1" {
		t.Errorf("User.ID = %q", res.User.ID())
	}
}

func TestAuthenticateSurfacesMustChangePassword(t *testing.T) {
	// The flag now has a single source of truth: the domain User. Authenticate
	// must read it off the returned user, not a separate repository return value.
	email, _ := domain.NewEmail("ana@garcialorca.mx")
	role, _ := domain.NewRole("r1", "Docente", []string{"students:read"})
	user, _ := domain.NewUser("u1", testTenant, email, "Ana", domain.StatusActive, true, []domain.Role{role})
	users := &fakeUsers{
		byEmail: map[domain.Email]struct {
			user domain.User
			hash string
		}{email: {user: user, hash: "h:secret"}},
		byID: map[string]domain.User{"u1": user},
	}
	roles := &fakeRoles{byID: map[string]domain.Role{"r1": role}}
	now := func() time.Time { return time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC) }
	svc := usecase.NewAuthService(users, roles, fakeHasher{}, &fakeTokens{}, &fakeTokens{}, testTenant, 15*time.Minute, now)

	res, err := svc.Authenticate(context.Background(), "ana@garcialorca.mx", "secret")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if !res.MustChangePassword {
		t.Error("AuthResult.MustChangePassword = false, want true")
	}
}

func TestAuthenticateWrongPasswordIsGenericUnauthorized(t *testing.T) {
	svc, _ := fixtureService(t, domain.StatusActive)
	_, err := svc.Authenticate(context.Background(), "ana@garcialorca.mx", "wrong")
	if glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Errorf("want KindUnauthorized, got %v", err)
	}
}

func TestAuthenticateUnknownEmailIsGenericUnauthorized(t *testing.T) {
	svc, _ := fixtureService(t, domain.StatusActive)
	// Same error kind as wrong-password: do NOT leak whether the email exists.
	_, err := svc.Authenticate(context.Background(), "nobody@garcialorca.mx", "secret")
	if glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Errorf("want KindUnauthorized, got %v", err)
	}
}

func TestAuthenticateSuspendedIsForbidden(t *testing.T) {
	svc, _ := fixtureService(t, domain.StatusSuspended)
	_, err := svc.Authenticate(context.Background(), "ana@garcialorca.mx", "secret")
	if glerrors.KindOf(err) != glerrors.KindForbidden {
		t.Errorf("want KindForbidden, got %v", err)
	}
}

func TestAuthenticateMalformedEmailIsInvalid(t *testing.T) {
	svc, _ := fixtureService(t, domain.StatusActive)
	_, err := svc.Authenticate(context.Background(), "not-an-email", "secret")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v", err)
	}
}

// --- CheckPermission -------------------------------------------------------

func TestCheckPermission(t *testing.T) {
	svc, _ := fixtureService(t, domain.StatusActive)
	ok, err := svc.CheckPermission(context.Background(), "access:u1", "students:read")
	if err != nil || !ok {
		t.Errorf("students:read should be allowed: ok=%v err=%v", ok, err)
	}
	ok, err = svc.CheckPermission(context.Background(), "access:u1", "students:delete")
	if err != nil || ok {
		t.Errorf("students:delete should be denied: ok=%v err=%v", ok, err)
	}
}

func TestCheckPermissionInvalidToken(t *testing.T) {
	svc, _ := fixtureService(t, domain.StatusActive)
	_, err := svc.CheckPermission(context.Background(), "garbage", "students:read")
	if glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Errorf("want KindUnauthorized, got %v", err)
	}
}

func TestValidateTokenReturnsClaims(t *testing.T) {
	svc, _ := fixtureService(t, domain.StatusActive)
	claims, err := svc.ValidateToken(context.Background(), "access:u1")
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.Subject != "u1" || !claims.Allows("students:read") {
		t.Errorf("claims mismatch: %+v", claims)
	}
}

func TestValidateTokenInvalid(t *testing.T) {
	svc, _ := fixtureService(t, domain.StatusActive)
	if _, err := svc.ValidateToken(context.Background(), "garbage"); glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Errorf("want KindUnauthorized, got %v", err)
	}
}

// --- RefreshToken ----------------------------------------------------------

func TestRefreshTokenRotates(t *testing.T) {
	svc, _ := fixtureService(t, domain.StatusActive)
	res, err := svc.RefreshToken(context.Background(), "refresh:u1")
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if res.AccessToken != "access:u1" || res.RefreshToken != "refresh:u1" {
		t.Errorf("rotated tokens = %q / %q", res.AccessToken, res.RefreshToken)
	}
}

func TestRefreshTokenInvalid(t *testing.T) {
	svc, _ := fixtureService(t, domain.StatusActive)
	_, err := svc.RefreshToken(context.Background(), "garbage")
	if glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Errorf("want KindUnauthorized, got %v", err)
	}
}

func TestRefreshTokenSuspendedIsForbidden(t *testing.T) {
	svc, _ := fixtureService(t, domain.StatusSuspended)
	_, err := svc.RefreshToken(context.Background(), "refresh:u1")
	if glerrors.KindOf(err) != glerrors.KindForbidden {
		t.Errorf("want KindForbidden, got %v", err)
	}
}

// --- AssignRole ------------------------------------------------------------

func TestAssignRole(t *testing.T) {
	svc, users := fixtureService(t, domain.StatusActive)
	if err := svc.AssignRole(context.Background(), "u1", "r2"); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}
	if len(users.added) != 1 || users.added[0] != [2]string{"u1", "r2"} {
		t.Errorf("added = %v, want [[u1 r2]]", users.added)
	}
}

func TestAssignRoleUnknownRoleIsNotFound(t *testing.T) {
	svc, _ := fixtureService(t, domain.StatusActive)
	err := svc.AssignRole(context.Background(), "u1", "nope")
	if glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("want KindNotFound, got %v", err)
	}
}

func TestRevokeRole(t *testing.T) {
	svc, users := fixtureService(t, domain.StatusActive)
	if err := svc.RevokeRole(context.Background(), "u1", "r1"); err != nil {
		t.Fatalf("RevokeRole: %v", err)
	}
	if len(users.removed) != 1 || users.removed[0] != [2]string{"u1", "r1"} {
		t.Errorf("removed = %v, want [[u1 r1]]", users.removed)
	}
}

// failingIssuer errors on access-token issuance to exercise the issue error path.
type failingIssuer struct{}

func (failingIssuer) IssueAccessToken(context.Context, domain.Claims) (string, error) {
	return "", glerrors.New(glerrors.KindInternal, "signing failed")
}
func (failingIssuer) IssueRefreshToken(context.Context, string) (string, error) {
	return "refresh", nil
}

func TestAuthenticatePropagatesIssuerError(t *testing.T) {
	email, _ := domain.NewEmail("ana@garcialorca.mx")
	user, _ := domain.NewUser("u1", testTenant, email, "Ana", domain.StatusActive, false, nil)
	users := &fakeUsers{
		byEmail: map[domain.Email]struct {
			user domain.User
			hash string
		}{email: {user: user, hash: "h:secret"}},
		byID: map[string]domain.User{"u1": user},
	}
	now := func() time.Time { return time.Unix(0, 0).UTC() }
	svc := usecase.NewAuthService(users, &fakeRoles{}, fakeHasher{}, failingIssuer{}, &fakeTokens{}, testTenant, time.Minute, now)

	_, err := svc.Authenticate(context.Background(), "ana@garcialorca.mx", "secret")
	if glerrors.KindOf(err) != glerrors.KindInternal {
		t.Errorf("want KindInternal from issuer, got %v", err)
	}
}
