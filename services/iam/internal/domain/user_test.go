package domain_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
)

func mustEmail(t *testing.T, s string) domain.Email {
	t.Helper()
	e, err := domain.NewEmail(s)
	if err != nil {
		t.Fatalf("NewEmail(%q): %v", s, err)
	}
	return e
}

func mustRole(t *testing.T, id, name string, perms []string) domain.Role {
	t.Helper()
	r, err := domain.NewRole(id, name, perms)
	if err != nil {
		t.Fatalf("NewRole(%q): %v", id, err)
	}
	return r
}

func TestNewUserRequiresIdAndTenant(t *testing.T) {
	email := mustEmail(t, "director@garcialorca.mx")
	for _, tc := range []struct {
		name, id, tenant string
	}{
		{"blank id", "", "t1"},
		{"blank tenant", "u1", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewUser(tc.id, tc.tenant, email, "Dir", domain.StatusActive, nil)
			if err == nil || glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("want KindInvalid, got %v", err)
			}
		})
	}
}

func TestUserEffectivePermissionsUnionAcrossRoles(t *testing.T) {
	email := mustEmail(t, "ana@garcialorca.mx")
	teacher := mustRole(t, "r1", "Docente", []string{"students:read", "grades:create"})
	tutor := mustRole(t, "r2", "Tutor", []string{"students:read", "attendance:read"})

	u, err := domain.NewUser("u1", "t1", email, "Ana", domain.StatusActive, []domain.Role{teacher, tutor})
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}

	// Union, deduplicated: students:read, grades:create, attendance:read = 3.
	if n := len(u.EffectivePermissions()); n != 3 {
		t.Errorf("EffectivePermissions len = %d, want 3", n)
	}

	read, _ := domain.NewPermission("students:read")
	create, _ := domain.NewPermission("grades:create")
	absent, _ := domain.NewPermission("finance:read")
	if !u.HasPermission(read) || !u.HasPermission(create) {
		t.Errorf("user should hold students:read and grades:create")
	}
	if u.HasPermission(absent) {
		t.Errorf("user should NOT hold finance:read")
	}
}

func TestUserCanAuthenticateOnlyWhenActive(t *testing.T) {
	email := mustEmail(t, "sus@garcialorca.mx")
	active, _ := domain.NewUser("u1", "t1", email, "A", domain.StatusActive, nil)
	suspended, _ := domain.NewUser("u2", "t1", email, "S", domain.StatusSuspended, nil)

	if !active.CanAuthenticate() {
		t.Errorf("active user should be able to authenticate")
	}
	if suspended.CanAuthenticate() {
		t.Errorf("suspended user should NOT be able to authenticate")
	}
}

func TestUserAccessorsAreStable(t *testing.T) {
	email := mustEmail(t, "x@garcialorca.mx")
	u, _ := domain.NewUser("u9", "t1", email, "Equis", domain.StatusActive, nil)
	if u.ID() != "u9" || u.TenantID() != "t1" || u.Email() != email || u.DisplayName() != "Equis" {
		t.Errorf("accessors mismatch: %q/%q/%q/%q", u.ID(), u.TenantID(), u.Email(), u.DisplayName())
	}
}
