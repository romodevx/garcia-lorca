package domain_test

import (
	"testing"
	"time"

	"github.com/garcia-lorca/services/iam/internal/domain"
)

func TestNewAccessClaimsFromUser(t *testing.T) {
	email := mustEmail(t, "ana@garcialorca.mx")
	teacher := mustRole(t, "r1", "Docente", []string{"students:read", "grades:create"})
	u, err := domain.NewUser("u1", "t1", email, "Ana", domain.StatusActive, false, []domain.Role{teacher})
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}

	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	ttl := 15 * time.Minute
	claims := domain.NewAccessClaims(u, now, ttl)

	if claims.Subject != "u1" {
		t.Errorf("Subject = %q, want u1", claims.Subject)
	}
	if claims.TenantID != "t1" {
		t.Errorf("TenantID = %q, want t1", claims.TenantID)
	}
	if claims.Email != "ana@garcialorca.mx" {
		t.Errorf("Email = %q", claims.Email)
	}
	if !claims.ExpiresAt.Equal(now.Add(ttl)) {
		t.Errorf("ExpiresAt = %v, want %v", claims.ExpiresAt, now.Add(ttl))
	}
	if len(claims.Permissions) != 2 {
		t.Errorf("Permissions = %v, want 2 entries", claims.Permissions)
	}
}

func TestNewAccessClaimsCarriesMustChangePassword(t *testing.T) {
	email, _ := domain.NewEmail("owner@example.com")
	u, _ := domain.NewUser("u1", "t1", email, "Owner", domain.StatusActive, true, nil)
	c := domain.NewAccessClaims(u, time.Unix(0, 0), time.Minute)
	if !c.MustChangePassword {
		t.Error("Claims.MustChangePassword = false, want true")
	}
}

func TestClaimsPermissionsAreCanonicalStrings(t *testing.T) {
	email := mustEmail(t, "b@garcialorca.mx")
	role := mustRole(t, "r", "R", []string{"students:read"})
	u, _ := domain.NewUser("u", "t", email, "B", domain.StatusActive, false, []domain.Role{role})
	claims := domain.NewAccessClaims(u, time.Unix(0, 0).UTC(), time.Hour)
	if claims.Permissions[0] != "students:read" {
		t.Errorf("permission string = %q, want students:read", claims.Permissions[0])
	}
}

func TestClaimsExpiryChecks(t *testing.T) {
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	claims := domain.Claims{Subject: "u", ExpiresAt: base}
	if claims.IsExpiredAt(base.Add(-time.Second)) {
		t.Errorf("should not be expired one second before expiry")
	}
	if !claims.IsExpiredAt(base.Add(time.Second)) {
		t.Errorf("should be expired one second after expiry")
	}
}

func TestClaimsAllows(t *testing.T) {
	claims := domain.Claims{Permissions: []string{"students:read", "grades:create"}}
	if !claims.Allows("students:read") {
		t.Errorf("Allows(students:read) should be true")
	}
	if claims.Allows("students:delete") {
		t.Errorf("Allows(students:delete) should be false")
	}
}
