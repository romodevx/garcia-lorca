package jwt_test

import (
	"context"
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/adapters/jwt"
	"github.com/garcia-lorca/services/iam/internal/domain"
)

// A throwaway signing key for tests only — built at runtime from an obvious
// placeholder so it is never a real credential.
var secret = strings.Repeat("gl-test-key-", 4)

var t0 = time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

func adapterAt(now time.Time) *jwt.HMAC {
	return jwt.NewHMAC([]byte(secret), "garcia-lorca", 24*time.Hour, func() time.Time { return now })
}

func sampleClaims() domain.Claims {
	return domain.Claims{
		Subject:     "u1",
		TenantID:    "t1",
		Email:       "ana@garcialorca.mx",
		Permissions: []string{"students:read", "grades:create"},
		ExpiresAt:   t0.Add(15 * time.Minute),
	}
}

func TestAccessTokenRoundTrip(t *testing.T) {
	a := adapterAt(t0)
	ctx := context.Background()

	token, err := a.IssueAccessToken(ctx, sampleClaims())
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}

	got, err := a.ValidateAccessToken(ctx, token)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if got.Subject != "u1" || got.TenantID != "t1" || got.Email != "ana@garcialorca.mx" {
		t.Errorf("principal mismatch: %+v", got)
	}
	if len(got.Permissions) != 2 || got.Permissions[0] != "students:read" {
		t.Errorf("permissions mismatch: %v", got.Permissions)
	}
	if got.ExpiresAt.Unix() != t0.Add(15*time.Minute).Unix() {
		t.Errorf("ExpiresAt = %v, want %v", got.ExpiresAt, t0.Add(15*time.Minute))
	}
}

func TestAccessTokenCarriesMustChangePassword(t *testing.T) {
	a := adapterAt(t0)
	ctx := context.Background()

	claims := sampleClaims()
	claims.MustChangePassword = true

	token, err := a.IssueAccessToken(ctx, claims)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}

	got, err := a.ValidateAccessToken(ctx, token)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if !got.MustChangePassword {
		t.Error("MustChangePassword = false, want true after issue→validate round-trip")
	}
}

func TestExpiredAccessTokenIsUnauthorized(t *testing.T) {
	issuer := adapterAt(t0)
	token, _ := issuer.IssueAccessToken(context.Background(), sampleClaims())

	// Validate 16 minutes later (past the 15-minute expiry).
	validator := adapterAt(t0.Add(16 * time.Minute))
	_, err := validator.ValidateAccessToken(context.Background(), token)
	if glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Errorf("want KindUnauthorized for expired token, got %v", err)
	}
}

func TestWrongSecretIsUnauthorized(t *testing.T) {
	token, _ := adapterAt(t0).IssueAccessToken(context.Background(), sampleClaims())

	other := jwt.NewHMAC([]byte(strings.Repeat("gl-other-key-", 4)), "garcia-lorca", time.Hour, func() time.Time { return t0 })
	_, err := other.ValidateAccessToken(context.Background(), token)
	if glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Errorf("want KindUnauthorized for bad signature, got %v", err)
	}
}

func TestGarbageTokenIsUnauthorized(t *testing.T) {
	a := adapterAt(t0)
	if _, err := a.ValidateAccessToken(context.Background(), "not.a.jwt"); glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Errorf("want KindUnauthorized, got %v", err)
	}
}

func TestRefreshTokenRoundTrip(t *testing.T) {
	a := adapterAt(t0)
	ctx := context.Background()

	rt, err := a.IssueRefreshToken(ctx, "u1", domain.SessionStaff, 7)
	if err != nil {
		t.Fatalf("IssueRefreshToken: %v", err)
	}
	got, err := a.ParseRefreshToken(ctx, rt)
	if err != nil {
		t.Fatalf("ParseRefreshToken: %v", err)
	}
	if got.Kind != domain.SessionStaff {
		t.Errorf("kind = %v, want staff", got.Kind)
	}
	if got.UserID != "u1" {
		t.Errorf("uid = %q, want u1", got.UserID)
	}
	if !got.IssuedAt.Equal(t0) {
		t.Errorf("issuedAt = %v, want %v", got.IssuedAt, t0)
	}
	if got.CredentialVersion != 7 {
		t.Errorf("credential version = %d, want 7 (CC-302)", got.CredentialVersion)
	}
}

func TestTokenTypesAreNotInterchangeable(t *testing.T) {
	a := adapterAt(t0)
	ctx := context.Background()

	access, _ := a.IssueAccessToken(ctx, sampleClaims())
	refresh, _ := a.IssueRefreshToken(ctx, "u1", domain.SessionStaff, 0)

	// A refresh token must not validate as an access token...
	if _, err := a.ValidateAccessToken(ctx, refresh); glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Errorf("refresh accepted as access: %v", err)
	}
	// ...and an access token must not parse as a refresh token.
	if _, err := a.ParseRefreshToken(ctx, access); glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Errorf("access accepted as refresh: %v", err)
	}
}

func TestExpiredRefreshTokenIsUnauthorized(t *testing.T) {
	issuer := adapterAt(t0) // staff refresh TTL = 24 h
	rt, _ := issuer.IssueRefreshToken(context.Background(), "u1", domain.SessionStaff, 0)

	validator := adapterAt(t0.Add(25 * time.Hour))
	if _, err := validator.ParseRefreshToken(context.Background(), rt); glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Errorf("want KindUnauthorized for expired refresh, got %v", err)
	}
}
