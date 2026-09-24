package domain_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func TestNewTeacherValidates(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		tenantID  string
		firstName string
		lastName  string
		email     string
		userID    string
		wantKind  glerrors.Kind
	}{
		{"empty id", "", "t1", "Ada", "Lovelace", "ada@example.com", "", glerrors.KindInvalid},
		{"empty tenant", "te1", "", "Ada", "Lovelace", "ada@example.com", "", glerrors.KindInvalid},
		{"empty first name", "te1", "t1", "  ", "Lovelace", "ada@example.com", "", glerrors.KindInvalid},
		{"empty last name", "te1", "t1", "Ada", "", "ada@example.com", "", glerrors.KindInvalid},
		{"empty email", "te1", "t1", "Ada", "Lovelace", "", "", glerrors.KindInvalid},
		{"email without at", "te1", "t1", "Ada", "Lovelace", "ada.example.com", "", glerrors.KindInvalid},
		{"email without dotted domain", "te1", "t1", "Ada", "Lovelace", "ada@example", "", glerrors.KindInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewTeacher(tt.id, tt.tenantID, tt.firstName, tt.lastName, tt.email, tt.userID)
			if glerrors.KindOf(err) != tt.wantKind {
				t.Fatalf("want %v, got %v", tt.wantKind, glerrors.KindOf(err))
			}
		})
	}
}

func TestNewTeacherValidWithoutUser(t *testing.T) {
	te, err := domain.NewTeacher("te1", "t1", "Ada", "Lovelace", "ada@example.com", "")
	if err != nil {
		t.Fatalf("valid teacher without user: %v", err)
	}
	if te.ID() != "te1" || te.TenantID() != "t1" || te.FirstName() != "Ada" || te.LastName() != "Lovelace" {
		t.Errorf("identity accessors: %q %q %q %q", te.ID(), te.TenantID(), te.FirstName(), te.LastName())
	}
	if te.Email() != "ada@example.com" {
		t.Errorf("email accessor: %q", te.Email())
	}
	// userID is optional; empty means the teacher has no IAM link yet.
	if te.UserID() != "" || te.HasUser() {
		t.Errorf("expected no user link, got UserID=%q HasUser=%v", te.UserID(), te.HasUser())
	}
}

func TestNewTeacherValidWithUser(t *testing.T) {
	te, err := domain.NewTeacher("te1", "t1", "Ada", "Lovelace", "ada@example.com", "user-1")
	if err != nil {
		t.Fatalf("valid teacher with user: %v", err)
	}
	if te.UserID() != "user-1" || !te.HasUser() {
		t.Errorf("expected user link, got UserID=%q HasUser=%v", te.UserID(), te.HasUser())
	}
}

func TestTeacherWithUserIDLinksAndUnlinks(t *testing.T) {
	base, err := domain.NewTeacher("te1", "t1", "Ada", "Lovelace", "ada@example.com", "")
	if err != nil {
		t.Fatalf("NewTeacher: %v", err)
	}

	linked := base.WithUserID("user-9")
	if linked.UserID() != "user-9" || !linked.HasUser() {
		t.Errorf("WithUserID(link): UserID=%q HasUser=%v, want user-9/true", linked.UserID(), linked.HasUser())
	}
	// The original is unchanged (value semantics): WithUserID returns a copy.
	if base.HasUser() {
		t.Error("WithUserID must not mutate the original teacher")
	}

	unlinked := linked.WithUserID("")
	if unlinked.HasUser() || unlinked.UserID() != "" {
		t.Errorf("WithUserID(unlink): UserID=%q HasUser=%v, want \"\"/false", unlinked.UserID(), unlinked.HasUser())
	}
}

// An address typed with capitals or stray spaces is the same address: the
// catalog stores the one spelling, so two teachers cannot hide behind case.
func TestNewTeacherNormalizesEmail(t *testing.T) {
	te, err := domain.NewTeacher("te1", "t1", "Ada", "Lovelace", "  Ada.Lovelace@Colegio.MX ", "")
	if err != nil {
		t.Fatalf("NewTeacher: %v", err)
	}
	if te.Email() != "ada.lovelace@colegio.mx" {
		t.Errorf("email = %q, want %q", te.Email(), "ada.lovelace@colegio.mx")
	}
}
