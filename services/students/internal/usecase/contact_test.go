package usecase_test

import (
	"context"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// contactFixture is one enrolled pupil plus one guardian account, which is the
// shape every contact test starts from.
type contactFixture struct {
	svc            *usecase.GuardianService
	studentID      string
	guardianUserID string
}

func newGuardianFixture(t *testing.T) contactFixture {
	t.Helper()
	svc, students, _, users := newGuardianService(t)
	seedStudent(t, students, "stu-1")
	seedGuardianUser(users, "usr-1", "guardian")
	return contactFixture{svc: svc, studentID: "stu-1", guardianUserID: "usr-1"}
}

// The grandmother who may collect the child. No login, no role, no account —
// and the school needs her name and her phone.
func TestAddContactStoresSomebodyWithNoAccount(t *testing.T) {
	f := newGuardianFixture(t)

	contact, err := f.svc.AddContact(context.Background(), usecase.ContactInput{
		StudentID:    f.studentID,
		Relationship: "other",
		FullName:     "Rosa Méndez",
		Phone:        "5551234567",
	})
	if err != nil {
		t.Fatalf("AddContact: %v", err)
	}
	if contact.HasAccount() {
		t.Error("the contact was given an account nobody asked for")
	}
	if contact.FullName() != "Rosa Méndez" || contact.Phone() != "5551234567" {
		t.Errorf("contact wrong: %+v", contact)
	}

	listed, err := f.svc.ListGuardiansForStudent(context.Background(), f.studentID)
	if err != nil {
		t.Fatalf("ListGuardiansForStudent: %v", err)
	}
	if len(listed) != 1 {
		t.Errorf("listed %d contacts, want 1", len(listed))
	}
}

// A contact on a pupil who does not exist is a typo, and one nobody would ever
// find again.
func TestAddContactToAnUnknownStudentIsRefused(t *testing.T) {
	f := newGuardianFixture(t)

	_, err := f.svc.AddContact(context.Background(), usecase.ContactInput{
		StudentID: "no-such-student", Relationship: "mother", FullName: "Rosa",
	})
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("unknown student: want KindInvalid, got %v", err)
	}
}

// A phone number with no name attached is a number nobody can use.
func TestAddContactNeedsAName(t *testing.T) {
	f := newGuardianFixture(t)

	_, err := f.svc.AddContact(context.Background(), usecase.ContactInput{
		StudentID: f.studentID, Relationship: "other", Phone: "5551234567",
	})
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("nameless contact: want KindInvalid, got %v", err)
	}
}

// A pupil has several contacts, which is the whole point. The account-holding
// guardian and the two phone numbers all live in the same list.
func TestAPupilCanHaveSeveralContacts(t *testing.T) {
	f := newGuardianFixture(t)

	for _, name := range []string{"Rosa Méndez", "Luis Cruz"} {
		if _, err := f.svc.AddContact(context.Background(), usecase.ContactInput{
			StudentID: f.studentID, Relationship: "other", FullName: name, Phone: "5550000000",
		}); err != nil {
			t.Fatalf("AddContact(%s): %v", name, err)
		}
	}
	if _, err := f.svc.LinkGuardian(context.Background(), f.guardianUserID, f.studentID, "mother"); err != nil {
		t.Fatalf("LinkGuardian: %v", err)
	}

	listed, err := f.svc.ListGuardiansForStudent(context.Background(), f.studentID)
	if err != nil {
		t.Fatalf("ListGuardiansForStudent: %v", err)
	}
	if len(listed) != 3 {
		t.Errorf("listed %d contacts, want 3", len(listed))
	}
	var withAccount int
	for _, c := range listed {
		if c.HasAccount() {
			withAccount++
		}
	}
	if withAccount != 1 {
		t.Errorf("%d contacts can open the app, want exactly the linked guardian", withAccount)
	}
}

// A relationship outside the vocabulary is a typo the database would reject
// anyway; hearing about it here is how the caller learns which word to use.
func TestAddContactRejectsAnUnknownRelationship(t *testing.T) {
	f := newGuardianFixture(t)

	_, err := f.svc.AddContact(context.Background(), usecase.ContactInput{
		StudentID: f.studentID, Relationship: "vecino", FullName: "Rosa",
	})
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("unknown relationship: want KindInvalid, got %v", err)
	}
}
