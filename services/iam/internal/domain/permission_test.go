package domain_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
)

func TestNewPermissionParsesResourceAction(t *testing.T) {
	tests := []struct {
		in       string
		resource string
		action   string
		want     string
	}{
		{"students:read", "students", "read", "students:read"},
		{" Students:Create ", "students", "create", "students:create"},
		{"finance.payment:create", "finance.payment", "create", "finance.payment:create"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			p, err := domain.NewPermission(tt.in)
			if err != nil {
				t.Fatalf("NewPermission(%q) error: %v", tt.in, err)
			}
			if p.Resource() != tt.resource || p.Action() != tt.action {
				t.Errorf("got resource=%q action=%q, want %q/%q", p.Resource(), p.Action(), tt.resource, tt.action)
			}
			if p.String() != tt.want {
				t.Errorf("String() = %q, want %q", p.String(), tt.want)
			}
		})
	}
}

func TestNewPermissionRejectsMalformed(t *testing.T) {
	for _, in := range []string{"", "students", "students:", ":read", "a:b:c", "  :  "} {
		if _, err := domain.NewPermission(in); err == nil || glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Errorf("NewPermission(%q): want KindInvalid, got %v", in, err)
		}
	}
}

func TestRoleHasPermission(t *testing.T) {
	role, err := domain.NewRole("role-1", "Docente", []string{"students:read", "grades:create"})
	if err != nil {
		t.Fatalf("NewRole error: %v", err)
	}
	read, _ := domain.NewPermission("students:read")
	del, _ := domain.NewPermission("students:delete")
	if !role.HasPermission(read) {
		t.Errorf("role should have students:read")
	}
	if role.HasPermission(del) {
		t.Errorf("role should NOT have students:delete")
	}
}

func TestRoleDeduplicatesAndValidatesPermissions(t *testing.T) {
	role, err := domain.NewRole("r", "R", []string{"students:read", "students:read", "grades:create"})
	if err != nil {
		t.Fatalf("NewRole error: %v", err)
	}
	if n := len(role.Permissions()); n != 2 {
		t.Errorf("Permissions() len = %d, want 2 (deduplicated)", n)
	}

	if _, err := domain.NewRole("r", "R", []string{"bad-perm"}); err == nil || glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("NewRole with malformed permission: want KindInvalid, got %v", err)
	}
}

func TestNewRoleRequiresName(t *testing.T) {
	if _, err := domain.NewRole("r", "  ", nil); err == nil || glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("NewRole with blank name: want KindInvalid, got %v", err)
	}
}
