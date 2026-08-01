package usecase_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/usecase"
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

// hasher fake: "H:"+plain is the "hash"; Verify matches that shape.
type cpHasher struct{}

func (cpHasher) Hash(plain string) (string, error) { return "H:" + plain, nil }
func (cpHasher) Verify(hash, plain string) error {
	if hash == "H:"+plain {
		return nil
	}
	return glerrors.New(glerrors.KindUnauthorized, "invalid credentials")
}

func newCPService(users *cpUsers) *usecase.AuthService {
	// roles/issuer/validator are unused by ChangePassword; pass nil where safe.
	return usecase.NewAuthService(users, nil, cpHasher{}, nil, nil,
		"00000000-0000-0000-0000-000000000001", 15*time.Minute, time.Now)
}

func TestChangePasswordSuccessClearsFlag(t *testing.T) {
	email, _ := domain.NewEmail("director@garcialorca.edu.mx")
	u, _ := domain.NewUser("d1", "00000000-0000-0000-0000-000000000001", email, "Director", domain.StatusActive, true, nil)
	users := &cpUsers{user: u, hash: "H:test-owner-pw"}
	svc := newCPService(users)

	err := svc.ChangePassword(context.Background(), "d1", "test-owner-pw", "test-new-pw-123")
	if err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if !users.updated || users.newHash != "H:test-new-pw-123" {
		t.Errorf("updated=%v newHash=%q", users.updated, users.newHash)
	}
}

func TestChangePasswordWrongCurrentIsUnauthorized(t *testing.T) {
	email, _ := domain.NewEmail("director@garcialorca.edu.mx")
	u, _ := domain.NewUser("d1", "00000000-0000-0000-0000-000000000001", email, "Director", domain.StatusActive, true, nil)
	users := &cpUsers{user: u, hash: "H:test-owner-pw"}
	svc := newCPService(users)

	err := svc.ChangePassword(context.Background(), "d1", "wrong", "test-new-pw-123")
	if glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Errorf("KindOf = %v, want Unauthorized", glerrors.KindOf(err))
	}
	if users.updated {
		t.Error("password updated despite wrong current password")
	}
}

func TestChangePasswordRejectsShortNew(t *testing.T) {
	email, _ := domain.NewEmail("director@garcialorca.edu.mx")
	u, _ := domain.NewUser("d1", "00000000-0000-0000-0000-000000000001", email, "Director", domain.StatusActive, true, nil)
	users := &cpUsers{user: u, hash: "H:test-owner-pw"}
	svc := newCPService(users)

	err := svc.ChangePassword(context.Background(), "d1", "test-owner-pw", "short")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("KindOf = %v, want Invalid", glerrors.KindOf(err))
	}
}
