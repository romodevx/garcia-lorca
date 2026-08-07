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
