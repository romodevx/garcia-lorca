package domain_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

func TestNewGuardianLinkAccessors(t *testing.T) {
	l, err := domain.NewGuardianLink("gl-1", "t1", "usr-1", "stu-1", domain.RelationshipMother)
	if err != nil {
		t.Fatalf("NewGuardianLink: %v", err)
	}
	if l.ID() != "gl-1" || l.TenantID() != "t1" || l.GuardianUserID() != "usr-1" ||
		l.StudentID() != "stu-1" || l.Relationship() != domain.RelationshipMother {
		t.Errorf("accessors mismatch: %+v", l)
	}
}

func TestNewGuardianLinkValidation(t *testing.T) {
	cases := []struct {
		name                                    string
		id, tenantID, guardianUserID, studentID string
		relationship                            domain.Relationship
	}{
		{"empty id", "", "t1", "usr-1", "stu-1", domain.RelationshipFather},
		{"blank id", "   ", "t1", "usr-1", "stu-1", domain.RelationshipFather},
		{"empty tenant", "gl-1", "", "usr-1", "stu-1", domain.RelationshipFather},
		{"empty guardian user", "gl-1", "t1", "", "stu-1", domain.RelationshipFather},
		{"empty student", "gl-1", "t1", "usr-1", "", domain.RelationshipFather},
		{"unspecified relationship", "gl-1", "t1", "usr-1", "stu-1", domain.RelationshipUnspecified},
		{"out-of-range relationship", "gl-1", "t1", "usr-1", "stu-1", domain.Relationship(99)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewGuardianLink(tc.id, tc.tenantID, tc.guardianUserID, tc.studentID, tc.relationship)
			if glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("want KindInvalid, got %v", err)
			}
		})
	}
}

// The canonical names are the DB CHECK vocabulary of guardian_students
// (mother|father|guardian|other) — the wire format is these strings.
func TestParseRelationshipRoundTrip(t *testing.T) {
	for _, name := range []string{"mother", "father", "guardian", "other"} {
		t.Run(name, func(t *testing.T) {
			rel, err := domain.ParseRelationship(name)
			if err != nil {
				t.Fatalf("ParseRelationship(%q): %v", name, err)
			}
			if !rel.Valid() {
				t.Errorf("%q parsed to an invalid relationship", name)
			}
			if rel.String() != name {
				t.Errorf("round-trip mismatch: %q -> %q", name, rel.String())
			}
		})
	}
}

func TestParseRelationshipNormalizesAndRejects(t *testing.T) {
	t.Run("trims and lowercases", func(t *testing.T) {
		rel, err := domain.ParseRelationship("  Mother ")
		if err != nil {
			t.Fatalf("ParseRelationship: %v", err)
		}
		if rel != domain.RelationshipMother {
			t.Errorf("got %v, want mother", rel)
		}
	})
	for _, raw := range []string{"", "uncle", "madre"} {
		t.Run("rejects "+raw, func(t *testing.T) {
			rel, err := domain.ParseRelationship(raw)
			if glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("want KindInvalid for %q, got %v", raw, err)
			}
			if rel != domain.RelationshipUnspecified {
				t.Errorf("a rejected relationship must be unspecified, got %v", rel)
			}
		})
	}
}

// A grandmother who may collect the child is a contact the school needs, and
// she has no reason to have a login. Requiring one is why the office kept the
// second number on paper.
func TestAContactMayHaveNoAccount(t *testing.T) {
	link, err := domain.NewContact("gl-1", "t1", "st-1", domain.RelationshipOther,
		domain.ContactPerson{FullName: "Rosa Méndez", Phone: "5551234567"})
	if err != nil {
		t.Fatalf("NewContact: %v", err)
	}
	if link.GuardianUserID() != "" {
		t.Errorf("user id = %q, want empty — this contact has no account", link.GuardianUserID())
	}
	if link.FullName() != "Rosa Méndez" || link.Phone() != "5551234567" {
		t.Errorf("contact wrong: %+v", link)
	}
}

// A contact with neither an account nor a name is nobody, and a row for nobody
// is a phone number the office cannot use.
func TestAContactMustBeSomebody(t *testing.T) {
	_, err := domain.NewContact("gl-1", "t1", "st-1", domain.RelationshipOther, domain.ContactPerson{Phone: "5551234567"})
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("nameless account-less contact: want KindInvalid, got %v", err)
	}
}

// A guardian with an account keeps working exactly as before, and may now also
// carry the phone that actually answers.
func TestALinkedGuardianCanCarryAPhone(t *testing.T) {
	link, err := domain.NewGuardianLink("gl-1", "t1", "user-1", "st-1", domain.RelationshipMother)
	if err != nil {
		t.Fatalf("NewGuardianLink: %v", err)
	}
	withPhone := link.WithPerson(domain.ContactPerson{FullName: "Ana Cruz", Phone: "5559876543"})
	if withPhone.Phone() != "5559876543" || withPhone.GuardianUserID() != "user-1" {
		t.Errorf("contact details did not attach: %+v", withPhone)
	}
}
