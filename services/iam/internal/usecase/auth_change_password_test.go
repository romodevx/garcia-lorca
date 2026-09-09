package usecase_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/usecase"
)

const (
	cpTenantID    = "00000000-0000-0000-0000-000000000001"
	cpSubject     = "d1"
	cpValidToken  = "valid-token"
	cpCurrentPass = "test-owner-pw"
	cpNewPass     = "test-new-pw-123"
)

type cpUsers struct {
	user    domain.User
	hash    string
	newHash string
	updated bool
	findErr error
}

func (f *cpUsers) FindAuthByEmail(context.Context, string, domain.Email) (domain.User, string, error) {
	return domain.User{}, "", glerrors.ErrNotFound
}
func (f *cpUsers) FindByID(context.Context, string) (domain.User, error) { return f.user, f.findErr }
func (f *cpUsers) FindAuthByID(context.Context, string) (domain.User, string, error) {
	return f.user, f.hash, f.findErr
}
func (f *cpUsers) UpdatePassword(_ context.Context, _, newHash string) error {
	f.updated, f.newHash = true, newHash
	return nil
}
func (f *cpUsers) AddRole(context.Context, string, string) error    { return nil }
func (f *cpUsers) RemoveRole(context.Context, string, string) error { return nil }

// Admin-surface methods are unused by the change-password flow; present only so
// cpUsers keeps satisfying the extended ports.UserRepository.
func (f *cpUsers) CreateUser(context.Context, domain.User, string) error  { return nil }
func (f *cpUsers) ListUsers(context.Context) ([]domain.User, error)       { return nil, nil }
func (f *cpUsers) SetStatus(context.Context, string, domain.Status) error { return nil }
func (f *cpUsers) SetUserRoles(context.Context, string, []string) error   { return nil }

// hasher fake: "H:"+plain is the "hash"; Verify matches that shape.
type cpHasher struct{}

func (cpHasher) Hash(plain string) (string, error) { return "H:" + plain, nil }
func (cpHasher) Verify(hash, plain string) error {
	if hash == "H:"+plain {
		return nil
	}
	return glerrors.New(glerrors.KindUnauthorized, "invalid credentials")
}

// cpValidator resolves cpValidToken to a fixed subject; anything else is
// Unauthorized. ChangePassword derives the subject from the verified token, never
// from a caller-supplied id.
type cpValidator struct{}

func (cpValidator) ValidateAccessToken(_ context.Context, token string) (domain.Claims, error) {
	if token == cpValidToken {
		return domain.Claims{Subject: cpSubject, TenantID: cpTenantID}, nil
	}
	return domain.Claims{}, glerrors.New(glerrors.KindUnauthorized, "invalid access token")
}
func (cpValidator) ParseRefreshToken(context.Context, string) (string, time.Time, domain.SessionKind, error) {
	return "", time.Time{}, domain.SessionStaff, glerrors.New(glerrors.KindUnauthorized, "not a refresh token")
}

func newCPService(users *cpUsers) *usecase.AuthService {
	// roles/issuer are unused by ChangePassword; pass nil where safe.
	return usecase.NewAuthService(users, nil, cpHasher{}, nil, cpValidator{}, nil, nil,
		cpTenantID, 15*time.Minute, time.Now)
}

func newCPUser(t *testing.T) domain.User {
	t.Helper()
	email, _ := domain.NewEmail("owner@example.com")
	u, _ := domain.NewUser(cpSubject, cpTenantID, email, "Owner", domain.StatusActive, true, nil)
	return u
}

func TestChangePasswordSuccessClearsFlag(t *testing.T) {
	users := &cpUsers{user: newCPUser(t), hash: "H:" + cpCurrentPass}
	svc := newCPService(users)

	err := svc.ChangePassword(context.Background(), cpValidToken, cpCurrentPass, cpNewPass)
	if err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if !users.updated || users.newHash != "H:"+cpNewPass {
		t.Errorf("updated=%v newHash=%q", users.updated, users.newHash)
	}
}

func TestChangePasswordWrongCurrentIsUnauthorized(t *testing.T) {
	users := &cpUsers{user: newCPUser(t), hash: "H:" + cpCurrentPass}
	svc := newCPService(users)

	err := svc.ChangePassword(context.Background(), cpValidToken, "wrong", cpNewPass)
	if glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Errorf("KindOf = %v, want Unauthorized", glerrors.KindOf(err))
	}
	if users.updated {
		t.Error("password updated despite wrong current password")
	}
}

func TestChangePasswordInvalidTokenIsUnauthorized(t *testing.T) {
	users := &cpUsers{user: newCPUser(t), hash: "H:" + cpCurrentPass}
	svc := newCPService(users)

	err := svc.ChangePassword(context.Background(), "bogus-token", cpCurrentPass, cpNewPass)
	if glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Errorf("KindOf = %v, want Unauthorized", glerrors.KindOf(err))
	}
	if users.updated {
		t.Error("password updated despite invalid access token")
	}
}

func TestChangePasswordRejectsShortNew(t *testing.T) {
	users := &cpUsers{user: newCPUser(t), hash: "H:" + cpCurrentPass}
	svc := newCPService(users)

	err := svc.ChangePassword(context.Background(), cpValidToken, cpCurrentPass, "short")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("KindOf = %v, want Invalid", glerrors.KindOf(err))
	}
}
