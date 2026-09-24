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

// A contact the school already recorded becomes a family that can open the app.
// The account attaches to the row that is already there: the pupil keeps ONE
// list of contacts, and the office keeps reading one name per person.
func TestAContactGainsAnAccount(t *testing.T) {
	contact, err := domain.NewContact("gl-1", "t1", "st-1", domain.RelationshipFather,
		domain.ContactPerson{FullName: "Mateo Contreras", Phone: "5551234567", Email: "mateo@example.com"})
	if err != nil {
		t.Fatalf("NewContact: %v", err)
	}
	if contact.HasAccount() {
		t.Fatal("a fresh contact must not have an account")
	}

	withAccount, err := contact.WithAccount("user-9")
	if err != nil {
		t.Fatalf("WithAccount: %v", err)
	}
	if !withAccount.HasAccount() || withAccount.GuardianUserID() != "user-9" {
		t.Errorf("account did not attach: %+v", withAccount)
	}
	if withAccount.ID() != "gl-1" || withAccount.FullName() != "Mateo Contreras" {
		t.Errorf("attaching an account must not disturb the person: %+v", withAccount)
	}
	if contact.HasAccount() {
		t.Error("WithAccount must not mutate the receiver")
	}
}

// Attaching an empty user id would silently produce a contact that claims an
// account it does not have — and the family read path authorizes on exactly
// that field.
func TestAnAccountNeedsAUserID(t *testing.T) {
	contact, err := domain.NewContact("gl-1", "t1", "st-1", domain.RelationshipFather,
		domain.ContactPerson{FullName: "Mateo Contreras"})
	if err != nil {
		t.Fatalf("NewContact: %v", err)
	}
	if _, err := contact.WithAccount("   "); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("blank user id: want KindInvalid, got %v", err)
	}
}

// Handing an account to a contact that already has one would quietly move a
// pupil from one family to another. The second grant must be refused, not won
// by whoever asked last.
func TestAContactKeepsTheAccountItAlreadyHas(t *testing.T) {
	linked, err := domain.NewGuardianLink("gl-1", "t1", "user-1", "st-1", domain.RelationshipMother)
	if err != nil {
		t.Fatalf("NewGuardianLink: %v", err)
	}
	if _, err := linked.WithAccount("user-2"); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("re-granting an account: want KindConflict, got %v", err)
	}
}

// A contact's email is stored as one spelling — trimmed and lowercase — so the
// invitation that later looks it up in iam finds the same address.
func TestAContactEmailIsNormalized(t *testing.T) {
	contact, err := domain.NewContact("gl-1", "t1", "st-1", domain.RelationshipMother,
		domain.ContactPerson{FullName: "Ana Cruz", Email: "  Ana.Cruz@Correo.COM "})
	if err != nil {
		t.Fatalf("NewContact: %v", err)
	}
	if contact.Email() != "ana.cruz@correo.com" {
		t.Errorf("email = %q, want %q", contact.Email(), "ana.cruz@correo.com")
	}
}

// A contact's email, when given, must be an address iam will accept: otherwise
// it is stored today and fails weeks later, when the office grants app access.
func TestAContactEmailMustBePlausible(t *testing.T) {
	for _, bad := range []string{"ana.ruiz.gmail.com", "ana ruiz@x.mx", "ana@gmail"} {
		_, err := domain.NewContact("gl-1", "t1", "st-1", domain.RelationshipMother,
			domain.ContactPerson{FullName: "Ana Cruz", Email: bad})
		if glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Errorf("NewContact with email %q: want KindInvalid, got %v", bad, err)
		}
	}
}

func TestAContactMayHaveNoEmail(t *testing.T) {
	if _, err := domain.NewContact("gl-1", "t1", "st-1", domain.RelationshipMother,
		domain.ContactPerson{FullName: "Ana Cruz", Phone: "5551234567"}); err != nil {
		t.Fatalf("a contact without an email is fine: %v", err)
	}
}

// A contact read back from storage is restored, not re-validated: a row stored
// before contact emails were shape-checked must not take the pupil's whole
// contact list down.
func TestRestoreContactToleratesALegacyEmail(t *testing.T) {
	contact, err := domain.RestoreContact("gl-1", "t1", "st-1", domain.RelationshipMother,
		domain.ContactPerson{FullName: "Ana Cruz", Email: "maria@gmail"})
	if err != nil {
		t.Fatalf("RestoreContact: %v", err)
	}
	if contact.Email() != "maria@gmail" {
		t.Errorf("email = %q", contact.Email())
	}
}

// A contact's phone, when given, is the ten digits the office can dial.
func TestAContactPhoneIsTenDigits(t *testing.T) {
	c, err := domain.NewContact("gl-1", "t1", "st-1", domain.RelationshipMother,
		domain.ContactPerson{FullName: "Ana Cruz", Phone: "55 1234-5678", Emergency: true})
	if err != nil {
		t.Fatalf("NewContact: %v", err)
	}
	if c.Phone() != "5512345678" || !c.IsEmergency() {
		t.Errorf("contact = %q emergency %v", c.Phone(), c.IsEmergency())
	}
	if _, err := domain.NewContact("gl-2", "t1", "st-1", domain.RelationshipMother,
		domain.ContactPerson{FullName: "Ana Cruz", Phone: "12345"}); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("a short phone must be refused, got %v", err)
	}
}

// Editing a contact re-judges what the office typed, and keeps the account.
func TestEditingAContact(t *testing.T) {
	c, _ := domain.NewContact("gl-1", "t1", "st-1", domain.RelationshipOther, domain.ContactPerson{FullName: "Rosa"})
	edited, err := c.Edit(domain.RelationshipGuardian, domain.ContactPerson{FullName: " Rosa Méndez ", Phone: "3312345678", Email: "Rosa@Correo.mx", Emergency: true})
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if edited.FullName() != "Rosa Méndez" || edited.Relationship() != domain.RelationshipGuardian ||
		edited.Email() != "rosa@correo.mx" || !edited.IsEmergency() || edited.ID() != "gl-1" {
		t.Errorf("edited = %+v", edited)
	}
	if _, err := c.Edit(domain.RelationshipOther, domain.ContactPerson{FullName: " "}); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("a contact without an account needs a name, got %v", err)
	}
	if _, err := c.Edit(domain.RelationshipOther, domain.ContactPerson{FullName: "Rosa", Phone: "1"}); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("a malformed phone must be refused, got %v", err)
	}
}
