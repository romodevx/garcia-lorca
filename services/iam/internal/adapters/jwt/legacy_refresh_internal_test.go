package jwt

import (
	"context"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// CC-302: a refresh token minted before credential versions existed carries
// none and reads as version 0 — the version every account starts at — so the
// first rotation after the deploy revokes it.
func TestLegacyRefreshTokenReadsAsVersionZero(t *testing.T) {
	at := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	a := NewHMAC([]byte("legacy-test-signing-material"), "iam", time.Hour, func() time.Time { return at })
	legacy, err := a.sign(refreshClaims{
		RegisteredClaims: gojwt.RegisteredClaims{Subject: "u1", IssuedAt: gojwt.NewNumericDate(at), ExpiresAt: gojwt.NewNumericDate(at.Add(time.Hour))},
		TokenType:        typeRefresh,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.ParseRefreshToken(context.Background(), legacy)
	if err != nil {
		t.Fatalf("ParseRefreshToken: %v", err)
	}
	if got.CredentialVersion != 0 {
		t.Errorf("legacy version = %d, want 0", got.CredentialVersion)
	}
}
