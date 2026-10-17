package usecase_test

import (
	"context"
	"slices"
	"strings"
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

	// findByIDErr, when set, is what FindByID fails with instead of looking the
	// user up. It exists to stand in for the database being unavailable, which
	// is a different answer from "there is no such user" and must stay so all
	// the way out of the service.
	findByIDErr error
	// findByEmailErr, when set, is what FindAuthByEmail fails with.
	findByEmailErr error

	// admin-surface recording
	created      []createdUser            // CreateUser calls
	listOut      []domain.User            // ListUsers return
	statusSet    map[string]domain.Status // SetStatus calls
	statusKeep   map[string]string        // SetStatus calls: user -> keepActiveHolderOf
	passwordsSet map[string]string        // UpdatePassword calls
	rotated      map[string]bool          // TouchPasswordChangedAt calls
	rolesSet     map[string][]string      // SetUserRoles calls
	deletedBy    map[string]string        // SoftDelete calls: target -> actor
	restored     map[string]string        // Restore calls: user -> display name
	restoredRole map[string][]string      // Restore calls: user -> roles set atomically
}

func (f *fakeUsers) FindAuthByEmail(_ context.Context, _ string, email domain.Email) (domain.User, string, error) {
	if f.findByEmailErr != nil {
		return domain.User{}, "", f.findByEmailErr
	}
	rec, ok := f.byEmail[email]
	if !ok {
		return domain.User{}, "", glerrors.ErrNotFound
	}
	return rec.user, rec.hash, nil
}

func (f *fakeUsers) FindByID(_ context.Context, userID string) (domain.User, error) {
	if f.findByIDErr != nil {
		return domain.User{}, f.findByIDErr
	}
	u, ok := f.byID[userID]
	if !ok {
		// Postgres reads a uuid in any case; so does this double, so a test
		// can send another spelling of the same id.
		u, ok = f.byID[strings.ToLower(userID)]
	}
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

func (f *fakeUsers) UpdatePassword(_ context.Context, userID, hash string) error {
	// RECORD it. Returning nil and forgetting made this double unable to tell
	// "the password was stored" from "nothing happened", which is the exact
	// difference between an activated account and an active one with no
	// credential at all.
	if f.passwordsSet == nil {
		f.passwordsSet = map[string]string{}
	}
	f.passwordsSet[userID] = hash
	return nil
}

func (f *fakeUsers) TouchPasswordChangedAt(_ context.Context, userID string) error {
	if f.rotated == nil {
		f.rotated = map[string]bool{}
	}
	f.rotated[userID] = true
	return nil
}

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
	// Register it, the way a repository does. Recording the call without storing
	// the row made this double lie about the most basic thing a repository
	// promises: what you create, you can then find.
	if f.byEmail == nil {
		f.byEmail = map[domain.Email]struct {
			user domain.User
			hash string
		}{}
	}
	if f.byID == nil {
		f.byID = map[string]domain.User{}
	}
	f.byEmail[user.Email()] = struct {
		user domain.User
		hash string
	}{user: user, hash: passwordHash}
	f.byID[user.ID()] = user
	return nil
}

func (f *fakeUsers) ListUsers(_ context.Context) ([]domain.User, error) {
	return f.listOut, nil
}

func (f *fakeUsers) SetStatus(_ context.Context, userID string, status domain.Status, keepActiveHolderOf string) error {
	// A deleted row is out of reach of a plain status change, as in the
	// repository: only Restore brings it back.
	target, ok := f.byID[userID]
	if !ok {
		target, ok = f.byID[strings.ToLower(userID)]
	}
	if ok && target.IsDeleted() {
		return glerrors.ErrNotFound
	}
	// The repository's last-holder rule, as SoftDelete emulates it: leaving
	// `active` is refused for the only active holder of keepActiveHolderOf.
	if ok && status != domain.StatusActive && target.Status() == domain.StatusActive &&
		slices.Contains(target.RoleIDs(), keepActiveHolderOf) && !f.otherActiveHolder(target.ID(), keepActiveHolderOf) {
		return domain.ErrLastOwner
	}
	if f.statusSet == nil {
		f.statusSet = map[string]domain.Status{}
		f.statusKeep = map[string]string{}
	}
	f.statusSet[userID] = status
	f.statusKeep[userID] = keepActiveHolderOf
	return nil
}

// StatusOf reads the account's lifecycle state the way FindByID finds it,
// failing with findByIDErr when that stands in for an unavailable database.
func (f *fakeUsers) StatusOf(ctx context.Context, userID string) (domain.Status, error) {
	u, err := f.FindByID(ctx, userID)
	if err != nil {
		return domain.StatusUnspecified, err
	}
	return u.Status(), nil
}

// SoftDelete emulates the repository's atomic rule: the only ACTIVE holder of
// keepActiveHolderOf cannot be deleted.
func (f *fakeUsers) SoftDelete(_ context.Context, userID, deletedBy, keepActiveHolderOf string) error {
	target, ok := f.byID[userID]
	if !ok || target.IsDeleted() {
		return glerrors.ErrNotFound
	}
	if target.Status() == domain.StatusActive && slices.Contains(target.RoleIDs(), keepActiveHolderOf) && !f.otherActiveHolder(userID, keepActiveHolderOf) {
		return domain.ErrLastOwner
	}
	if f.deletedBy == nil {
		f.deletedBy = map[string]string{}
	}
	f.deletedBy[userID] = deletedBy
	return nil
}

func (f *fakeUsers) otherActiveHolder(userID, roleID string) bool {
	for id, u := range f.byID {
		if id != userID && u.Status() == domain.StatusActive && slices.Contains(u.RoleIDs(), roleID) {
			return true
		}
	}
	return false
}

func (f *fakeUsers) Restore(_ context.Context, userID, displayName string, roleIDs []string) error {
	u, ok := f.byID[userID]
	if !ok || !u.IsDeleted() {
		return glerrors.ErrNotFound
	}
	if f.restored == nil {
		f.restored = map[string]string{}
		f.restoredRole = map[string][]string{}
	}
	f.restored[userID] = displayName
	f.restoredRole[userID] = roleIDs
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
	lastClaims      domain.Claims
	refreshFor      string
	refreshIssuedAt time.Time // issued-at reported by ParseRefreshToken (zero ⇒ no floor)
	// lastKind is the session kind the last refresh token was issued for, and
	// refreshKind is what ParseRefreshToken reports back — the two together are
	// how "a rotation keeps its kind" is observable.
	lastKind    domain.SessionKind
	refreshKind domain.SessionKind
}

func (t *fakeTokens) IssueAccessToken(_ context.Context, claims domain.Claims) (string, error) {
	t.lastClaims = claims
	return "access:" + claims.Subject, nil
}
func (t *fakeTokens) IssueRefreshToken(_ context.Context, userID string, kind domain.SessionKind) (string, error) {
	t.lastKind = kind
	t.refreshKind = kind
	return "refresh:" + userID, nil
}
func (t *fakeTokens) ValidateAccessToken(_ context.Context, token string) (domain.Claims, error) {
	if token == "access:u1" {
		return domain.Claims{Subject: "u1", Permissions: []string{"students:read"}}, nil
	}
	return domain.Claims{}, glerrors.ErrUnauthorized
}
func (t *fakeTokens) ParseRefreshToken(_ context.Context, token string) (string, time.Time, domain.SessionKind, error) {
	if token == "refresh:u1" {
		return "u1", t.refreshIssuedAt, t.refreshKind, nil
	}
	return "", time.Time{}, domain.SessionStaff, glerrors.ErrUnauthorized
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
	svc := usecase.NewAuthService(users, roles, fakeHasher{}, &fakeTokens{}, &fakeTokens{}, nil, nil, testTenant, 15*time.Minute, now)
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
	svc := usecase.NewAuthService(users, roles, fakeHasher{}, &fakeTokens{}, &fakeTokens{}, nil, nil, testTenant, 15*time.Minute, now)

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

// countingHasher records how many times Verify runs, so the timing-equalization
// behavior is testable deterministically (no wall-clock measurement).
type countingHasher struct{ verifyCalls int }

func (c *countingHasher) Hash(plain string) (string, error) { return "h:" + plain, nil }
func (c *countingHasher) Verify(hash, plain string) error {
	c.verifyCalls++
	if hash == "h:"+plain {
		return nil
	}
	return glerrors.ErrUnauthorized
}

// SECURITY: the unknown-email path must perform the SAME password-verification
// work as the known-email path, or the response-time delta (bcrypt runs only
// when the email exists) leaks which emails are registered — defeating the
// generic-error enumeration defense. Deterministic proxy for the timing check:
// Verify must be invoked exactly once even when the email does not exist.
func TestAuthenticateUnknownEmailStillPerformsHashVerification(t *testing.T) {
	users := &fakeUsers{
		byEmail: map[domain.Email]struct {
			user domain.User
			hash string
		}{}, // empty → every email is unknown
		byID: map[string]domain.User{},
	}
	hasher := &countingHasher{}
	now := func() time.Time { return time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC) }
	svc := usecase.NewAuthService(users, &fakeRoles{}, hasher, &fakeTokens{}, &fakeTokens{}, nil, nil, testTenant, 15*time.Minute, now)

	_, err := svc.Authenticate(context.Background(), "nobody@garcialorca.mx", "secret")
	if glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Fatalf("want KindUnauthorized, got %v", err)
	}
	if hasher.verifyCalls != 1 {
		t.Errorf("unknown-email path performed %d password verifications, want 1 (to equalize timing)", hasher.verifyCalls)
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

// refreshFixture builds a service whose one user rotated their password at
// changedAt, and whose refresh token reports refreshIssuedAt. Used to pin the
// revocation-on-password-change cutoff.
func refreshFixture(t *testing.T, changedAt, refreshIssuedAt time.Time) *usecase.AuthService {
	t.Helper()
	email, _ := domain.NewEmail("ana@garcialorca.mx")
	role, _ := domain.NewRole("r1", "Docente", []string{"students:read"})
	user, _ := domain.NewUser("u1", testTenant, email, "Ana", domain.StatusActive, false, []domain.Role{role})
	user = user.WithPasswordChangedAt(changedAt)
	users := &fakeUsers{
		byEmail: map[domain.Email]struct {
			user domain.User
			hash string
		}{email: {user: user, hash: "h:secret"}},
		byID: map[string]domain.User{"u1": user},
	}
	roles := &fakeRoles{byID: map[string]domain.Role{"r1": role}}
	now := func() time.Time { return changedAt.Add(time.Hour) }
	validator := &fakeTokens{refreshIssuedAt: refreshIssuedAt}
	return usecase.NewAuthService(users, roles, fakeHasher{}, &fakeTokens{}, validator, nil, nil, testTenant, 15*time.Minute, now)
}

// SECURITY: a refresh token minted BEFORE the last password change is revoked —
// a stolen token stops working the instant the user rotates their password.
func TestRefreshTokenRevokedByLaterPasswordChange(t *testing.T) {
	changedAt := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	svc := refreshFixture(t, changedAt, changedAt.Add(-time.Minute)) // issued before the change

	_, err := svc.RefreshToken(context.Background(), "refresh:u1")
	if glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Fatalf("a refresh token issued before the password change must be Unauthorized, got %v", err)
	}
}

// A refresh token minted AFTER the change is still honored (only pre-change
// tokens are revoked, so the legitimate current session keeps working).
func TestRefreshTokenAfterPasswordChangeStillValid(t *testing.T) {
	changedAt := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	svc := refreshFixture(t, changedAt, changedAt.Add(time.Minute)) // issued after the change

	res, err := svc.RefreshToken(context.Background(), "refresh:u1")
	if err != nil {
		t.Fatalf("a refresh token issued after the password change must still rotate: %v", err)
	}
	if res.AccessToken != "access:u1" {
		t.Errorf("rotated access token = %q, want access:u1", res.AccessToken)
	}
}

// REGRESSION (runtime-found): a JWT's iat is floored to whole seconds, but
// password_changed_at carries sub-second precision. The re-issued session token
// (minted the same second as the change) has iat = floor(second), which is
// strictly before a sub-second password_changed_at — it must NOT be revoked, or
// changing a password immediately logs the user out. The cutoff compares at
// second granularity to keep the same-second re-issue alive.
func TestRefreshTokenSameSecondAsPasswordChangeSurvives(t *testing.T) {
	// Password changed at 12:00:00.862; the fresh token's iat floors to 12:00:00.
	changedAt := time.Date(2026, 7, 1, 12, 0, 0, 862_000_000, time.UTC)
	issuedAt := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC) // floored iat, same second
	svc := refreshFixture(t, changedAt, issuedAt)

	res, err := svc.RefreshToken(context.Background(), "refresh:u1")
	if err != nil {
		t.Fatalf("a token issued in the same second as the change must survive (no logout): %v", err)
	}
	if res.AccessToken != "access:u1" {
		t.Errorf("rotated access token = %q, want access:u1", res.AccessToken)
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
func (failingIssuer) IssueRefreshToken(context.Context, string, domain.SessionKind) (string, error) {
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
	svc := usecase.NewAuthService(users, &fakeRoles{}, fakeHasher{}, failingIssuer{}, &fakeTokens{}, nil, nil, testTenant, time.Minute, now)

	_, err := svc.Authenticate(context.Background(), "ana@garcialorca.mx", "secret")
	if glerrors.KindOf(err) != glerrors.KindInternal {
		t.Errorf("want KindInternal from issuer, got %v", err)
	}
}
