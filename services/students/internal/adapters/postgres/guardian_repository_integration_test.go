//go:build integration

// Integration tests for the guardian-link side of the students postgres adapter
// against a REAL Postgres (testcontainers-go). They prove the link round-trips,
// that the (tenant, guardian, student) UNIQUE constraint surfaces as a conflict,
// and — critically — that RLS isolates tenants on `guardian_students` when
// connected as the non-owner `app` role (ADR 0001).
//
// The container, migrations, and the app-role DSN come from TestMain in
// repository_integration_test.go.
//
// Run: go test -tags=integration ./internal/adapters/postgres/
// Requires Docker.
package postgres_test

import (
	"context"
	"slices"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

// Guardian user ids are cross-service references to iam (plain UUIDs, never FKs).
const (
	guardianOne = "60000000-0000-0000-0000-000000000001"
	guardianTwo = "60000000-0000-0000-0000-000000000002"
)

func guardianLink(t *testing.T, id, tenant, guardianUserID, studentID string, rel domain.Relationship) domain.GuardianLink {
	t.Helper()
	l, err := domain.NewGuardianLink(id, tenant, guardianUserID, studentID, rel)
	if err != nil {
		t.Fatalf("new guardian link: %v", err)
	}
	return l
}

// TestGuardianLinkRoundTripAndUnlink proves a link round-trips through both list
// projections and that Unlink removes exactly it (and only once).
func TestGuardianLinkRoundTripAndUnlink(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	studentID := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-0000000000a1", "GAXA050312MDFRNN20")
	linkID := "f0000000-0000-0000-0000-0000000000a1"

	if err := repo.Link(ctx, guardianLink(t, linkID, tenantA, guardianOne, studentID, domain.RelationshipMother)); err != nil {
		t.Fatalf("Link: %v", err)
	}

	byStudent, err := repo.ListByStudent(ctx, studentID)
	if err != nil {
		t.Fatalf("ListByStudent: %v", err)
	}
	if len(byStudent) != 1 {
		t.Fatalf("ListByStudent = %d rows, want 1", len(byStudent))
	}
	got := byStudent[0]
	if got.ID() != linkID || got.GuardianUserID() != guardianOne || got.Relationship() != domain.RelationshipMother {
		t.Errorf("round-trip mismatch: %+v", got)
	}

	byGuardian, err := repo.ListByGuardian(ctx, guardianOne)
	if err != nil {
		t.Fatalf("ListByGuardian: %v", err)
	}
	if len(byGuardian) != 1 || byGuardian[0].StudentID() != studentID {
		t.Errorf("ListByGuardian = %+v, want the seeded link", byGuardian)
	}

	if err := repo.Unlink(ctx, guardianOne, studentID); err != nil {
		t.Fatalf("Unlink: %v", err)
	}
	remaining, err := repo.ListByStudent(ctx, studentID)
	if err != nil {
		t.Fatalf("ListByStudent after unlink: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("the link survived Unlink: %+v", remaining)
	}
	// Unlinking again finds nothing to delete.
	if err := repo.Unlink(ctx, guardianOne, studentID); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("second Unlink: want KindNotFound, got %v", err)
	}
}

// TestGuardianListsAreScopedToTheirSubject proves the two list projections filter
// on the right column: a guardian with two students, a student with two guardians.
func TestGuardianListsAreScopedToTheirSubject(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	first := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-0000000000a2", "GAXB050312MDFRNN21")
	second := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-0000000000a3", "GAXC050312MDFRNN22")

	// guardianOne guards both siblings; guardianTwo guards only the first.
	for i, l := range []struct {
		id, guardian, student string
		rel                   domain.Relationship
	}{
		{"f0000000-0000-0000-0000-0000000000b1", guardianOne, first, domain.RelationshipMother},
		{"f0000000-0000-0000-0000-0000000000b2", guardianOne, second, domain.RelationshipMother},
		{"f0000000-0000-0000-0000-0000000000b3", guardianTwo, first, domain.RelationshipFather},
	} {
		if err := repo.Link(ctx, guardianLink(t, l.id, tenantA, l.guardian, l.student, l.rel)); err != nil {
			t.Fatalf("seed link %d: %v", i, err)
		}
	}

	guardiansOfFirst, err := repo.ListByStudent(ctx, first)
	if err != nil {
		t.Fatalf("ListByStudent: %v", err)
	}
	if len(guardiansOfFirst) != 2 {
		t.Errorf("the first student should have 2 guardians, got %d", len(guardiansOfFirst))
	}

	studentsOfTwo, err := repo.ListByGuardian(ctx, guardianTwo)
	if err != nil {
		t.Fatalf("ListByGuardian: %v", err)
	}
	if len(studentsOfTwo) != 1 || studentsOfTwo[0].StudentID() != first {
		t.Errorf("guardianTwo should guard only the first student, got %+v", studentsOfTwo)
	}
}

// TestDuplicateGuardianLinkIsConflict proves the UNIQUE(tenant, guardian, student)
// constraint surfaces as KindConflict — the same pair cannot be linked twice even
// with a different relationship.
func TestDuplicateGuardianLinkIsConflict(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	studentID := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-0000000000a4", "GAXD050312MDFRNN23")

	if err := repo.Link(ctx, guardianLink(t, "f0000000-0000-0000-0000-0000000000c1", tenantA, guardianOne, studentID, domain.RelationshipMother)); err != nil {
		t.Fatalf("first link: %v", err)
	}
	err := repo.Link(ctx, guardianLink(t, "f0000000-0000-0000-0000-0000000000c2", tenantA, guardianOne, studentID, domain.RelationshipOther))
	if glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("duplicate link: want KindConflict, got %v", err)
	}
}

// TestGuardianLinkToMissingStudentIsInvalid proves the same-service student FK is
// enforced by the database, not just by the use case.
func TestGuardianLinkToMissingStudentIsInvalid(t *testing.T) {
	repo := appRepo(t, tenantA)
	const ghostStudent = "dddddddd-0000-0000-0000-0000000000ff"
	err := repo.Link(context.Background(),
		guardianLink(t, "f0000000-0000-0000-0000-0000000000cf", tenantA, guardianOne, ghostStudent, domain.RelationshipOther))
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("link to a missing student: want KindInvalid, got %v", err)
	}
}

// TestRLSIsolatesGuardianLinks proves read AND write isolation on
// `guardian_students` for the non-owner app role: tenant A cannot read or delete
// tenant B's link.
func TestRLSIsolatesGuardianLinks(t *testing.T) {
	ctx := context.Background()
	studentB := seedStudent(t, tenantB, "dddddddd-0000-0000-0000-0000000000b9", "GAXE050312MDFRNN24")
	linkB := "f0000000-0000-0000-0000-0000000000d9"
	if err := appRepo(t, tenantB).Link(ctx, guardianLink(t, linkB, tenantB, guardianOne, studentB, domain.RelationshipMother)); err != nil {
		t.Fatalf("seed link B: %v", err)
	}

	// Read isolation: tenant A sees none of tenant B's links; tenant B sees its own.
	fromA, err := appRepo(t, tenantA).ListByStudent(ctx, studentB)
	if err != nil {
		t.Fatalf("ListByStudent as A: %v", err)
	}
	if len(fromA) != 0 {
		t.Errorf("tenant A saw tenant B's guardian links (RLS read breach): %+v", fromA)
	}
	if fromB, err := appRepo(t, tenantB).ListByGuardian(ctx, guardianOne); err != nil || len(fromB) != 1 {
		t.Errorf("tenant B should see its own link: %+v (err %v)", fromB, err)
	}

	// Write isolation: tenant A cannot delete tenant B's link (no visible row →
	// NotFound), and the link survives.
	if err := appRepo(t, tenantA).Unlink(ctx, guardianOne, studentB); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("tenant A deleted tenant B's link (RLS write breach): %v", err)
	}
	after, err := appRepo(t, tenantB).ListByStudent(ctx, studentB)
	if err != nil {
		t.Fatalf("re-read link B: %v", err)
	}
	if len(after) != 1 {
		t.Errorf("tenant A's delete leaked into tenant B: %d links remain", len(after))
	}
}

// A pupil has several contacts and most have no account. The UNIQUE index is on
// (tenant, guardian_user_id, student) and Postgres does not collide NULLs —
// which is exactly what makes this work, and exactly the kind of thing that is
// right in a fake and wrong against SQL.
func TestSeveralContactsWithoutAnAccountOnOnePupil(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	studentID := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-0000000000c1", "COXA050312MDFRNN21")

	for i, person := range []domain.ContactPerson{
		{FullName: "Rosa Méndez", Phone: "5551234567"},
		{FullName: "Luis Cruz", Phone: "5559876543"},
	} {
		contact, err := domain.NewContact(
			"f0000000-0000-0000-0000-0000000000e"+string(rune('1'+i)),
			tenantA, studentID, domain.RelationshipOther, person)
		if err != nil {
			t.Fatalf("NewContact(%s): %v", person.FullName, err)
		}
		if err := repo.Link(ctx, contact); err != nil {
			t.Fatalf("Link(%s): %v", person.FullName, err)
		}
	}

	// The guardian with an account sits in the same list.
	if err := repo.Link(ctx, guardianLink(t, "f0000000-0000-0000-0000-0000000000e9",
		tenantA, guardianOne, studentID, domain.RelationshipMother)); err != nil {
		t.Fatalf("Link(guardian): %v", err)
	}

	contacts, err := repo.ListByStudent(ctx, studentID)
	if err != nil {
		t.Fatalf("ListByStudent: %v", err)
	}
	if len(contacts) != 3 {
		t.Fatalf("listed %d contacts, want 3", len(contacts))
	}
	var withAccount, phones int
	for _, c := range contacts {
		if c.HasAccount() {
			withAccount++
		}
		if c.Phone() != "" {
			phones++
		}
	}
	if withAccount != 1 {
		t.Errorf("%d contacts can open the app, want exactly the linked guardian", withAccount)
	}
	if phones != 2 {
		t.Errorf("%d contacts carry a phone, want 2", phones)
	}

	// The family read path is unchanged: it asks by user id and never sees the
	// account-less rows.
	byGuardian, err := repo.ListByGuardian(ctx, guardianOne)
	if err != nil {
		t.Fatalf("ListByGuardian: %v", err)
	}
	for _, c := range byGuardian {
		if !c.HasAccount() {
			t.Errorf("the family read path returned an account-less contact: %+v", c)
		}
	}
}

// TestGrantingAccessKeepsOneRow proves the whole point of AttachAccount: the
// account lands on the contact that is already there. The office typed the
// mother once; after she is given the app she is still one entry, still carrying
// the phone that answers.
func TestGrantingAccessKeepsOneRow(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	studentID := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-0000000000d1", "COXA050312MDFRNN31")

	contact, err := domain.NewContact("f0000000-0000-0000-0000-0000000000f1", tenantA, studentID,
		domain.RelationshipMother,
		domain.ContactPerson{FullName: "Ana Cruz", Phone: "5559876543", Email: "ana@example.com"})
	if err != nil {
		t.Fatalf("NewContact: %v", err)
	}
	if err := repo.Link(ctx, contact); err != nil {
		t.Fatalf("Link: %v", err)
	}

	stored, err := repo.ByID(ctx, contact.ID())
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if stored.HasAccount() {
		t.Fatal("a contact recorded by the office must start with no account")
	}

	withAccount, err := stored.WithAccount(guardianOne)
	if err != nil {
		t.Fatalf("WithAccount: %v", err)
	}
	if err := repo.AttachAccount(ctx, withAccount); err != nil {
		t.Fatalf("AttachAccount: %v", err)
	}

	contacts, err := repo.ListByStudent(ctx, studentID)
	if err != nil {
		t.Fatalf("ListByStudent: %v", err)
	}
	if len(contacts) != 1 {
		t.Fatalf("the pupil has %d contacts, want 1 — the account must not create a second person", len(contacts))
	}
	got := contacts[0]
	if !got.HasAccount() || got.GuardianUserID() != guardianOne {
		t.Errorf("account did not persist: %+v", got)
	}
	if got.FullName() != "Ana Cruz" || got.Phone() != "5559876543" {
		t.Errorf("the person was lost on the way: %+v", got)
	}

	// And she is now reachable from the family read path, which is the entire
	// purpose of giving her the account.
	//
	// Se busca a ESTA hija, no se cuentan las filas. `guardianOne` es el mismo
	// tutor en media docena de pruebas de este archivo, y todas comparten un
	// Postgres: exigir `len(mine) == 1` sólo era cierto si esta prueba corría
	// primero, y corre de las últimas. La falla estuvo latente desde siempre —
	// las pruebas de integración sólo corren en PRs (ci.yml:190), así que los
	// push a dev nunca la ejecutaron.
	mine, err := repo.ListByGuardian(ctx, guardianOne)
	if err != nil {
		t.Fatalf("ListByGuardian: %v", err)
	}
	if !slices.ContainsFunc(mine, func(l domain.GuardianLink) bool { return l.StudentID() == studentID }) {
		t.Errorf("the guardian cannot see their child: %+v", mine)
	}
}

// TestAttachAccountIsConditional proves the write refuses a contact that already
// has an account. Two clerks granting access at the same time must not let the
// second silently move the pupil to a different family: the UPDATE is guarded on
// the account still being absent, so the loser gets a conflict, not the child.
func TestAttachAccountIsConditional(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	studentID := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-0000000000d2", "COXA050312MDFRNN41")

	contact, err := domain.NewContact("f0000000-0000-0000-0000-0000000000f2", tenantA, studentID,
		domain.RelationshipFather, domain.ContactPerson{FullName: "Mateo Contreras"})
	if err != nil {
		t.Fatalf("NewContact: %v", err)
	}
	if err := repo.Link(ctx, contact); err != nil {
		t.Fatalf("Link: %v", err)
	}

	first, err := contact.WithAccount(guardianOne)
	if err != nil {
		t.Fatalf("WithAccount: %v", err)
	}
	if err := repo.AttachAccount(ctx, first); err != nil {
		t.Fatalf("first AttachAccount: %v", err)
	}

	// The second caller still holds the pre-grant contact — exactly the stale
	// read a race produces — and tries to attach a different account to it.
	second, err := contact.WithAccount(guardianTwo)
	if err != nil {
		t.Fatalf("WithAccount(second): %v", err)
	}
	if err := repo.AttachAccount(ctx, second); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("racing grant: want KindConflict, got %v", err)
	}

	stored, err := repo.ByID(ctx, contact.ID())
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if stored.GuardianUserID() != guardianOne {
		t.Errorf("the losing grant overwrote the winner: %+v", stored)
	}
}

// TestByIDIsTenantScoped: a contact of another tenant is NOT FOUND, not
// forbidden. RLS makes the two answers identical, so the reply never confirms
// that somebody else's contact exists.
func TestByIDIsTenantScoped(t *testing.T) {
	ctx := context.Background()
	studentID := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-0000000000d3", "COXA050312MDFRNN51")

	contact, err := domain.NewContact("f0000000-0000-0000-0000-0000000000f3", tenantA, studentID,
		domain.RelationshipOther, domain.ContactPerson{FullName: "Rosa Méndez"})
	if err != nil {
		t.Fatalf("NewContact: %v", err)
	}
	if err := appRepo(t, tenantA).Link(ctx, contact); err != nil {
		t.Fatalf("Link: %v", err)
	}

	if _, err := appRepo(t, tenantB).ByID(ctx, contact.ID()); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("cross-tenant ByID: want KindNotFound, got %v", err)
	}
}
